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
	"quant-system/internal/risk"
	"quant-system/internal/strategy"
	"quant-system/pkg/contracts"
)

// HandleCreateBacktest handles POST /api/v1/backtests.
// The request body is BacktestRequest; the response contains the created
// record (including metrics when Status=="done"). The MVP runs the backtest
// synchronously, so successful calls return 201 with a fully-populated Result.
func (s *Server) HandleCreateBacktest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	var req BacktestRequest
	if err := s.readJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := validateBacktestRequest(req); err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	rec := &BacktestRecord{
		ID:        newBacktestID(),
		Status:    BacktestStatusQueued,
		Request:   req,
		CreatedAt: time.Now().UTC(),
	}
	s.backtests.Put(rec)

	s.runBacktest(r.Context(), rec.ID, req)

	// Reload the record after execution so the caller sees the final state.
	final, ok := s.backtests.Get(rec.ID)
	if !ok {
		// Should not happen; store retention is bounded but we just added this.
		s.writeError(w, http.StatusInternalServerError, "internal_error", "record missing after run")
		return
	}
	s.writeJSON(w, http.StatusCreated, final)
}

// HandleGetBacktest handles GET /api/v1/backtests/{id}. The response includes
// the Result (equity curve, metrics, trade log) when Status=="done".
func (s *Server) HandleGetBacktest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET required")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/backtests/")
	if idx := strings.Index(id, "/"); idx != -1 {
		id = id[:idx]
	}
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "bad_request", "missing backtest id")
		return
	}
	rec, ok := s.backtests.Get(id)
	if !ok {
		s.writeError(w, http.StatusNotFound, "not_found", "backtest not found")
		return
	}
	s.writeJSON(w, http.StatusOK, rec)
}

// HandleListBacktests handles GET /api/v1/backtests. A ?limit=N query
// parameter caps the returned list; default 50, max 500.
func (s *Server) HandleListBacktests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET required")
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			if v > 500 {
				v = 500
			}
			limit = v
		}
	}
	items := s.backtests.List(limit)
	s.writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"count": len(items),
	})
}

// runBacktest builds the strategy from the registry, generates the dataset,
// invokes v2.Run, and updates the store record with the outcome. It is
// synchronous for the MVP; wrapping this in a goroutine + status polling
// is a one-line change when long-running ClickHouse-backed runs land.
func (s *Server) runBacktest(ctx context.Context, id string, req BacktestRequest) {
	now := time.Now().UTC()
	s.backtests.Update(id, func(rec *BacktestRecord) {
		rec.Status = BacktestStatusRunning
		rec.StartedAt = now
	})

	fail := func(msg string) {
		s.backtests.Update(id, func(rec *BacktestRecord) {
			rec.Status = BacktestStatusFailed
			rec.Error = msg
			rec.FinishedAt = time.Now().UTC()
		})
	}

	ctor, ok := strategy.Lookup(req.StrategyType)
	if !ok {
		fail(fmt.Sprintf("unknown strategy_type %q", req.StrategyType))
		return
	}
	strat, err := ctor(req.StrategyParams)
	if err != nil {
		fail(fmt.Sprintf("build strategy: %v", err))
		return
	}

	ds := buildDataset(req.Dataset)
	if len(ds.Events) == 0 {
		fail("dataset produced 0 events")
		return
	}

	cfg := v2.Config{
		AccountID:   req.AccountID,
		Strategy:    strat,
		Dataset:     ds,
		StartEquity: req.StartEquity,
		Risk:        toRiskConfig(req.Risk),
		Matcher: v2.SimMatcherConfig{
			SlippageBps: req.SlippageBps,
			TakerFeeBps: req.FeeBps,
		},
	}

	result, err := v2.Run(ctx, cfg)
	if err != nil {
		fail(fmt.Sprintf("run: %v", err))
		return
	}
	s.backtests.Update(id, func(rec *BacktestRecord) {
		rec.Status = BacktestStatusDone
		rec.Result = &result
		rec.FinishedAt = time.Now().UTC()
	})
}

// buildDataset dispatches on Source. Only "synthetic" is supported today.
func buildDataset(spec BacktestDatasetSpec) v2.Dataset {
	source := strings.ToLower(strings.TrimSpace(spec.Source))
	if source == "" {
		source = "synthetic"
	}
	switch source {
	case "synthetic":
		return v2.GenerateSynthetic(v2.SyntheticConfig{
			Symbol:          spec.Symbol,
			NumEvents:       spec.NumEvents,
			Seed:            spec.Seed,
			StartPrice:      spec.StartPrice,
			VolatilityBps:   spec.VolatilityBps,
			TrendBpsPerStep: spec.TrendBpsPerStep,
			SpreadBps:       spec.SpreadBps,
			StepMS:          spec.StepMS,
			StartTSMS:       spec.StartTSMS,
		})
	default:
		return v2.Dataset{}
	}
}

func toRiskConfig(spec BacktestRiskSpec) risk.Config {
	allowed := make(map[string]struct{}, len(spec.AllowedSymbols))
	for _, sym := range spec.AllowedSymbols {
		sym = strings.ToUpper(strings.TrimSpace(sym))
		if sym != "" {
			allowed[sym] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		allowed = nil
	}
	return risk.Config{
		MaxOrderQty:    spec.MaxOrderQty,
		MaxOrderAmount: spec.MaxOrderAmount,
		AllowedSymbols: allowed,
	}
}

func validateBacktestRequest(req BacktestRequest) error {
	if strings.TrimSpace(req.StrategyType) == "" {
		return errors.New("strategy_type is required")
	}
	if len(req.StrategyParams) == 0 {
		return errors.New("strategy_params is required")
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

// contracts import retained to anchor the Dataset → contracts.Kline linkage
// for future CSV / ClickHouse dataset sources added here.
var _ = contracts.Kline{}
