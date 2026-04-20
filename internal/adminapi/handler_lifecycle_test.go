package adminapi

import (
	"context"
	"testing"
	"time"

	v2 "quant-system/internal/backtest/v2"
	"quant-system/internal/lifecycle"
)

// The lifecycle HTTP handlers need a real *adminstore.Store for the
// GET/POST end-to-end round-trip, so those are deferred to the e2e
// suite. Here we unit-test the serverEvidence adapter which owns the
// translation between in-memory backtest records and the numbers the
// lifecycle Guards compare against — it's the piece most likely to
// drift silently as fields evolve.

func seedBacktest(t *testing.T, s *Server, strategyType string, sharpe float64) {
	t.Helper()
	if s.backtests == nil {
		s.backtests = NewBacktestStore(10)
	}
	rec := &BacktestRecord{
		ID:        "bt_test_" + strategyType,
		Status:    BacktestStatusDone,
		CreatedAt: time.Now(),
		Request: BacktestRequest{
			StrategyType: strategyType,
		},
		Result: &v2.Result{
			Metrics: v2.Metrics{Sharpe: sharpe, NumTrades: 10},
		},
	}
	s.backtests.Put(rec)
}

func TestEvidenceSource_BestBacktestSharpe_PicksHighest(t *testing.T) {
	srv := &Server{}
	seedBacktest(t, srv, "momentum", 0.9)
	seedBacktest(t, srv, "momentum", 1.8)
	seedBacktest(t, srv, "template", 2.5) // unrelated type — should be skipped

	src := srv.evidenceSource()
	got, err := src.BestBacktestSharpe(context.Background(), "momentum-btcusdt")
	if err != nil {
		t.Fatal(err)
	}
	if got != 1.8 {
		t.Fatalf("got=%v want 1.8 (best among momentum-typed runs)", got)
	}
}

func TestEvidenceSource_BestBacktestSharpe_ZeroWhenNoMatch(t *testing.T) {
	srv := &Server{backtests: NewBacktestStore(10)}
	src := srv.evidenceSource()
	got, err := src.BestBacktestSharpe(context.Background(), "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Fatalf("got=%v want 0 (no matching backtests)", got)
	}
}

func TestEvidenceSource_GuardIntegration(t *testing.T) {
	srv := &Server{}
	seedBacktest(t, srv, "momentum", 1.5) // passes default 0.8 threshold

	kind, err := lifecycle.Transition(
		context.Background(),
		"momentum-btcusdt",
		lifecycle.StageDraft, lifecycle.StageBacktested,
		srv.evidenceSource(),
		lifecycle.DefaultPolicy(),
	)
	if err != nil {
		t.Fatalf("transition via server evidence failed: %v", err)
	}
	if kind != lifecycle.TransitionPromote {
		t.Fatalf("kind=%s want promote", kind)
	}
}

func TestStrategyMatchesType(t *testing.T) {
	cases := []struct {
		id, ty string
		want   bool
	}{
		{"momentum-btcusdt", "momentum", true},
		{"momentum", "momentum", true},
		{"my-momentum-custom", "momentum", true},
		{"template-mvp", "momentum", false},
		{"", "momentum", false},
		{"momentum", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.id+"/"+tc.ty, func(t *testing.T) {
			if got := strategyMatchesType(tc.id, tc.ty); got != tc.want {
				t.Fatalf("got=%v want=%v", got, tc.want)
			}
		})
	}
}
