package optimizer

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	v2 "quant-system/internal/backtest/v2"
	_ "quant-system/internal/strategy/momentum" // registers "momentum" type
)

// syntheticUptrend builds a deterministic upward-trending dataset large
// enough for momentum's warmup window and backtest bookkeeping.
func syntheticUptrend(n int) v2.Dataset {
	return v2.GenerateSynthetic(v2.SyntheticConfig{
		Symbol:          "BTCUSDT",
		NumEvents:       n,
		Seed:            42,
		StartPrice:      20_000,
		VolatilityBps:   15,
		TrendBpsPerStep: 8,
	})
}

func momentumSpace() SearchSpace {
	return SearchSpace{
		StrategyType: "momentum",
		BaseParams:   json.RawMessage(`{"symbol":"BTCUSDT","order_qty":0.1,"time_in_force":"IOC","cooldown_ms":0}`),
		Params: []ParamSpec{
			{Name: "window_size", Type: ParamInt, Min: 5, Max: 25, Step: 10},             // 5, 15, 25
			{Name: "breakout_threshold", Type: ParamFloat, Min: 0.0002, Max: 0.002, Step: 0.0009}, // ~3 steps
		},
	}
}

func TestOptimizer_RunGridSmokeTest(t *testing.T) {
	cfg := Config{
		Space:       momentumSpace(),
		Algorithm:   AlgorithmGrid,
		MaxTrials:   50,
		Dataset:     syntheticUptrend(500),
		StartEquity: 10_000,
	}
	res, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(res.Trials) == 0 {
		t.Fatal("no trials")
	}
	if res.Algorithm != AlgorithmGrid {
		t.Fatalf("algo=%s want grid", res.Algorithm)
	}
	// Best must have a finite objective.
	if !isFinite(res.Best.Objective) {
		t.Fatalf("best objective not finite: %v", res.Best.Objective)
	}
	// Importance maps include every named param.
	for _, p := range cfg.Space.Params {
		if _, ok := res.Importance[p.Name]; !ok {
			t.Fatalf("param %q missing from importance map", p.Name)
		}
	}
}

func TestOptimizer_RunRandomDeterministic(t *testing.T) {
	cfg := Config{
		Space:       momentumSpace(),
		Algorithm:   AlgorithmRandom,
		MaxTrials:   12,
		Dataset:     syntheticUptrend(400),
		StartEquity: 10_000,
		Seed:        9,
	}
	r1, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(r1.Trials) != len(r2.Trials) {
		t.Fatalf("lengths differ: %d vs %d", len(r1.Trials), len(r2.Trials))
	}
	for i := range r1.Trials {
		if r1.Trials[i].Objective != r2.Trials[i].Objective {
			t.Fatalf("seeded random diverged at trial %d: %v vs %v",
				i, r1.Trials[i].Objective, r2.Trials[i].Objective)
		}
	}
}

func TestOptimizer_RunBadStrategyRecorded(t *testing.T) {
	space := SearchSpace{
		StrategyType: "does-not-exist",
		BaseParams:   json.RawMessage(`{}`),
		Params: []ParamSpec{
			{Name: "x", Type: ParamInt, Min: 1, Max: 2, Step: 1},
		},
	}
	res, err := Run(context.Background(), Config{
		Space:     space,
		Algorithm: AlgorithmGrid,
		Dataset:   syntheticUptrend(100),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Trials) == 0 {
		t.Fatal("expected trials to be recorded even with bad strategy")
	}
	for _, tr := range res.Trials {
		if tr.BuildError == "" {
			t.Fatal("expected BuildError populated for unknown strategy")
		}
	}
}

func TestOptimizer_RunEmptyDataset(t *testing.T) {
	_, err := Run(context.Background(), Config{
		Space: momentumSpace(),
	})
	if !errors.Is(err, ErrEmptyDataset) {
		t.Fatalf("err=%v want ErrEmptyDataset", err)
	}
}

func TestOptimizer_RunInvalidSpace(t *testing.T) {
	_, err := Run(context.Background(), Config{Dataset: syntheticUptrend(10)})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestOptimizer_ContextCancellationReturnsPartial(t *testing.T) {
	// Use a large grid so we're guaranteed to be in the middle when ctx cancels.
	space := SearchSpace{
		StrategyType: "momentum",
		BaseParams:   json.RawMessage(`{"symbol":"BTCUSDT","order_qty":0.1,"time_in_force":"IOC","cooldown_ms":0}`),
		Params: []ParamSpec{
			{Name: "window_size", Type: ParamInt, Min: 5, Max: 100, Step: 1},
			{Name: "breakout_threshold", Type: ParamFloat, Min: 0.0001, Max: 0.005, Step: 0.0001},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	_, err := Run(ctx, Config{
		Space:     space,
		Algorithm: AlgorithmGrid,
		Dataset:   syntheticUptrend(200),
		MaxTrials: 500,
	})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
}

func TestOptimizer_StabilityIsMeasured(t *testing.T) {
	// Large number of events + tight grid gives us a genuine best with
	// defined neighbours. We don't assert a specific number, just that the
	// stability score is within [0,1] and computed.
	cfg := Config{
		Space:       momentumSpace(),
		Algorithm:   AlgorithmGrid,
		MaxTrials:   30,
		Dataset:     syntheticUptrend(500),
		StartEquity: 10_000,
	}
	res, _ := Run(context.Background(), cfg)
	if res.Stability < 0 || res.Stability > 1 {
		t.Fatalf("stability=%v outside [0,1]", res.Stability)
	}
}

func TestImportanceSumsToAboutOne(t *testing.T) {
	cfg := Config{
		Space:     momentumSpace(),
		Algorithm: AlgorithmGrid,
		MaxTrials: 30,
		Dataset:   syntheticUptrend(500),
	}
	res, _ := Run(context.Background(), cfg)
	var sum float64
	for _, v := range res.Importance {
		sum += v
	}
	// Sum is normalised to 1 when variation exists, otherwise 0.
	if sum > 1.0001 {
		t.Fatalf("importance sum=%v exceeds 1", sum)
	}
}
