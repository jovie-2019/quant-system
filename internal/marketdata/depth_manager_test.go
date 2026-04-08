package marketdata

import (
	"math"
	"testing"

	"quant-system/pkg/contracts"
)

func makeDepthSnapshot(venue contracts.Venue, symbol string, bids, asks []contracts.DepthLevel) contracts.DepthSnapshot {
	return contracts.DepthSnapshot{
		Venue:  venue,
		Symbol: symbol,
		Bids:   bids,
		Asks:   asks,
		TSms:   1700000000000,
	}
}

func TestDepthUpdate(t *testing.T) {
	m := NewDepthManager(nil)

	snap := makeDepthSnapshot(contracts.VenueBinance, "BTC-USDT",
		[]contracts.DepthLevel{{Price: 50000, Quantity: 1.5}},
		[]contracts.DepthLevel{{Price: 50010, Quantity: 2.0}},
	)
	m.Update(snap)

	got, ok := m.Get("binance", "BTC-USDT")
	if !ok {
		t.Fatal("expected depth snapshot to exist")
	}
	if len(got.Bids) != 1 || got.Bids[0].Price != 50000 {
		t.Errorf("unexpected bid: %+v", got.Bids)
	}
	if len(got.Asks) != 1 || got.Asks[0].Price != 50010 {
		t.Errorf("unexpected ask: %+v", got.Asks)
	}
}

func TestDepthSpread(t *testing.T) {
	m := NewDepthManager(nil)

	snap := makeDepthSnapshot(contracts.VenueBinance, "BTC-USDT",
		[]contracts.DepthLevel{
			{Price: 50000, Quantity: 1.0},
			{Price: 49990, Quantity: 2.0},
			{Price: 49980, Quantity: 3.0},
		},
		[]contracts.DepthLevel{
			{Price: 50010, Quantity: 1.0},
			{Price: 50020, Quantity: 2.0},
			{Price: 50030, Quantity: 3.0},
		},
	)
	m.Update(snap)

	spread, ok := m.Spread("binance", "BTC-USDT")
	if !ok {
		t.Fatal("expected spread to be available")
	}
	// 50010 - 50000 = 10
	if math.Abs(spread-10) > 1e-9 {
		t.Errorf("expected spread 10, got %f", spread)
	}
}

func TestDepthMidPrice(t *testing.T) {
	m := NewDepthManager(nil)

	snap := makeDepthSnapshot(contracts.VenueBinance, "ETH-USDT",
		[]contracts.DepthLevel{{Price: 3000, Quantity: 5}},
		[]contracts.DepthLevel{{Price: 3010, Quantity: 5}},
	)
	m.Update(snap)

	mid, ok := m.MidPrice("binance", "ETH-USDT")
	if !ok {
		t.Fatal("expected mid price to be available")
	}
	// (3000 + 3010) / 2 = 3005
	if math.Abs(mid-3005) > 1e-9 {
		t.Errorf("expected mid price 3005, got %f", mid)
	}
}

func TestDepthGetMissing(t *testing.T) {
	m := NewDepthManager(nil)

	_, ok := m.Get("binance", "NONEXISTENT")
	if ok {
		t.Error("expected Get to return false for non-existent symbol")
	}

	_, ok = m.Spread("binance", "NONEXISTENT")
	if ok {
		t.Error("expected Spread to return false for non-existent symbol")
	}

	_, ok = m.MidPrice("binance", "NONEXISTENT")
	if ok {
		t.Error("expected MidPrice to return false for non-existent symbol")
	}
}
