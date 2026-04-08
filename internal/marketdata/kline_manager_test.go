package marketdata

import (
	"context"
	"log/slog"
	"testing"

	"quant-system/pkg/contracts"
)

func makeKline(symbol, interval string, closePrice float64, openTime int64, closed bool) contracts.Kline {
	return contracts.Kline{
		Venue:     contracts.VenueBinance,
		Symbol:    symbol,
		Interval:  interval,
		OpenTime:  openTime,
		Open:      closePrice - 1,
		High:      closePrice + 1,
		Low:       closePrice - 2,
		Close:     closePrice,
		Volume:    100,
		CloseTime: openTime + 60000,
		Closed:    closed,
	}
}

func mockFetcher(klines []contracts.Kline) KlineFetcher {
	return func(_ context.Context, _, _ string, _ int) ([]contracts.Kline, error) {
		return klines, nil
	}
}

func TestWarmup(t *testing.T) {
	logger := slog.Default()
	m := NewKlineManager(logger)

	historical := make([]contracts.Kline, 5)
	for i := range historical {
		historical[i] = makeKline("BTC-USDT", "1m", float64(100+i), int64(i*60000), true)
	}

	err := m.Warmup(context.Background(), []string{"BTC-USDT"}, []string{"1m"}, 5, mockFetcher(historical))
	if err != nil {
		t.Fatalf("Warmup returned error: %v", err)
	}

	got := m.Get("BTC-USDT", "1m", 10)
	if len(got) != 5 {
		t.Fatalf("expected 5 klines, got %d", len(got))
	}
	if got[0].Close != 100 {
		t.Errorf("expected first close 100, got %f", got[0].Close)
	}
	if got[4].Close != 104 {
		t.Errorf("expected last close 104, got %f", got[4].Close)
	}
}

func TestUpdateOpenKline(t *testing.T) {
	m := NewKlineManager(nil)

	// Seed with one closed kline.
	m.Update(makeKline("BTC-USDT", "1m", 100, 0, true))

	// Update with an open (unclosed) kline — should modify the last entry.
	open := makeKline("BTC-USDT", "1m", 105, 60000, false)
	m.Update(open)

	got := m.Get("BTC-USDT", "1m", 10)
	if len(got) != 1 {
		t.Fatalf("expected 1 kline (open replaces last), got %d", len(got))
	}
	if got[0].Close != 105 {
		t.Errorf("expected close 105 after open update, got %f", got[0].Close)
	}
}

func TestUpdateClosedKline(t *testing.T) {
	m := NewKlineManager(nil)

	m.Update(makeKline("BTC-USDT", "1m", 100, 0, true))
	m.Update(makeKline("BTC-USDT", "1m", 200, 60000, true))

	got := m.Get("BTC-USDT", "1m", 10)
	if len(got) != 2 {
		t.Fatalf("expected 2 klines, got %d", len(got))
	}
	if got[1].Close != 200 {
		t.Errorf("expected second close 200, got %f", got[1].Close)
	}
}

func TestRingBufferOverflow(t *testing.T) {
	m := NewKlineManager(nil)

	// Warmup with exactly 200 klines.
	historical := make([]contracts.Kline, 200)
	for i := range historical {
		historical[i] = makeKline("BTC-USDT", "1m", float64(i), int64(i*60000), true)
	}

	err := m.Warmup(context.Background(), []string{"BTC-USDT"}, []string{"1m"}, 200, mockFetcher(historical))
	if err != nil {
		t.Fatalf("Warmup returned error: %v", err)
	}

	// Add one more closed kline.
	m.Update(makeKline("BTC-USDT", "1m", 999, 200*60000, true))

	got := m.Get("BTC-USDT", "1m", 0)
	if len(got) != 200 {
		t.Fatalf("expected 200 klines after overflow, got %d", len(got))
	}

	// Oldest (close=0) should be dropped; first should now be close=1.
	if got[0].Close != 1 {
		t.Errorf("expected oldest close=1 after drop, got %f", got[0].Close)
	}
	// Last should be the newly added one.
	if got[199].Close != 999 {
		t.Errorf("expected newest close=999, got %f", got[199].Close)
	}
}

func TestCloses(t *testing.T) {
	m := NewKlineManager(nil)

	for i := 0; i < 5; i++ {
		m.Update(makeKline("ETH-USDT", "5m", float64(10+i), int64(i*300000), true))
	}

	closes := m.Closes("ETH-USDT", "5m", 3)
	if len(closes) != 3 {
		t.Fatalf("expected 3 closes, got %d", len(closes))
	}
	// Should be the last 3: 12, 13, 14
	expected := []float64{12, 13, 14}
	for i, v := range expected {
		if closes[i] != v {
			t.Errorf("closes[%d] = %f, want %f", i, closes[i], v)
		}
	}
}

func TestGetEmpty(t *testing.T) {
	m := NewKlineManager(nil)

	got := m.Get("NONE", "1m", 10)
	if got != nil {
		t.Errorf("expected nil for empty buffer, got %v", got)
	}

	_, ok := m.Latest("NONE", "1m")
	if ok {
		t.Error("expected Latest to return false for empty buffer")
	}
}
