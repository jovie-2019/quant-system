package marketstore

import (
	"context"
	"errors"
	"testing"

	"quant-system/pkg/contracts"
)

func kline(venue, sym, interval string, openTime int64, closePx float64) contracts.Kline {
	return contracts.Kline{
		Venue:     contracts.Venue(venue),
		Symbol:    sym,
		Interval:  interval,
		OpenTime:  openTime,
		CloseTime: openTime + 59_999,
		Open:      closePx,
		High:      closePx,
		Low:       closePx,
		Close:     closePx,
		Volume:    1,
		Closed:    true,
	}
}

func TestMemoryStore_UpsertAndQuery(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	// Insert out-of-order; store should return sorted.
	err := s.Upsert(ctx, []contracts.Kline{
		kline("binance", "BTCUSDT", "1m", 2000, 102),
		kline("binance", "BTCUSDT", "1m", 1000, 101),
		kline("binance", "BTCUSDT", "1m", 3000, 103),
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.Query(ctx, KlineQuery{Venue: "binance", Symbol: "BTCUSDT", Interval: "1m"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("len=%d want 3", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i].OpenTime <= got[i-1].OpenTime {
			t.Fatalf("not sorted at %d: %d <= %d", i, got[i].OpenTime, got[i-1].OpenTime)
		}
	}
}

func TestMemoryStore_UpsertReplacesOnDuplicateKey(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	_ = s.Upsert(ctx, []contracts.Kline{kline("binance", "BTCUSDT", "1m", 1000, 100)})
	_ = s.Upsert(ctx, []contracts.Kline{kline("binance", "BTCUSDT", "1m", 1000, 999)})

	got, _ := s.Query(ctx, KlineQuery{Venue: "binance", Symbol: "BTCUSDT", Interval: "1m"})
	if len(got) != 1 {
		t.Fatalf("len=%d want 1 (duplicate key should replace)", len(got))
	}
	if got[0].Close != 999 {
		t.Fatalf("close=%v want 999 (latest write should win)", got[0].Close)
	}
}

func TestMemoryStore_QueryRangeFilters(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	for i := int64(0); i < 10; i++ {
		_ = s.Upsert(ctx, []contracts.Kline{kline("binance", "BTCUSDT", "1m", i*60_000, float64(i))})
	}

	got, err := s.Query(ctx, KlineQuery{
		Venue: "binance", Symbol: "BTCUSDT", Interval: "1m",
		StartMS: 3 * 60_000, EndMS: 6 * 60_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("len=%d want 4 (inclusive range)", len(got))
	}
	if got[0].OpenTime != 3*60_000 || got[len(got)-1].OpenTime != 6*60_000 {
		t.Fatalf("bounds wrong: %+v .. %+v", got[0], got[len(got)-1])
	}
}

func TestMemoryStore_LimitTruncates(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	for i := int64(0); i < 100; i++ {
		_ = s.Upsert(ctx, []contracts.Kline{kline("binance", "BTCUSDT", "1m", i, float64(i))})
	}

	got, _ := s.Query(ctx, KlineQuery{Symbol: "BTCUSDT", Interval: "1m", Limit: 7})
	if len(got) != 7 {
		t.Fatalf("len=%d want 7", len(got))
	}
}

func TestMemoryStore_Count(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	for i := int64(0); i < 50; i++ {
		_ = s.Upsert(ctx, []contracts.Kline{kline("binance", "BTCUSDT", "1m", i*60_000, float64(i))})
	}

	n, err := s.Count(ctx, KlineQuery{Symbol: "BTCUSDT", Interval: "1m"})
	if err != nil {
		t.Fatal(err)
	}
	if n != 50 {
		t.Fatalf("count=%d want 50", n)
	}

	// Ranged count ignores Limit.
	n2, _ := s.Count(ctx, KlineQuery{
		Symbol: "BTCUSDT", Interval: "1m",
		StartMS: 10 * 60_000, EndMS: 19 * 60_000,
		Limit:   3,
	})
	if n2 != 10 {
		t.Fatalf("count=%d want 10 (limit ignored)", n2)
	}
}

func TestMemoryStore_QueryValidation(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	_, err := s.Query(ctx, KlineQuery{Interval: "1m"})
	if err == nil || !errors.Is(err, err) { // sanity; we just want non-nil
		t.Fatal("expected error for missing symbol")
	}
	_, err = s.Query(ctx, KlineQuery{Symbol: "X"})
	if err == nil {
		t.Fatal("expected error for missing interval")
	}
	_, err = s.Query(ctx, KlineQuery{Symbol: "X", Interval: "1m", StartMS: 100, EndMS: 50})
	if err == nil {
		t.Fatal("expected error for StartMS > EndMS")
	}
}

func TestMemoryStore_MultipleSymbolsIsolated(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	_ = s.Upsert(ctx, []contracts.Kline{
		kline("binance", "BTCUSDT", "1m", 1000, 100),
		kline("binance", "ETHUSDT", "1m", 1000, 200),
	})
	btc, _ := s.Query(ctx, KlineQuery{Symbol: "BTCUSDT", Interval: "1m"})
	eth, _ := s.Query(ctx, KlineQuery{Symbol: "ETHUSDT", Interval: "1m"})
	if len(btc) != 1 || btc[0].Close != 100 {
		t.Fatalf("btc=%+v", btc)
	}
	if len(eth) != 1 || eth[0].Close != 200 {
		t.Fatalf("eth=%+v", eth)
	}
}

func TestMemoryStore_PingAndClose(t *testing.T) {
	s := NewMemoryStore()
	if err := s.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}
