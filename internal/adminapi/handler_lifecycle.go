package adminapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"quant-system/internal/adminstore"
	"quant-system/internal/lifecycle"
)

// Lifecycle REST handlers.
//
// Routes (all JWT-gated):
//   POST /api/v1/strategies/:id/lifecycle        propose a stage transition
//   GET  /api/v1/strategies/:id/lifecycle        current stage + audit trail
//   GET  /api/v1/strategies/lifecycle-board      Kanban view: all strategies by stage
//   GET  /api/v1/strategies/:id/health           runtime health score vs backtest
//
// The transition handler dispatches through the lifecycle package so
// the rules (what follows what, which guards apply) live in one place
// and are covered by the lifecycle package's unit tests.

// LifecycleTransitionRequest is the body of POST /strategies/:id/lifecycle.
type LifecycleTransitionRequest struct {
	ToStage lifecycle.Stage `json:"to_stage"`
	Reason  string          `json:"reason,omitempty"`
	Actor   string          `json:"actor,omitempty"`
}

// LifecycleTransitionResponse echoes the accepted transition.
type LifecycleTransitionResponse struct {
	StrategyID       string                  `json:"strategy_id"`
	FromStage        lifecycle.Stage         `json:"from_stage"`
	ToStage          lifecycle.Stage         `json:"to_stage"`
	Kind             lifecycle.TransitionKind `json:"kind"`
	TransitionedMS   int64                   `json:"transitioned_ms"`
}

// StrategyLifecycleViewResponse is returned by the GET variant.
type StrategyLifecycleViewResponse struct {
	StrategyID  string                   `json:"strategy_id"`
	Stage       lifecycle.Stage          `json:"stage"`
	Transitions []LifecycleTransitionRow `json:"transitions"`
}

// LifecycleTransitionRow is one audit row.
type LifecycleTransitionRow struct {
	ID             int64  `json:"id"`
	FromStage      string `json:"from_stage"`
	ToStage        string `json:"to_stage"`
	Kind           string `json:"kind"`
	Actor          string `json:"actor"`
	Reason         string `json:"reason,omitempty"`
	TransitionedMS int64  `json:"transitioned_ms"`
}

// LifecycleBoardCard is one strategy in the Kanban view.
type LifecycleBoardCard struct {
	ID                 int64           `json:"id"`
	StrategyID         string          `json:"strategy_id"`
	StrategyType       string          `json:"strategy_type"`
	Stage              lifecycle.Stage `json:"stage"`
	Status             string          `json:"status"`
	Config             json.RawMessage `json:"config,omitempty"`
	UpdatedMS          int64           `json:"updated_ms"`
	LastTransitionMS   int64           `json:"last_transition_ms,omitempty"`
}

// LifecycleBoardResponse groups cards by stage for the UI.
type LifecycleBoardResponse struct {
	Stages    []lifecycle.Stage                       `json:"stages"`
	ByStage   map[lifecycle.Stage][]LifecycleBoardCard `json:"by_stage"`
	TotalCount int                                     `json:"total_count"`
}

// HealthResponse reports the rolling quality of a live strategy.
// For the MVP we fill what we can from the backtest store and leave a
// clear "not yet implemented" flag on the fields that require a fills
// feed (real PnL, rolling live Sharpe). When P6/P7's fills pipeline
// lands these fields will be populated without changing the shape.
type HealthResponse struct {
	StrategyID         string  `json:"strategy_id"`
	Stage              lifecycle.Stage `json:"stage"`
	BestBacktestSharpe float64 `json:"best_backtest_sharpe"`
	ShadowDurationMS   int64   `json:"shadow_duration_ms"`
	ShadowVirtualPnL   float64 `json:"shadow_virtual_pnl"`
	CanaryDurationMS   int64   `json:"canary_duration_ms"`
	CanaryLiveSharpe   float64 `json:"canary_live_sharpe"`
	SharpeDrift        float64 `json:"sharpe_drift"`
	Message            string  `json:"message,omitempty"`
}

// HandleProposeStrategyLifecycle implements POST /strategies/:id/lifecycle.
func (s *Server) HandleProposeStrategyLifecycle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	id, err := parseTrailingID(r.URL.Path, "/api/v1/strategies/", "/lifecycle")
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	cfg, ok, err := s.store.GetStrategyConfig(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if !ok {
		s.writeError(w, http.StatusNotFound, "not_found", "strategy not found")
		return
	}

	var req LifecycleTransitionRequest
	if err := s.readJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if !req.ToStage.Valid() {
		s.writeError(w, http.StatusBadRequest, "bad_request", "to_stage is invalid")
		return
	}

	fromStageStr, err := s.store.GetLifecycleStage(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if fromStageStr == "" {
		fromStageStr = string(lifecycle.StageDraft)
	}
	from := lifecycle.Stage(fromStageStr)

	kind, err := lifecycle.Transition(r.Context(), cfg.StrategyID, from, req.ToStage, s.evidenceSource(), s.lifecyclePolicy())
	if err != nil {
		if errors.Is(err, lifecycle.ErrGuardFailed) {
			s.writeError(w, http.StatusUnprocessableEntity, "guard_failed", err.Error())
			return
		}
		if errors.Is(err, lifecycle.ErrIllegalTransition) ||
			errors.Is(err, lifecycle.ErrTerminal) ||
			errors.Is(err, lifecycle.ErrNoChange) ||
			errors.Is(err, lifecycle.ErrUnknownStage) {
			s.writeError(w, http.StatusConflict, "illegal_transition", err.Error())
			return
		}
		s.writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	if req.Actor == "" {
		if uid, ok := userIDFromContext(r.Context()); ok {
			req.Actor = uid
		} else {
			req.Actor = "anonymous"
		}
	}
	now := time.Now().UTC().UnixMilli()
	if err := s.store.SetLifecycleStage(r.Context(), adminstore.LifecycleTransition{
		StrategyID:     cfg.StrategyID,
		FromStage:      string(from),
		ToStage:        string(req.ToStage),
		Kind:           string(kind),
		Actor:          req.Actor,
		Reason:         req.Reason,
		TransitionedMS: now,
	}); err != nil {
		s.writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, LifecycleTransitionResponse{
		StrategyID:     cfg.StrategyID,
		FromStage:      from,
		ToStage:        req.ToStage,
		Kind:           kind,
		TransitionedMS: now,
	})
}

// HandleGetStrategyLifecycle implements GET /strategies/:id/lifecycle.
func (s *Server) HandleGetStrategyLifecycle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET required")
		return
	}
	id, err := parseTrailingID(r.URL.Path, "/api/v1/strategies/", "/lifecycle")
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	cfg, ok, err := s.store.GetStrategyConfig(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if !ok {
		s.writeError(w, http.StatusNotFound, "not_found", "strategy not found")
		return
	}

	stageStr, err := s.store.GetLifecycleStage(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if stageStr == "" {
		stageStr = string(lifecycle.StageDraft)
	}

	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 && v <= 500 {
			limit = v
		}
	}
	trans, err := s.store.ListLifecycleTransitions(r.Context(), cfg.StrategyID, limit)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	rows := make([]LifecycleTransitionRow, 0, len(trans))
	for _, t := range trans {
		rows = append(rows, LifecycleTransitionRow{
			ID:             t.ID,
			FromStage:      t.FromStage,
			ToStage:        t.ToStage,
			Kind:           t.Kind,
			Actor:          t.Actor,
			Reason:         t.Reason,
			TransitionedMS: t.TransitionedMS,
		})
	}
	s.writeJSON(w, http.StatusOK, StrategyLifecycleViewResponse{
		StrategyID:  cfg.StrategyID,
		Stage:       lifecycle.Stage(stageStr),
		Transitions: rows,
	})
}

// HandleLifecycleBoard implements GET /strategies/lifecycle-board.
func (s *Server) HandleLifecycleBoard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET required")
		return
	}
	rows, err := s.store.ListLifecycleBoard(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	resp := LifecycleBoardResponse{
		Stages:  lifecycle.AllStages(),
		ByStage: make(map[lifecycle.Stage][]LifecycleBoardCard, 6),
	}
	for _, stage := range lifecycle.AllStages() {
		resp.ByStage[stage] = nil
	}
	for _, row := range rows {
		card := LifecycleBoardCard{
			ID:           row.ID,
			StrategyID:   row.StrategyID,
			StrategyType: row.StrategyType,
			Stage:        lifecycle.Stage(row.Stage),
			Status:       row.Status,
			UpdatedMS:    row.UpdatedMS,
		}
		if row.ConfigJSON != "" {
			card.Config = json.RawMessage(row.ConfigJSON)
		}
		if row.LastTransition.Valid {
			card.LastTransitionMS = row.LastTransition.Int64
		}
		resp.ByStage[card.Stage] = append(resp.ByStage[card.Stage], card)
		resp.TotalCount++
	}
	s.writeJSON(w, http.StatusOK, resp)
}

// HandleStrategyHealth implements GET /strategies/:id/health. The MVP
// sources most numbers from the same evidence adapter the transition
// guards use, which today means backtest Sharpe is populated and live
// / shadow / canary metrics are best-effort.
func (s *Server) HandleStrategyHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET required")
		return
	}
	id, err := parseTrailingID(r.URL.Path, "/api/v1/strategies/", "/health")
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	cfg, ok, err := s.store.GetStrategyConfig(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if !ok {
		s.writeError(w, http.StatusNotFound, "not_found", "strategy not found")
		return
	}
	stageStr, _ := s.store.GetLifecycleStage(r.Context(), id)
	if stageStr == "" {
		stageStr = string(lifecycle.StageDraft)
	}

	src := s.evidenceSource()
	ctx := r.Context()
	sharpe, _ := src.BestBacktestSharpe(ctx, cfg.StrategyID)
	shadowDur, _ := src.ShadowRunDuration(ctx, cfg.StrategyID)
	shadowPnL, _ := src.ShadowVirtualPnL(ctx, cfg.StrategyID)
	canaryDur, _ := src.CanaryRunDuration(ctx, cfg.StrategyID)
	canarySharpe, _ := src.CanaryLiveSharpe(ctx, cfg.StrategyID)

	drift := 0.0
	if canarySharpe > 0 && sharpe > 0 {
		drift = sharpe - canarySharpe
	}
	msg := ""
	if shadowDur == 0 && canaryDur == 0 {
		msg = "shadow / canary metrics not yet wired to live fills feed; values are placeholders"
	}

	s.writeJSON(w, http.StatusOK, HealthResponse{
		StrategyID:         cfg.StrategyID,
		Stage:              lifecycle.Stage(stageStr),
		BestBacktestSharpe: sharpe,
		ShadowDurationMS:   shadowDur.Milliseconds(),
		ShadowVirtualPnL:   shadowPnL,
		CanaryDurationMS:   canaryDur.Milliseconds(),
		CanaryLiveSharpe:   canarySharpe,
		SharpeDrift:        drift,
		Message:            msg,
	})
}

// evidenceSource returns the lifecycle.EvidenceSource the server will
// use for guard checks. The default implementation aggregates over the
// in-memory BacktestStore; a future revision can swap in a ClickHouse-
// or MySQL-backed adapter without touching callers.
func (s *Server) evidenceSource() lifecycle.EvidenceSource {
	return &serverEvidence{server: s}
}

// lifecyclePolicy returns the current promotion thresholds. For the MVP
// the default values are used everywhere; per-strategy overrides are a
// follow-up, stored in a yet-to-be-added strategy_lifecycle_policies
// table.
func (s *Server) lifecyclePolicy() lifecycle.Policy {
	return lifecycle.DefaultPolicy()
}

// serverEvidence adapts the admin-api's in-process state into the
// lifecycle.EvidenceSource interface. BestBacktestSharpe walks the
// BacktestStore for the highest Sharpe among done runs whose request
// targets the given strategy_type. Shadow/canary metrics are stubbed
// to zero until the fills feed lands — the Message field on the health
// response tells operators they're in placeholder mode.
type serverEvidence struct {
	server *Server
}

func (e *serverEvidence) BestBacktestSharpe(_ context.Context, strategyID string) (float64, error) {
	if e.server.backtests == nil {
		return 0, nil
	}
	best := 0.0
	for _, rec := range e.server.backtests.List(0) {
		if rec.Status != BacktestStatusDone || rec.Result == nil {
			continue
		}
		// Match by strategy_type since the backtest API does not carry
		// strategy_id; operators should keep names consistent.
		if rec.Request.StrategyType == "" {
			continue
		}
		// Accept any strategy whose strategy_id starts with or equals
		// the request's StrategyType, which is the convention used by
		// the built-in strategies (e.g. momentum-breakout for "momentum").
		if !strategyMatchesType(strategyID, rec.Request.StrategyType) {
			continue
		}
		s := rec.Result.Metrics.Sharpe
		if s > best {
			best = s
		}
	}
	return best, nil
}

func (e *serverEvidence) ShadowRunDuration(context.Context, string) (time.Duration, error) {
	return 0, nil
}
func (e *serverEvidence) ShadowVirtualPnL(context.Context, string) (float64, error) {
	return 0, nil
}
func (e *serverEvidence) CanaryRunDuration(context.Context, string) (time.Duration, error) {
	return 0, nil
}
func (e *serverEvidence) CanaryLiveSharpe(context.Context, string) (float64, error) {
	return 0, nil
}

// strategyMatchesType is a best-effort pairing between a strategy_id
// (e.g. "momentum-btcusdt") and a strategy_type (e.g. "momentum") used
// by the backtest request. Returns true if either is a prefix of the
// other or the strategy_id contains the strategy_type.
func strategyMatchesType(strategyID, strategyType string) bool {
	if strategyID == "" || strategyType == "" {
		return false
	}
	if strategyID == strategyType {
		return true
	}
	// strategy_id often looks like "<type>-<symbol>" or "<type>-breakout".
	return len(strategyID) >= len(strategyType) &&
		(strategyID[:len(strategyType)] == strategyType ||
			containsSubstring(strategyID, strategyType))
}

// containsSubstring is a tiny helper that avoids importing strings
// just for one call.
func containsSubstring(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	if len(needle) > len(haystack) {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// Guard-rail: the format strings above use fmt.Errorf via the package
// elsewhere; silence the import if a future refactor trims everything.
var _ = fmt.Sprintf
