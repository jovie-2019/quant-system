package strategy

import (
	"context"
	"errors"
	"testing"

	"quant-system/internal/book"
	"quant-system/internal/normalizer"
)

type fakeStrategy struct {
	id string
}

type fakeBookReader struct{}

func (f fakeBookReader) Snapshot(key book.VenueSymbol) (book.BookSnapshot, bool) {
	return book.BookSnapshot{
		Venue:     key.Venue,
		Symbol:    key.Symbol,
		BestBidPX: 62000.1,
		BestAskPX: 62000.2,
	}, true
}

func (f fakeStrategy) ID() string { return f.id }

func (f fakeStrategy) OnMarket(_ normalizer.MarketNormalizedEvent) []OrderIntent {
	return []OrderIntent{
		{
			IntentID:    "intent-1",
			Symbol:      "BTC-USDT",
			Side:        "buy",
			Price:       62000.0,
			Quantity:    0.1,
			TimeInForce: "IOC",
		},
	}
}

func TestRegisterDuplicateStrategy(t *testing.T) {
	rt, err := NewInMemoryRuntime(func(_ context.Context, _ OrderIntent) error { return nil })
	if err != nil {
		t.Fatalf("NewInMemoryRuntime() error = %v", err)
	}

	if err := rt.Register(fakeStrategy{id: "s1"}); err != nil {
		t.Fatalf("Register() first call error = %v", err)
	}
	err = rt.Register(fakeStrategy{id: "s1"})
	if err == nil {
		t.Fatal("expected duplicate registration error")
	}
	if !errors.Is(err, ErrStrategyExists) {
		t.Fatalf("expected ErrStrategyExists, got %v", err)
	}
}

func TestHandleMarketDispatchesIntent(t *testing.T) {
	var sinkCalls int
	var strategyID string
	rt, err := NewInMemoryRuntime(func(_ context.Context, intent OrderIntent) error {
		sinkCalls++
		strategyID = intent.StrategyID
		return nil
	})
	if err != nil {
		t.Fatalf("NewInMemoryRuntime() error = %v", err)
	}
	if err := rt.Register(fakeStrategy{id: "s1"}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	err = rt.HandleMarket(context.Background(), normalizer.MarketNormalizedEvent{Symbol: "BTC-USDT"})
	if err != nil {
		t.Fatalf("HandleMarket() error = %v", err)
	}
	if sinkCalls != 1 {
		t.Fatalf("expected 1 sink call, got %d", sinkCalls)
	}
	if strategyID != "s1" {
		t.Fatalf("expected strategy id s1, got %s", strategyID)
	}
}

func TestBookReaderAccess(t *testing.T) {
	rt, err := NewInMemoryRuntime(func(_ context.Context, _ OrderIntent) error { return nil })
	if err != nil {
		t.Fatalf("NewInMemoryRuntime() error = %v", err)
	}

	rt.SetBookReader(fakeBookReader{})

	snap, ok := rt.GetBookSnapshot(book.VenueSymbol{
		Symbol: "BTC-USDT",
	})
	if !ok {
		t.Fatal("expected book snapshot")
	}
	if snap.BestBidPX != 62000.1 || snap.BestAskPX != 62000.2 {
		t.Fatalf("unexpected book snapshot: %+v", snap)
	}
}
