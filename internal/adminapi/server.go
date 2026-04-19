package adminapi

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"quant-system/internal/adminstore"
	"quant-system/internal/crypto"
	"quant-system/internal/marketstore"
	"quant-system/internal/notify"
	"quant-system/internal/obs/metrics"
	"quant-system/internal/store/mysqlstore"
)

// Server is the admin API HTTP server.
type Server struct {
	store     *adminstore.Store
	repo      *mysqlstore.Repository
	encryptor *crypto.Encryptor
	jwtSecret []byte
	passHash  string
	logger    *slog.Logger
	staticDir string
	feishu    *notify.FeishuClient

	riskMu  sync.RWMutex
	riskCfg RiskConfigPayload

	// backtests holds the in-memory registry of recent backtest runs. It is
	// bounded (default 100) with oldest-first eviction. When ClickHouse lands
	// this can be backed by a durable store without changing the handlers.
	backtests *BacktestStore

	// klines is the optional ClickHouse-backed historical kline source used
	// when a BacktestRequest selects dataset.source="clickhouse". Nil means
	// only the synthetic source is available.
	klines marketstore.KlineStore

	// regimes is the optional persistent store for classifier output.
	// When nil, the /api/v1/regime/* endpoints return 503.
	regimes marketstore.RegimeStore
}

// Config holds admin API configuration.
type Config struct {
	Store            *adminstore.Store
	Repo             *mysqlstore.Repository
	Encryptor        *crypto.Encryptor
	JWTSecret        string // hex encoded
	PassHash         string // bcrypt hash
	Logger           *slog.Logger
	StaticDir        string // directory for static files (empty = disabled)
	FeishuWebhookURL string // Feishu webhook URL for alert forwarding (empty = disabled)

	// KlineStore is an optional historical kline source used by backtests
	// with dataset.source="clickhouse". Leaving it nil restricts the
	// backtest API to the synthetic source.
	KlineStore marketstore.KlineStore

	// RegimeStore is the optional persistent store for regime classifier
	// output. When nil, the /api/v1/regime/* endpoints return 503.
	RegimeStore marketstore.RegimeStore
}

var (
	// ErrNilStore is returned when Config.Store is nil.
	ErrNilStore = errors.New("adminapi: store is nil")

	// ErrNilEncryptor is returned when Config.Encryptor is nil.
	ErrNilEncryptor = errors.New("adminapi: encryptor is nil")

	// ErrInvalidJWTSecret is returned when JWTSecret is not valid hex.
	ErrInvalidJWTSecret = errors.New("adminapi: invalid JWT secret")
)

// NewServer creates a new admin API server.
func NewServer(cfg Config) (*Server, error) {
	if cfg.Store == nil {
		return nil, ErrNilStore
	}
	if cfg.Encryptor == nil {
		return nil, ErrNilEncryptor
	}

	secret, err := hex.DecodeString(cfg.JWTSecret)
	if err != nil || len(secret) == 0 {
		return nil, ErrInvalidJWTSecret
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	var feishuClient *notify.FeishuClient
	if cfg.FeishuWebhookURL != "" {
		feishuClient = notify.NewFeishuClient(cfg.FeishuWebhookURL)
	}

	return &Server{
		store:     cfg.Store,
		repo:      cfg.Repo,
		encryptor: cfg.Encryptor,
		jwtSecret: secret,
		passHash:  cfg.PassHash,
		logger:    logger,
		staticDir: cfg.StaticDir,
		feishu:    feishuClient,
		backtests: NewBacktestStore(100),
		klines:    cfg.KlineStore,
		regimes:   cfg.RegimeStore,
	}, nil
}

// Handler returns the HTTP handler with all routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Public routes (no auth).
	mux.HandleFunc("/api/v1/health", s.handleHealth)
	mux.HandleFunc("/metrics", s.handleMetrics)
	mux.HandleFunc("/api/v1/auth/login", s.HandleLogin)

	// Authenticated routes.
	auth := http.NewServeMux()

	// Exchanges.
	auth.HandleFunc("/api/v1/exchanges", s.routeExchanges)
	auth.HandleFunc("/api/v1/exchanges/", s.routeExchangeByID)

	// Accounts (API Keys).
	auth.HandleFunc("/api/v1/accounts", s.routeAccounts)
	auth.HandleFunc("/api/v1/accounts/", s.routeAccountByID)

	// Strategy types (metadata catalog).
	auth.HandleFunc("/api/v1/strategy-types", s.HandleListStrategyTypes)

	// Strategies.
	auth.HandleFunc("/api/v1/strategies/stop-all", s.HandleStopAll)
	auth.HandleFunc("/api/v1/strategies", s.routeStrategies)
	auth.HandleFunc("/api/v1/strategies/", s.routeStrategyByID)

	// Positions and orders.
	auth.HandleFunc("/api/v1/positions", s.HandleListPositions)
	auth.HandleFunc("/api/v1/orders", s.HandleListOrders)

	// Risk config.
	auth.HandleFunc("/api/v1/risk/config", s.routeRiskConfig)

	// Overview.
	auth.HandleFunc("/api/v1/overview", s.HandleOverview)

	// System status.
	auth.HandleFunc("/api/v1/system/status", s.HandleSystemStatus)

	// Backtests.
	auth.HandleFunc("/api/v1/backtests", s.routeBacktests)
	auth.HandleFunc("/api/v1/backtests/", s.HandleGetBacktest)

	// Regime (market state classifier).
	auth.HandleFunc("/api/v1/regime/compute", s.HandleComputeRegime)
	auth.HandleFunc("/api/v1/regime/history", s.HandleRegimeHistory)
	auth.HandleFunc("/api/v1/regime/matrix", s.HandleRegimeMatrix)

	mux.Handle("/api/v1/", s.JWTMiddleware(auth))

	// Alert webhook (no auth — called by Alertmanager directly).
	mux.HandleFunc("/api/v1/alerts/webhook", s.HandleAlertWebhook)

	// Serve static files, fallback to index.html for SPA routing.
	if s.staticDir != "" {
		mux.HandleFunc("/", s.handleStatic)
	}

	return s.metricsMiddleware(mux)
}

// statusRecorder wraps http.ResponseWriter to capture the status code.
type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code before delegating to the wrapped writer.
func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

// metricsMiddleware records request count and latency for each HTTP request.
func (s *Server) metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, statusCode: 200}
		next.ServeHTTP(rec, r)
		metrics.ObserveHTTP(r.Method, r.URL.Path, rec.statusCode, time.Since(start))
	})
}

// handleHealth handles GET /api/v1/health.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleMetrics serves Prometheus metrics placeholder.
func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "# placeholder for prometheus metrics")
}

// routeExchanges dispatches GET/POST to the correct exchange handler.
func (s *Server) routeExchanges(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.HandleListExchanges(w, r)
	case http.MethodPost:
		s.HandleCreateExchange(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET or POST required")
	}
}

// routeExchangeByID dispatches GET/PUT/DELETE for /api/v1/exchanges/{id}.
func (s *Server) routeExchangeByID(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.HandleGetExchange(w, r)
	case http.MethodPut:
		s.HandleUpdateExchange(w, r)
	case http.MethodDelete:
		s.HandleDeleteExchange(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET, PUT or DELETE required")
	}
}

// routeAccounts dispatches GET/POST to the correct account handler.
func (s *Server) routeAccounts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.HandleListAPIKeys(w, r)
	case http.MethodPost:
		s.HandleCreateAPIKey(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET or POST required")
	}
}

// routeAccountByID dispatches GET/DELETE for /api/v1/accounts/{id} and sub-routes.
func (s *Server) routeAccountByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/accounts/")

	// Check for sub-actions: {id}/balance.
	if parts := strings.SplitN(path, "/", 2); len(parts) == 2 {
		switch parts[1] {
		case "balance":
			s.HandleGetBalance(w, r)
			return
		}
	}

	switch r.Method {
	case http.MethodGet:
		s.HandleGetAPIKey(w, r)
	case http.MethodDelete:
		s.HandleDeleteAPIKey(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET or DELETE required")
	}
}

// routeStrategies dispatches GET/POST/PUT to the correct strategy handler.
func (s *Server) routeStrategies(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.HandleListStrategies(w, r)
	case http.MethodPost:
		s.HandleCreateStrategy(w, r)
	case http.MethodPut:
		s.HandleUpdateStrategy(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET, POST or PUT required")
	}
}

// routeStrategyByID dispatches GET/PUT/DELETE and sub-actions for /api/v1/strategies/{id}.
func (s *Server) routeStrategyByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/strategies/")

	// Handle stop-all (routed here because /strategies/ pattern matches first).
	if path == "stop-all" {
		s.HandleStopAll(w, r)
		return
	}

	// Check for sub-actions: {id}/start or {id}/stop.
	if parts := strings.SplitN(path, "/", 2); len(parts) == 2 {
		switch parts[1] {
		case "start":
			s.HandleStartStrategy(w, r)
			return
		case "stop":
			s.HandleStopStrategy(w, r)
			return
		case "logs":
			s.HandleStrategyLogs(w, r)
			return
		}
	}

	switch r.Method {
	case http.MethodGet:
		s.HandleGetStrategy(w, r)
	case http.MethodPut:
		s.HandleUpdateStrategy(w, r)
	case http.MethodDelete:
		s.HandleDeleteStrategy(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET, PUT or DELETE required")
	}
}

// routeBacktests dispatches GET/POST for /api/v1/backtests.
func (s *Server) routeBacktests(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.HandleListBacktests(w, r)
	case http.MethodPost:
		s.HandleCreateBacktest(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET or POST required")
	}
}

// routeRiskConfig dispatches GET/PUT to the risk config handler.
func (s *Server) routeRiskConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.HandleGetRiskConfig(w, r)
	case http.MethodPut:
		s.HandleUpdateRiskConfig(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET or PUT required")
	}
}

// handleStatic serves static files from the configured directory.
// If the requested file does not exist, it serves index.html for SPA routing.
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	path := filepath.Join(s.staticDir, filepath.Clean(r.URL.Path))
	if _, err := os.Stat(path); os.IsNotExist(err) {
		http.ServeFile(w, r, filepath.Join(s.staticDir, "index.html"))
		return
	}
	http.ServeFile(w, r, path)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// writeJSON writes a JSON response with the given status code.
func (s *Server) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.Error("writeJSON: encode failed", "error", err)
	}
}

// writeError writes a JSON error response with the given status code.
func (s *Server) writeError(w http.ResponseWriter, status int, code, message string) {
	s.writeJSON(w, status, map[string]string{
		"error":   code,
		"message": message,
	})
}

// readJSON decodes the request body JSON into dst.
func (s *Server) readJSON(r *http.Request, dst any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MB limit
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

// pathParam extracts a path parameter from the URL by stripping the route
// prefix. For example, a request to "/api/v1/exchanges/42" with prefix
// "/api/v1/exchanges/" yields "42".
func (s *Server) pathParam(r *http.Request, name string) string {
	prefixes := map[string]string{
		"exchange_id": "/api/v1/exchanges/",
		"account_id":  "/api/v1/accounts/",
		"strategy_id": "/api/v1/strategies/",
	}
	prefix, ok := prefixes[name]
	if !ok {
		// Generic "id" param: find the last path segment with a known prefix.
		for _, p := range prefixes {
			if strings.HasPrefix(r.URL.Path, p) {
				prefix = p
				ok = true
				break
			}
		}
		if !ok {
			return ""
		}
	}
	remainder := strings.TrimPrefix(r.URL.Path, prefix)
	// Strip any trailing sub-path (e.g. "42/start" -> "42").
	if idx := strings.Index(remainder, "/"); idx != -1 {
		remainder = remainder[:idx]
	}
	return remainder
}
