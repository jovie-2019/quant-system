package controlapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"quant-system/internal/obs/metrics"
	"quant-system/internal/risk"
)

type RiskConfigSetter interface {
	SetConfig(config risk.Config)
}

type StrategyState struct {
	ID        string         `json:"id"`
	Running   bool           `json:"running"`
	Config    map[string]any `json:"config"`
	UpdatedMS int64          `json:"updated_ms"`
}

type RiskConfigPayload struct {
	MaxOrderQty    float64  `json:"max_order_qty"`
	MaxOrderAmount float64  `json:"max_order_amount"`
	AllowedSymbols []string `json:"allowed_symbols"`
}

type Server struct {
	mu         sync.RWMutex
	strategies map[string]StrategyState
	riskConfig RiskConfigPayload
	riskSetter RiskConfigSetter
}

func NewServer(riskSetter RiskConfigSetter) *Server {
	return &Server{
		strategies: make(map[string]StrategyState),
		riskSetter: riskSetter,
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && r.URL.Path == "/metrics" {
		s.handleMetrics(w)
		return
	}

	start := time.Now()
	rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}

	defer func() {
		metrics.ObserveHTTP(
			r.Method,
			normalizePath(r.URL.Path),
			rec.statusCode,
			time.Since(start),
		)
	}()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/health":
		s.handleHealth(rec)
		return
	case strings.HasPrefix(r.URL.Path, "/api/v1/strategies/"):
		s.handleStrategies(rec, r)
		return
	case r.Method == http.MethodPut && r.URL.Path == "/api/v1/risk/config":
		s.handleRiskConfig(rec, r)
		return
	default:
		writeJSON(rec, http.StatusNotFound, map[string]any{
			"error": "not_found",
		})
	}
}

func (s *Server) GetStrategyState(id string) (StrategyState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.strategies[id]
	return state, ok
}

func (s *Server) GetRiskConfig() RiskConfigPayload {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.riskConfig
}

func (s *Server) handleHealth(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
	})
}

func (s *Server) handleMetrics(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(metrics.ExposePrometheus()))
}

func (s *Server) handleStrategies(w http.ResponseWriter, r *http.Request) {
	id, action, ok := parseStrategyAction(r.URL.Path)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
		return
	}

	switch {
	case r.Method == http.MethodPost && action == "start":
		s.setStrategyRunning(id, true)
		writeJSON(w, http.StatusOK, map[string]any{"status": "started", "strategy_id": id})
	case r.Method == http.MethodPost && action == "stop":
		s.setStrategyRunning(id, false)
		writeJSON(w, http.StatusOK, map[string]any{"status": "stopped", "strategy_id": id})
	case r.Method == http.MethodPut && action == "config":
		var req struct {
			Config map[string]any `json:"config"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json"})
			return
		}
		s.setStrategyConfig(id, req.Config)
		writeJSON(w, http.StatusOK, map[string]any{"status": "config_updated", "strategy_id": id})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed"})
	}
}

func (s *Server) handleRiskConfig(w http.ResponseWriter, r *http.Request) {
	var req RiskConfigPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json"})
		return
	}

	s.mu.Lock()
	s.riskConfig = req
	s.mu.Unlock()

	if s.riskSetter != nil {
		allowed := make(map[string]struct{}, len(req.AllowedSymbols))
		for _, symbol := range req.AllowedSymbols {
			allowed[strings.ToUpper(strings.TrimSpace(symbol))] = struct{}{}
		}
		s.riskSetter.SetConfig(risk.Config{
			MaxOrderQty:    req.MaxOrderQty,
			MaxOrderAmount: req.MaxOrderAmount,
			AllowedSymbols: allowed,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "risk_config_updated"})
}

func (s *Server) setStrategyRunning(id string, running bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.strategies[id]
	if state.Config == nil {
		state.Config = map[string]any{}
	}
	state.ID = id
	state.Running = running
	state.UpdatedMS = time.Now().UnixMilli()
	s.strategies[id] = state
}

func (s *Server) setStrategyConfig(id string, cfg map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.strategies[id]
	state.ID = id
	if state.Config == nil {
		state.Config = map[string]any{}
	}
	for k, v := range cfg {
		state.Config[k] = v
	}
	state.UpdatedMS = time.Now().UnixMilli()
	s.strategies[id] = state
}

func parseStrategyAction(path string) (strategyID string, action string, ok bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 5 {
		return "", "", false
	}
	if parts[0] != "api" || parts[1] != "v1" || parts[2] != "strategies" {
		return "", "", false
	}
	return parts[3], parts[4], true
}

func normalizePath(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 5 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "strategies" {
		return "/api/v1/strategies/{id}/" + parts[4]
	}
	return path
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func writeJSON(w http.ResponseWriter, status int, body map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
