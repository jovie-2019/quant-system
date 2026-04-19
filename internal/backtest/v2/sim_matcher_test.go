package v2

import (
	"context"
	"errors"
	"math"
	"testing"

	"quant-system/internal/adapter"
	"quant-system/pkg/contracts"
)

func TestSimMatcher_NoMarketRejectsOrder(t *testing.T) {
	m := NewSimMatcher(SimMatcherConfig{})
	_, err := m.PlaceOrder(context.Background(), adapter.VenueOrderRequest{
		ClientOrderID: "c1",
		Symbol:        "BTCUSDT",
		Side:          "buy",
		Price:         100,
		Quantity:      1,
	})
	if !errors.Is(err, ErrNoMarket) {
		t.Fatalf("err=%v want ErrNoMarket", err)
	}
}

func TestSimMatcher_BuyAppliesSlippageAndFee(t *testing.T) {
	m := NewSimMatcher(SimMatcherConfig{SlippageBps: 10, TakerFeeBps: 20}) // 10bps slip, 20bps fee
	m.UpdateMarket(contracts.MarketNormalizedEvent{
		Symbol: "BTCUSDT",
		BidPX:  999,
		AskPX:  1001,
		LastPX: 1000,
	})

	ack, err := m.PlaceOrder(context.Background(), adapter.VenueOrderRequest{
		ClientOrderID: "c1",
		Symbol:        "BTCUSDT",
		Side:          "buy",
		Quantity:      2,
	})
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	if ack.Status != "ack" || ack.VenueOrderID == "" {
		t.Fatalf("unexpected ack %+v", ack)
	}

	ticket, ok := m.TakeFill("c1")
	if !ok {
		t.Fatal("no fill queued")
	}
	// buy at ask (1001) * (1 + 10bps) = 1001 * 1.001 = 1002.001
	wantPx := 1001.0 * 1.001
	if math.Abs(ticket.FillPrice-wantPx) > 1e-9 {
		t.Fatalf("fill px=%v want=%v", ticket.FillPrice, wantPx)
	}
	// fee = px * qty * 20bps = 1002.001 * 2 * 0.002 = 4.008004
	wantFee := wantPx * 2 * 0.002
	if math.Abs(ticket.Fee-wantFee) > 1e-9 {
		t.Fatalf("fee=%v want=%v", ticket.Fee, wantFee)
	}
	if ticket.FillQty != 2 {
		t.Fatalf("qty=%v want=2", ticket.FillQty)
	}

	// TakeFill is one-shot.
	if _, ok := m.TakeFill("c1"); ok {
		t.Fatal("TakeFill should clear the ticket")
	}
}

func TestSimMatcher_SellMirrorsBuy(t *testing.T) {
	m := NewSimMatcher(SimMatcherConfig{SlippageBps: 5})
	m.UpdateMarket(contracts.MarketNormalizedEvent{
		Symbol: "BTCUSDT",
		BidPX:  2000,
		AskPX:  2002,
		LastPX: 2001,
	})
	_, err := m.PlaceOrder(context.Background(), adapter.VenueOrderRequest{
		ClientOrderID: "c2",
		Symbol:        "BTCUSDT",
		Side:          "sell",
		Quantity:      1,
	})
	if err != nil {
		t.Fatal(err)
	}
	ticket, _ := m.TakeFill("c2")
	// sell at bid (2000) * (1 - 5bps) = 2000 * 0.9995 = 1999.0
	wantPx := 2000.0 * 0.9995
	if math.Abs(ticket.FillPrice-wantPx) > 1e-9 {
		t.Fatalf("sell px=%v want=%v", ticket.FillPrice, wantPx)
	}
}

func TestSimMatcher_UpdateFromKline(t *testing.T) {
	m := NewSimMatcher(SimMatcherConfig{})
	m.UpdateMarketFromKline(contracts.Kline{
		Symbol: "BTCUSDT",
		Close:  50000,
		Closed: true,
	})
	ack, err := m.PlaceOrder(context.Background(), adapter.VenueOrderRequest{
		ClientOrderID: "c3",
		Symbol:        "BTCUSDT",
		Side:          "buy",
		Quantity:      1,
	})
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	if ack.Status != "ack" {
		t.Fatalf("status=%s", ack.Status)
	}
	ticket, _ := m.TakeFill("c3")
	// half-spread 0.5bps → ask = 50000 * (1 + 0.00005) = 50002.5
	wantPx := 50000.0 * 1.00005
	if math.Abs(ticket.FillPrice-wantPx) > 1e-6 {
		t.Fatalf("kline-buy px=%v want=%v", ticket.FillPrice, wantPx)
	}
}

func TestSimMatcher_UnsupportedSide(t *testing.T) {
	m := NewSimMatcher(SimMatcherConfig{})
	m.UpdateMarket(contracts.MarketNormalizedEvent{Symbol: "X", LastPX: 1, BidPX: 1, AskPX: 1})
	_, err := m.PlaceOrder(context.Background(), adapter.VenueOrderRequest{
		ClientOrderID: "c4",
		Symbol:        "X",
		Side:          "short",
		Quantity:      1,
	})
	if !errors.Is(err, ErrUnsupportedSide) {
		t.Fatalf("err=%v want ErrUnsupportedSide", err)
	}
}

func TestSimMatcher_CancelDropsPendingFill(t *testing.T) {
	m := NewSimMatcher(SimMatcherConfig{})
	m.UpdateMarket(contracts.MarketNormalizedEvent{Symbol: "X", LastPX: 1, BidPX: 1, AskPX: 1})
	if _, err := m.PlaceOrder(context.Background(), adapter.VenueOrderRequest{
		ClientOrderID: "c5",
		Symbol:        "X",
		Side:          "buy",
		Quantity:      1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CancelOrder(context.Background(), adapter.VenueCancelRequest{
		ClientOrderID: "c5",
		Symbol:        "X",
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.TakeFill("c5"); ok {
		t.Fatal("canceled ticket should not be retrievable")
	}
}
