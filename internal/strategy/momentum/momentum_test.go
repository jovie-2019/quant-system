package momentum

import (
	"strings"
	"testing"
	"time"

	"quant-system/pkg/contracts"
)

func stableEvents(symbol string, px float64, n int) []contracts.MarketNormalizedEvent {
	evts := make([]contracts.MarketNormalizedEvent, n)
	for i := range evts {
		evts[i] = contracts.MarketNormalizedEvent{
			Symbol: symbol,
			LastPX: px,
			AskPX:  px + 1,
			BidPX:  px - 1,
		}
	}
	return evts
}

func TestWindowNotFull_NoSignal(t *testing.T) {
	s := New(Config{Symbol: "BTC-USDT", WindowSize: 5, OrderQty: 0.1})
	for _, evt := range stableEvents("BTC-USDT", 60000, 4) {
		intents := s.OnMarket(evt)
		if len(intents) != 0 {
			t.Fatalf("expected no signal before window full, got %d", len(intents))
		}
	}
}

func TestBreakoutBuy(t *testing.T) {
	s := New(Config{
		Symbol:            "BTC-USDT",
		WindowSize:        5,
		BreakoutThreshold: 0.001,
		OrderQty:          0.1,
		Cooldown:          0,
	})

	// Fill the window.
	for _, evt := range stableEvents("BTC-USDT", 60000, 5) {
		s.OnMarket(evt)
	}

	// Breakout above: 60000 * 1.001 = 60060 → price 60100 should trigger BUY.
	intents := s.OnMarket(contracts.MarketNormalizedEvent{
		Symbol: "BTC-USDT",
		LastPX: 60100,
	})
	if len(intents) != 1 {
		t.Fatalf("expected 1 BUY intent, got %d", len(intents))
	}
	if intents[0].Side != "buy" {
		t.Fatalf("expected buy, got %s", intents[0].Side)
	}
	if intents[0].Quantity != 0.1 {
		t.Fatalf("expected qty 0.1, got %f", intents[0].Quantity)
	}
}

func TestBreakdownSell(t *testing.T) {
	s := New(Config{
		Symbol:            "BTC-USDT",
		WindowSize:        5,
		BreakoutThreshold: 0.001,
		OrderQty:          0.1,
		Cooldown:          0,
	})

	// Fill window.
	for _, evt := range stableEvents("BTC-USDT", 60000, 5) {
		s.OnMarket(evt)
	}

	// Trigger a buy first so hasPos=true.
	s.OnMarket(contracts.MarketNormalizedEvent{Symbol: "BTC-USDT", LastPX: 60100})

	// Now fill window with a higher baseline so low stays reasonable.
	for _, evt := range stableEvents("BTC-USDT", 60100, 5) {
		s.OnMarket(evt)
	}

	// Breakdown below: 60100 * (1-0.001) ≈ 60039.9 → price 59900 should trigger SELL.
	intents := s.OnMarket(contracts.MarketNormalizedEvent{
		Symbol: "BTC-USDT",
		LastPX: 59900,
	})
	if len(intents) != 1 {
		t.Fatalf("expected 1 SELL intent, got %d", len(intents))
	}
	if intents[0].Side != "sell" {
		t.Fatalf("expected sell, got %s", intents[0].Side)
	}
}

func TestNoSellWithoutPosition(t *testing.T) {
	s := New(Config{
		Symbol:            "BTC-USDT",
		WindowSize:        5,
		BreakoutThreshold: 0.001,
		OrderQty:          0.1,
		Cooldown:          0,
	})

	for _, evt := range stableEvents("BTC-USDT", 60000, 5) {
		s.OnMarket(evt)
	}

	// Breakdown without having a position.
	intents := s.OnMarket(contracts.MarketNormalizedEvent{
		Symbol: "BTC-USDT",
		LastPX: 59900,
	})
	if len(intents) != 0 {
		t.Fatalf("expected no signal without position, got %d", len(intents))
	}
}

func TestCooldown(t *testing.T) {
	s := New(Config{
		Symbol:            "BTC-USDT",
		WindowSize:        5,
		BreakoutThreshold: 0.001,
		OrderQty:          0.1,
		Cooldown:          time.Hour,
	})

	for _, evt := range stableEvents("BTC-USDT", 60000, 5) {
		s.OnMarket(evt)
	}

	// First signal.
	intents := s.OnMarket(contracts.MarketNormalizedEvent{Symbol: "BTC-USDT", LastPX: 60100})
	if len(intents) != 1 {
		t.Fatalf("expected first signal, got %d", len(intents))
	}

	// Second signal within cooldown.
	intents = s.OnMarket(contracts.MarketNormalizedEvent{Symbol: "BTC-USDT", LastPX: 60200})
	if len(intents) != 0 {
		t.Fatalf("expected no signal during cooldown, got %d", len(intents))
	}
}

func TestIgnoreDifferentSymbol(t *testing.T) {
	s := New(Config{Symbol: "BTC-USDT", WindowSize: 5, OrderQty: 0.1, Cooldown: 0})

	for _, evt := range stableEvents("BTC-USDT", 60000, 5) {
		s.OnMarket(evt)
	}

	intents := s.OnMarket(contracts.MarketNormalizedEvent{Symbol: "ETH-USDT", LastPX: 99999})
	if len(intents) != 0 {
		t.Fatalf("expected no signal for different symbol, got %d", len(intents))
	}
}

func TestIntentIDUnique(t *testing.T) {
	s := New(Config{
		Symbol:            "BTC-USDT",
		WindowSize:        5,
		BreakoutThreshold: 0.001,
		OrderQty:          0.1,
		Cooldown:          0,
	})

	for _, evt := range stableEvents("BTC-USDT", 60000, 5) {
		s.OnMarket(evt)
	}

	i1 := s.OnMarket(contracts.MarketNormalizedEvent{Symbol: "BTC-USDT", LastPX: 60100})
	// Reset window to allow second signal.
	for _, evt := range stableEvents("BTC-USDT", 60100, 5) {
		s.OnMarket(evt)
	}
	i2 := s.OnMarket(contracts.MarketNormalizedEvent{Symbol: "BTC-USDT", LastPX: 60200})

	if len(i1) != 1 || len(i2) != 1 {
		t.Fatalf("expected 1+1 intents, got %d+%d", len(i1), len(i2))
	}
	if i1[0].IntentID == i2[0].IntentID {
		t.Fatal("intent IDs should be unique")
	}
	if !strings.HasPrefix(i1[0].IntentID, "momentum-BTC-USDT-") {
		t.Fatalf("unexpected intent ID format: %s", i1[0].IntentID)
	}
}
