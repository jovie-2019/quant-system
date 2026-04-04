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

	"quant-system/internal/adminstore"
	"quant-system/internal/crypto"
	"quant-system/internal/notify"
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

	mux.Handle("/api/v1/", s.JWTMiddleware(auth))

	// Alert webhook (no auth — called by Alertmanager directly).
	mux.HandleFunc("/api/v1/alerts/webhook", s.HandleAlertWebhook)

	// Serve static files, fallback to index.html for SPA routing.
	if s.staticDir != "" {
		mux.HandleFunc("/", s.handleStatic)
	}

	return mux
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

// routeAccountByID dispatches GET/DELETE for /api/v1/accounts/{id}.
func (s *Server) routeAccountByID(w http.ResponseWriter, r *http.Request) {
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
