package adminapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"quant-system/internal/adminstore"
)

// strategyResponse is the JSON representation of a strategy config.
type strategyResponse struct {
	ID           int64           `json:"id"`
	StrategyID   string          `json:"strategy_id"`
	StrategyType string          `json:"strategy_type"`
	ExchangeID   int64           `json:"exchange_id"`
	APIKeyID     int64           `json:"api_key_id"`
	Config       json.RawMessage `json:"config"`
	Status       string          `json:"status"`
	CreatedMS    int64           `json:"created_ms"`
	UpdatedMS    int64           `json:"updated_ms"`
	DockerHint   string          `json:"docker_hint,omitempty"`
}

// toStrategyResponse converts an adminstore.StrategyConfig to a JSON-friendly response.
func toStrategyResponse(cfg adminstore.StrategyConfig) strategyResponse {
	raw := json.RawMessage(cfg.ConfigJSON)
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	return strategyResponse{
		ID:           cfg.ID,
		StrategyID:   cfg.StrategyID,
		StrategyType: cfg.StrategyType,
		ExchangeID:   cfg.ExchangeID,
		APIKeyID:     cfg.APIKeyID,
		Config:       raw,
		Status:       cfg.Status,
		CreatedMS:    cfg.CreatedMS,
		UpdatedMS:    cfg.UpdatedMS,
	}
}

// dockerComposeHint generates a docker-compose snippet for the given strategy.
func dockerComposeHint(cfg adminstore.StrategyConfig) string {
	return fmt.Sprintf(`To run this strategy, add to docker-compose.yml:

  strategy-%s:
    build: .
    entrypoint: ["/app/strategy-runner"]
    environment:
      STRATEGY_CONFIG_ID: "%d"
      MYSQL_DSN: ${MYSQL_DSN}
      NATS_URL: ${NATS_URL}
    depends_on:
      mysql: { condition: service_healthy }
      nats: { condition: service_healthy }
    networks:
      - quant-net

Then run: docker compose up -d strategy-%s`, cfg.StrategyID, cfg.ID, cfg.StrategyID)
}

// HandleListStrategies handles GET /api/v1/strategies.
func (s *Server) HandleListStrategies(w http.ResponseWriter, r *http.Request) {
	cfgs, err := s.store.ListStrategyConfigs(r.Context())
	if err != nil {
		s.logger.Error("list strategies failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to list strategies")
		return
	}

	out := make([]strategyResponse, len(cfgs))
	for i, cfg := range cfgs {
		out[i] = toStrategyResponse(cfg)
	}
	s.writeJSON(w, http.StatusOK, out)
}

// HandleCreateStrategy handles POST /api/v1/strategies.
func (s *Server) HandleCreateStrategy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StrategyID   string          `json:"strategy_id"`
		StrategyType string          `json:"strategy_type"`
		ExchangeID   int64           `json:"exchange_id"`
		APIKeyID     int64           `json:"api_key_id"`
		Config       json.RawMessage `json:"config"`
	}
	if err := s.readJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	if req.StrategyID == "" {
		s.writeError(w, http.StatusBadRequest, "bad_request", "strategy_id is required")
		return
	}
	if req.StrategyType == "" {
		s.writeError(w, http.StatusBadRequest, "bad_request", "strategy_type is required")
		return
	}

	configJSON := "{}"
	if len(req.Config) > 0 {
		configJSON = string(req.Config)
	}

	cfg := adminstore.StrategyConfig{
		StrategyID:   req.StrategyID,
		StrategyType: req.StrategyType,
		ExchangeID:   req.ExchangeID,
		APIKeyID:     req.APIKeyID,
		ConfigJSON:   configJSON,
		Status:       "stopped",
	}

	id, err := s.store.CreateStrategyConfig(r.Context(), cfg)
	if err != nil {
		s.logger.Error("create strategy failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to create strategy")
		return
	}

	cfg.ID = id
	created, ok, err := s.store.GetStrategyConfig(r.Context(), id)
	if err != nil || !ok {
		s.logger.Error("get created strategy failed", "error", err)
		// Return minimal response with the ID.
		cfg.ID = id
		resp := toStrategyResponse(cfg)
		resp.DockerHint = dockerComposeHint(cfg)
		s.writeJSON(w, http.StatusCreated, resp)
		return
	}
	resp := toStrategyResponse(created)
	resp.DockerHint = dockerComposeHint(created)
	s.writeJSON(w, http.StatusCreated, resp)
}

// HandleGetStrategy handles GET /api/v1/strategies/{id}.
func (s *Server) HandleGetStrategy(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(s.pathParam(r, "id"), 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", "invalid strategy id")
		return
	}

	cfg, ok, err := s.store.GetStrategyConfig(r.Context(), id)
	if err != nil {
		s.logger.Error("get strategy failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to get strategy")
		return
	}
	if !ok {
		s.writeError(w, http.StatusNotFound, "not_found", "strategy not found")
		return
	}
	resp := toStrategyResponse(cfg)
	resp.DockerHint = dockerComposeHint(cfg)
	s.writeJSON(w, http.StatusOK, resp)
}

// HandleUpdateStrategy handles PUT /api/v1/strategies/{id}.
func (s *Server) HandleUpdateStrategy(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(s.pathParam(r, "id"), 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", "invalid strategy id")
		return
	}

	existing, ok, err := s.store.GetStrategyConfig(r.Context(), id)
	if err != nil {
		s.logger.Error("get strategy for update failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to get strategy")
		return
	}
	if !ok {
		s.writeError(w, http.StatusNotFound, "not_found", "strategy not found")
		return
	}

	var req struct {
		ExchangeID *int64          `json:"exchange_id"`
		APIKeyID   *int64          `json:"api_key_id"`
		Config     json.RawMessage `json:"config"`
	}
	if err := s.readJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	if req.ExchangeID != nil {
		existing.ExchangeID = *req.ExchangeID
	}
	if req.APIKeyID != nil {
		existing.APIKeyID = *req.APIKeyID
	}
	if len(req.Config) > 0 {
		existing.ConfigJSON = string(req.Config)
	}

	if err := s.store.UpdateStrategyConfig(r.Context(), existing); err != nil {
		s.logger.Error("update strategy failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to update strategy")
		return
	}

	updated, ok, err := s.store.GetStrategyConfig(r.Context(), id)
	if err != nil || !ok {
		s.writeJSON(w, http.StatusOK, toStrategyResponse(existing))
		return
	}
	s.writeJSON(w, http.StatusOK, toStrategyResponse(updated))
}

// HandleDeleteStrategy handles DELETE /api/v1/strategies/{id}.
func (s *Server) HandleDeleteStrategy(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(s.pathParam(r, "id"), 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", "invalid strategy id")
		return
	}

	_, ok, err := s.store.GetStrategyConfig(r.Context(), id)
	if err != nil {
		s.logger.Error("get strategy for delete failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to get strategy")
		return
	}
	if !ok {
		s.writeError(w, http.StatusNotFound, "not_found", "strategy not found")
		return
	}

	if err := s.store.DeleteStrategyConfig(r.Context(), id); err != nil {
		s.logger.Error("delete strategy failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to delete strategy")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// HandleStartStrategy handles POST /api/v1/strategies/{id}/start.
func (s *Server) HandleStartStrategy(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(s.pathParam(r, "id"), 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", "invalid strategy id")
		return
	}

	cfg, ok, err := s.store.GetStrategyConfig(r.Context(), id)
	if err != nil {
		s.logger.Error("get strategy for start failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to get strategy")
		return
	}
	if !ok {
		s.writeError(w, http.StatusNotFound, "not_found", "strategy not found")
		return
	}
	if cfg.Status == "running" {
		s.writeError(w, http.StatusConflict, "conflict", "strategy is already running")
		return
	}

	if err := s.store.UpdateStrategyStatus(r.Context(), id, "running"); err != nil {
		s.logger.Error("start strategy failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to start strategy")
		return
	}

	cfg.Status = "running"
	s.writeJSON(w, http.StatusOK, toStrategyResponse(cfg))
}

// HandleStopStrategy handles POST /api/v1/strategies/{id}/stop.
func (s *Server) HandleStopStrategy(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(s.pathParam(r, "id"), 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", "invalid strategy id")
		return
	}

	cfg, ok, err := s.store.GetStrategyConfig(r.Context(), id)
	if err != nil {
		s.logger.Error("get strategy for stop failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to get strategy")
		return
	}
	if !ok {
		s.writeError(w, http.StatusNotFound, "not_found", "strategy not found")
		return
	}
	if cfg.Status == "stopped" {
		s.writeError(w, http.StatusConflict, "conflict", "strategy is already stopped")
		return
	}

	if err := s.store.UpdateStrategyStatus(r.Context(), id, "stopped"); err != nil {
		s.logger.Error("stop strategy failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to stop strategy")
		return
	}

	cfg.Status = "stopped"
	s.writeJSON(w, http.StatusOK, toStrategyResponse(cfg))
}

// HandleStopAll handles POST /api/v1/strategies/stop-all.
// Sets ALL running strategies to "stopped" and sends Feishu notification.
func (s *Server) HandleStopAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}

	cfgs, err := s.store.ListStrategyConfigs(r.Context())
	if err != nil {
		s.logger.Error("stop-all: list strategies failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to list strategies")
		return
	}

	stoppedCount := 0
	for _, cfg := range cfgs {
		if cfg.Status == "running" {
			if err := s.store.UpdateStrategyStatus(r.Context(), cfg.ID, "stopped"); err != nil {
				s.logger.Error("stop-all: failed to stop strategy", "id", cfg.ID, "error", err)
				continue
			}
			stoppedCount++
		}
	}

	if s.feishu != nil {
		alertContent := fmt.Sprintf("已停止 %d 个策略", stoppedCount)
		if err := s.feishu.SendAlert(r.Context(), "critical", "紧急停止所有策略", alertContent); err != nil {
			s.logger.Error("stop-all: feishu notification failed", "error", err)
		}
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"stopped_count": stoppedCount,
		"message":       "all strategies stopped",
	})
}
