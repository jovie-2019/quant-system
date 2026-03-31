package backtest

import (
	"context"
	"errors"
	"testing"

	momentum "quant-system/internal/strategy/momentum"
	"quant-system/pkg/contracts"
)

func TestNewEngineNilStrategy(t *testing.T) {
	_, err := NewEngine(nil)
	if !errors.Is(err, ErrStrategyNil) {
		t.Fatalf("expected ErrStrategyNil, got %v", err)
	}
}

func TestMomentumBacktestDeterministic(t *testing.T) {
	dataset := momentumDataset()

	first := runMomentumBacktest(t, dataset)
	second := runMomentumBacktest(t, dataset)

	if first.StrategyID != second.StrategyID {
		t.Fatalf("strategy mismatch: first=%s second=%s", first.StrategyID, second.StrategyID)
	}
	if first.Dataset != second.Dataset {
		t.Fatalf("dataset mismatch: first=%s second=%s", first.Dataset, second.Dataset)
	}
	if first.Events != second.Events || first.Intents != second.Intents {
		t.Fatalf("event/intent mismatch: first=%+v second=%+v", first, second)
	}
	if first.BySide["buy"] != 1 || first.BySide["sell"] != 1 {
		t.Fatalf("unexpected side counts: %+v", first.BySide)
	}
	if first.BySymbol["BTC-USDT"] != 2 {
		t.Fatalf("unexpected symbol counts: %+v", first.BySymbol)
	}
	if len(first.Signals) != len(second.Signals) {
		t.Fatalf("signals length mismatch: first=%d second=%d", len(first.Signals), len(second.Signals))
	}
	for i := range first.Signals {
		if first.Signals[i] != second.Signals[i] {
			t.Fatalf("signal mismatch at %d: first=%+v second=%+v", i, first.Signals[i], second.Signals[i])
		}
	}

	if len(first.Signals) != 2 {
		t.Fatalf("expected exactly 2 signals (buy/sell), got %d", len(first.Signals))
	}
	if first.Signals[0].Side != "buy" || first.Signals[1].Side != "sell" {
		t.Fatalf("unexpected signal sides: %+v", first.Signals)
	}
	if first.Signals[0].EventIndex >= first.Signals[1].EventIndex {
		t.Fatalf("expected buy before sell, got %+v", first.Signals)
	}
}

func runMomentumBacktest(t *testing.T, dataset Dataset) Result {
	t.Helper()

	strat := momentum.New(momentum.Config{
		Symbol:            "BTC-USDT",
		WindowSize:        5,
		BreakoutThreshold: 0.001,
		OrderQty:          0.5,
		Cooldown:          0,
	})
	engine, err := NewEngine(strat)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	res, err := engine.Run(context.Background(), dataset)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	return res
}

func momentumDataset() Dataset {
	events := make([]contracts.MarketNormalizedEvent, 0, 12)
	for i := 0; i < 5; i++ {
		events = append(events, contracts.MarketNormalizedEvent{Symbol: "BTC-USDT", LastPX: 60000})
	}
	events = append(events, contracts.MarketNormalizedEvent{Symbol: "BTC-USDT", LastPX: 60100})
	for i := 0; i < 5; i++ {
		events = append(events, contracts.MarketNormalizedEvent{Symbol: "BTC-USDT", LastPX: 60100})
	}
	events = append(events, contracts.MarketNormalizedEvent{Symbol: "BTC-USDT", LastPX: 59900})

	return Dataset{
		Name:   "momentum_buy_sell_roundtrip",
		Events: events,
	}
}
