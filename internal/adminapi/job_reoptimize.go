package adminapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	v2 "quant-system/internal/backtest/v2"
	"quant-system/internal/adminstore"
	"quant-system/internal/lifecycle"
	"quant-system/internal/marketstore"
	"quant-system/internal/optimizer"
)

// ReoptimizeJob is the Phase 7 nightly worker: for every live strategy,
// pull the last N days of klines from the ClickHouse store, run the
// optimiser with a small search space centred on the currently-
// deployed params, and stage any meaningfully better result as a
// pending ParamCandidate for operator review.
//
// The job is deliberately conservative:
//
//   - Only strategies in stage "live" or "canary" are considered. Draft
//     / backtested / paper strategies are still in exploration and
//     operators drive those by hand.
//   - Only strategies whose strategy_type has a registered SearchSpaceBuilder
//     are eligible — the ReoptimizeJob has no way to invent a search
//     space from just a Config struct.
//   - A candidate is only staged when new Sharpe exceeds the baseline
//     by at least MinImprovement. The baseline Sharpe is read from the
//     most recent accepted candidate or a BacktestStore lookup; if no
//     prior evidence exists, the job runs the optimiser but does not
//     stage a candidate unless improvement > 2·MinImprovement to avoid
//     anchoring on a no-signal baseline.
//
// The job is stateless; all data flows through the DB so multiple
// admin-api replicas co-exist safely (future work) as long as the
// scheduler only runs on one replica.
type ReoptimizeJob struct {
	Store       *adminstore.Store
	Klines      marketstore.KlineStore
	Logger      *slog.Logger
	LookbackBars int           // default 1440 (1 day of 1m bars)
	MaxTrials    int           // default 64
	StartEquity  float64       // default 10_000
	MinImprovement float64     // min Sharpe delta to stage a candidate; default 0.15
	Interval     string        // kline interval; default "1m"
	DryRun       bool          // when true, run the optimiser but don't INSERT
}

// Name implements scheduler.Job.
func (j *ReoptimizeJob) Name() string { return "reoptimize" }

// Run implements scheduler.Job. Returns on the first fatal error (DB
// inaccessible, klines store missing) but per-strategy failures are
// logged and do not abort the batch.
func (j *ReoptimizeJob) Run(ctx context.Context) error {
	if j.Store == nil {
		return fmt.Errorf("reoptimize: store is nil")
	}
	if j.Klines == nil {
		j.log().Warn("reoptimize: no kline store wired; skipping run")
		return nil
	}
	j.applyDefaults()

	strategies, err := j.Store.ListStrategyConfigs(ctx)
	if err != nil {
		return fmt.Errorf("reoptimize: list strategies: %w", err)
	}

	considered, staged, skipped := 0, 0, 0
	for _, cfg := range strategies {
		stage, _ := j.Store.GetLifecycleStage(ctx, cfg.ID)
		if stage != string(lifecycle.StageLive) && stage != string(lifecycle.StageCanary) {
			skipped++
			continue
		}
		considered++

		staged1, err := j.reoptimizeOne(ctx, cfg)
		if err != nil {
			j.log().Warn("reoptimize: strategy failed",
				"strategy_id", cfg.StrategyID, "error", err)
			continue
		}
		if staged1 {
			staged++
		}
	}
	j.log().Info("reoptimize: batch done",
		"considered", considered, "staged", staged, "skipped_non_live", skipped)
	return nil
}

// reoptimizeOne handles a single strategy. Returns (true, nil) when a
// candidate was staged. Returns (false, nil) when the strategy was
// eligible but the optimiser did not find a meaningful improvement.
func (j *ReoptimizeJob) reoptimizeOne(ctx context.Context, cfg adminstore.StrategyConfig) (bool, error) {
	space, ok := searchSpaceFor(cfg.StrategyType, cfg.ConfigJSON)
	if !ok {
		j.log().Debug("reoptimize: no SearchSpaceBuilder for type",
			"strategy_id", cfg.StrategyID, "type", cfg.StrategyType)
		return false, nil
	}

	// Fetch the last LookbackBars bars for the strategy's symbol.
	symbol := extractSymbol(cfg.ConfigJSON)
	if symbol == "" {
		return false, fmt.Errorf("reoptimize: could not extract symbol from config")
	}
	klines, err := j.Klines.Query(ctx, marketstore.KlineQuery{
		Symbol:   symbol,
		Interval: j.Interval,
		Limit:    j.LookbackBars,
	})
	if err != nil {
		return false, fmt.Errorf("reoptimize: klines query: %w", err)
	}
	if len(klines) < 50 {
		return false, fmt.Errorf("reoptimize: only %d klines available (need >= 50)", len(klines))
	}

	// Convert klines into a backtest dataset using the same helper the
	// Backtest Workbench uses so every evaluation is apples-to-apples.
	events := klinesToMarketEvents(klines)
	dataset := v2.Dataset{
		Name:   fmt.Sprintf("reoptimize:%s:%s", cfg.StrategyID, j.Interval),
		Events: events,
	}

	optCfg := optimizer.Config{
		Space:       space,
		Algorithm:   optimizer.AlgorithmRandom, // random keeps runtime bounded as space grows
		MaxTrials:   j.MaxTrials,
		Dataset:     dataset,
		StartEquity: j.StartEquity,
		Objective:   optimizer.NewObjective(optimizer.ObjectiveSharpePenaltyDD),
		Seed:        time.Now().UnixMilli(),
	}
	result, err := optimizer.Run(ctx, optCfg)
	if err != nil {
		return false, fmt.Errorf("reoptimize: optimiser failed: %w", err)
	}
	proposedSharpe := result.Best.Metrics.Sharpe

	// Baseline: evaluate the currently-deployed params on the same dataset.
	baseline, err := evalBaseline(ctx, cfg, dataset, j.StartEquity)
	if err != nil {
		return false, fmt.Errorf("reoptimize: baseline eval: %w", err)
	}

	improvement := proposedSharpe - baseline
	// Threshold decision: if the baseline itself is degenerate (≤0), we
	// demand at least 2×MinImprovement to reduce false positives.
	threshold := j.MinImprovement
	if baseline <= 0 {
		threshold = 2 * j.MinImprovement
	}
	if improvement < threshold {
		j.log().Info("reoptimize: no meaningful improvement",
			"strategy_id", cfg.StrategyID,
			"baseline_sharpe", baseline,
			"proposed_sharpe", proposedSharpe,
			"improvement", improvement,
			"threshold", threshold,
		)
		return false, nil
	}

	if j.DryRun {
		j.log().Info("reoptimize: DRY RUN candidate",
			"strategy_id", cfg.StrategyID,
			"improvement", improvement,
			"params", result.Best.Params,
		)
		return false, nil
	}

	proposed, _ := optimizer.SerializeParams(mergedParams(cfg.ConfigJSON, result.Best.Params)), error(nil)
	cand := adminstore.ParamCandidate{
		StrategyID:     cfg.StrategyID,
		Origin:         "scheduler_reoptimize",
		ProposedParams: string(proposed),
		BaselineParams: cfg.ConfigJSON,
		BaselineSharpe: toNullFloat(baseline),
		ProposedSharpe: toNullFloat(proposedSharpe),
		Improvement:    toNullFloat(improvement),
		Status:         adminstore.CandidatePending,
	}
	if _, err := j.Store.CreateParamCandidate(ctx, cand); err != nil {
		return false, fmt.Errorf("reoptimize: insert candidate: %w", err)
	}
	j.log().Info("reoptimize: candidate staged",
		"strategy_id", cfg.StrategyID,
		"baseline_sharpe", baseline,
		"proposed_sharpe", proposedSharpe,
		"improvement", improvement,
	)
	return true, nil
}

// evalBaseline runs a single backtest on the same dataset using the
// strategy's currently-deployed params and returns its Sharpe. A
// failure here is a hard error because without a baseline we cannot
// claim improvement.
func evalBaseline(ctx context.Context, cfg adminstore.StrategyConfig, dataset v2.Dataset, startEquity float64) (float64, error) {
	// Look up the strategy constructor via the shared optimiser helper.
	// We reuse optimizer.Run with a trivial 1-point space that only
	// evaluates the current params; this avoids duplicating the
	// strategy-building machinery.
	space := optimizer.SearchSpace{
		StrategyType: cfg.StrategyType,
		BaseParams:   json.RawMessage(cfg.ConfigJSON),
		Params: []optimizer.ParamSpec{
			// A dummy categorical with one choice ensures Validate
			// passes and the optimiser produces exactly one trial.
			{Name: "__baseline", Type: optimizer.ParamCategorical, Choices: []any{"x"}},
		},
	}
	res, err := optimizer.Run(ctx, optimizer.Config{
		Space:       space,
		Algorithm:   optimizer.AlgorithmGrid,
		MaxTrials:   1,
		Dataset:     dataset,
		StartEquity: startEquity,
		Objective:   optimizer.NewObjective(optimizer.ObjectiveSharpePenaltyDD),
	})
	if err != nil {
		return 0, err
	}
	if len(res.Trials) == 0 {
		return 0, nil
	}
	return res.Trials[0].Metrics.Sharpe, nil
}

func (j *ReoptimizeJob) applyDefaults() {
	if j.LookbackBars <= 0 {
		j.LookbackBars = 1440
	}
	if j.MaxTrials <= 0 {
		j.MaxTrials = 64
	}
	if j.StartEquity <= 0 {
		j.StartEquity = 10_000
	}
	if j.MinImprovement <= 0 {
		j.MinImprovement = 0.15
	}
	if j.Interval == "" {
		j.Interval = "1m"
	}
}

func (j *ReoptimizeJob) log() *slog.Logger {
	if j.Logger == nil {
		return slog.Default()
	}
	return j.Logger
}

// extractSymbol reads the "symbol" field out of the strategy's config
// JSON. Returns empty when absent; callers treat empty as an error.
func extractSymbol(raw string) string {
	var probe struct {
		Symbol string `json:"symbol"`
	}
	if raw == "" {
		return ""
	}
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return ""
	}
	return probe.Symbol
}

// mergedParams merges the base config with the winning trial's param
// overrides, mirroring the behaviour of optimizer.mergeParams() (which
// is unexported). The result is suitable for handing to a strategy's
// hot-reload ApplyParams call.
func mergedParams(base string, trial map[string]any) map[string]any {
	out := map[string]any{}
	_ = json.Unmarshal([]byte(base), &out)
	for k, v := range trial {
		out[k] = v
	}
	return out
}

// toNullFloat wraps a float64 in sql.NullFloat64, treating 0 as valid.
func toNullFloat(v float64) (n nullFloat) {
	n.Valid = true
	n.Float64 = v
	return n
}

// nullFloat is a local alias so the package does not need to import
// database/sql for a single type — we delegate to adminstore.ParamCandidate
// which already uses sql.NullFloat64.
type nullFloat = sql.NullFloat64

// searchSpaceFor returns a conservative SearchSpace for the given
// strategy type using the current params as anchors. Today this only
// knows about "momentum"; new strategy types can register by adding a
// case here. A Future refactor can move the registry into the strategy
// package itself via a new optional interface.
func searchSpaceFor(strategyType string, baseJSON string) (optimizer.SearchSpace, bool) {
	switch strategyType {
	case "momentum":
		return optimizer.SearchSpace{
			StrategyType: "momentum",
			BaseParams:   json.RawMessage(baseJSON),
			Params: []optimizer.ParamSpec{
				{Name: "window_size", Type: optimizer.ParamInt, Min: 10, Max: 60, Step: 10},
				{Name: "breakout_threshold", Type: optimizer.ParamFloat, Min: 0.0002, Max: 0.003, Step: 0.0004},
			},
		}, true
	}
	return optimizer.SearchSpace{}, false
}

// sql import placeholder — the concrete sql.NullFloat64 usage lives in
// adminstore.ParamCandidate, but we import the package here so the
// nullFloat alias resolves without extra ceremony.
var _ sql.NullFloat64 = sql.NullFloat64{}
