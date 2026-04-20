package adminapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	v2 "quant-system/internal/backtest/v2"
	"quant-system/internal/optimizer"
)

// Parameter-optimisation REST handlers.
//
// Routes exposed (all JWT-gated):
//   POST /api/v1/optimizations        create + run, synchronous for MVP
//   GET  /api/v1/optimizations/{id}   fetch record + result
//   GET  /api/v1/optimizations        list recent runs, newest first
//
// The MVP runs optimisations synchronously because even a 500-trial
// grid search over a synthetic 500-bar dataset takes <10s on a laptop.
// When real historical data pushes durations beyond 30s we'll spin this
// into a goroutine + job queue without changing the handler contract —
// Status transitions already model queued → running → done/failed.

// HandleCreateOptimization runs one optimisation job end-to-end.
func (s *Server) HandleCreateOptimization(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	var req OptimizationRequest
	if err := s.readJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := validateOptimizationRequest(req); err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	rec := &OptimizationRecord{
		ID:        newOptimizationID(),
		Status:    OptimizationStatusQueued,
		Request:   req,
		CreatedAt: time.Now().UTC(),
	}
	s.optimizations.Put(rec)

	s.runOptimization(r.Context(), rec.ID, req)

	final, ok := s.optimizations.Get(rec.ID)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "internal_error", "record missing after run")
		return
	}
	s.writeJSON(w, http.StatusCreated, final)
}

// HandleGetOptimization returns one record by ID.
func (s *Server) HandleGetOptimization(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET required")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/optimizations/")
	if idx := strings.Index(id, "/"); idx != -1 {
		id = id[:idx]
	}
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "bad_request", "missing optimization id")
		return
	}
	rec, ok := s.optimizations.Get(id)
	if !ok {
		s.writeError(w, http.StatusNotFound, "not_found", "optimization not found")
		return
	}
	s.writeJSON(w, http.StatusOK, rec)
}

// HandleListOptimizations lists recent jobs newest-first.
func (s *Server) HandleListOptimizations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET required")
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			if v > 200 {
				v = 200
			}
			limit = v
		}
	}
	items := s.optimizations.List(limit)
	s.writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"count": len(items),
	})
}

// runOptimization executes the search synchronously. Status transitions
// match what an async queue would emit, so switching to a goroutine is a
// one-line change later.
func (s *Server) runOptimization(ctx context.Context, id string, req OptimizationRequest) {
	now := time.Now().UTC()
	s.optimizations.Update(id, func(rec *OptimizationRecord) {
		rec.Status = OptimizationStatusRunning
		rec.StartedAt = now
	})

	fail := func(msg string) {
		s.optimizations.Update(id, func(rec *OptimizationRecord) {
			rec.Status = OptimizationStatusFailed
			rec.Error = msg
			rec.FinishedAt = time.Now().UTC()
		})
	}

	// Build dataset using the same dispatcher as the backtest endpoint so
	// operators can point at the same synthetic / clickhouse source.
	ds, err := s.buildDataset(ctx, req.Dataset)
	if err != nil {
		fail(fmt.Sprintf("dataset: %v", err))
		return
	}
	if len(ds.Events) == 0 {
		fail("dataset produced 0 events")
		return
	}

	// Convert payload params to internal specs.
	params := make([]optimizer.ParamSpec, 0, len(req.Params))
	for _, p := range req.Params {
		params = append(params, p.toSpec())
	}

	cfg := optimizer.Config{
		Space: optimizer.SearchSpace{
			StrategyType: req.StrategyType,
			BaseParams:   req.BaseParams,
			Params:       params,
		},
		Algorithm:   req.Algorithm,
		MaxTrials:   req.MaxTrials,
		Dataset:     ds,
		AccountID:   req.AccountID,
		StartEquity: req.StartEquity,
		Risk:        toRiskConfig(req.Risk),
		Matcher: v2.SimMatcherConfig{
			SlippageBps: req.SlippageBps,
			TakerFeeBps: req.FeeBps,
		},
		Objective: optimizer.NewObjective(req.Objective),
		Seed:      req.Seed,
	}

	result, err := optimizer.Run(ctx, cfg)
	if err != nil {
		fail(fmt.Sprintf("run: %v", err))
		return
	}
	// Re-stamp the Objective preset into the result so the UI can echo
	// the chosen scoring back to the operator.
	if req.Objective != "" {
		result.Objective = req.Objective
	} else {
		result.Objective = optimizer.ObjectiveSharpePenaltyDD
	}

	s.optimizations.Update(id, func(rec *OptimizationRecord) {
		rec.Status = OptimizationStatusDone
		rec.Result = result
		rec.FinishedAt = time.Now().UTC()
	})
}

func validateOptimizationRequest(req OptimizationRequest) error {
	if strings.TrimSpace(req.StrategyType) == "" {
		return errors.New("strategy_type is required")
	}
	if len(req.Params) == 0 {
		return errors.New("params must contain at least one entry")
	}
	if req.Dataset.NumEvents <= 0 {
		return errors.New("dataset.num_events must be > 0")
	}
	if req.Dataset.NumEvents > 100_000 {
		return errors.New("dataset.num_events capped at 100000 for MVP")
	}
	if strings.TrimSpace(req.Dataset.Symbol) == "" {
		return errors.New("dataset.symbol is required")
	}
	if req.MaxTrials < 0 {
		return errors.New("max_trials must be >= 0")
	}
	if req.MaxTrials > 5_000 {
		return errors.New("max_trials capped at 5000 for MVP")
	}
	if req.StartEquity < 0 {
		return errors.New("start_equity must be >= 0")
	}
	if req.SlippageBps < 0 || req.SlippageBps > 1_000 {
		return errors.New("slippage_bps must be in [0, 1000]")
	}
	if req.FeeBps < 0 || req.FeeBps > 1_000 {
		return errors.New("fee_bps must be in [0, 1000]")
	}
	return nil
}
