package marketstore

import (
	"context"
	"testing"

	"quant-system/internal/regime"
)

func regimeRec(symbol string, barTime int64, label regime.Regime) regime.Record {
	return regime.Record{
		Venue:      "binance",
		Symbol:     symbol,
		Interval:   "1m",
		BarTime:    barTime,
		Method:     regime.MethodThreshold,
		Regime:     label,
		Confidence: 0.5,
		Features: regime.Features{
			ADX: 30, ATR: 1, BBW: 0.02, Hurst: 0.55,
		},
	}
}

func TestInMemoryRegimeStore_UpsertAndQuery(t *testing.T) {
	s := NewInMemoryRegimeStore()
	ctx := context.Background()

	recs := []regime.Record{
		regimeRec("BTCUSDT", 1000, regime.RegimeTrendUp),
		regimeRec("BTCUSDT", 2000, regime.RegimeTrendUp),
		regimeRec("BTCUSDT", 3000, regime.RegimeRange),
	}
	if err := s.UpsertRegimes(ctx, recs); err != nil {
		t.Fatal(err)
	}

	got, err := s.QueryRegimes(ctx, RegimeQuery{Symbol: "BTCUSDT", Interval: "1m"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d want 3", len(got))
	}
	// Ordered ASC by BarTime.
	if got[0].BarTime != 1000 || got[2].BarTime != 3000 {
		t.Fatalf("order wrong: %+v", got)
	}
	// Last is Range.
	if got[2].Regime != regime.RegimeRange {
		t.Fatalf("last regime=%s", got[2].Regime)
	}
}

func TestInMemoryRegimeStore_UpsertReplacesOnDuplicate(t *testing.T) {
	s := NewInMemoryRegimeStore()
	ctx := context.Background()

	_ = s.UpsertRegimes(ctx, []regime.Record{regimeRec("BTCUSDT", 1000, regime.RegimeTrendUp)})
	_ = s.UpsertRegimes(ctx, []regime.Record{regimeRec("BTCUSDT", 1000, regime.RegimeRange)})

	got, _ := s.QueryRegimes(ctx, RegimeQuery{Symbol: "BTCUSDT", Interval: "1m"})
	if len(got) != 1 {
		t.Fatalf("len=%d want 1", len(got))
	}
	if got[0].Regime != regime.RegimeRange {
		t.Fatalf("regime=%s want range (latest wins)", got[0].Regime)
	}
}

func TestInMemoryRegimeStore_LatestRegimesMatrix(t *testing.T) {
	s := NewInMemoryRegimeStore()
	ctx := context.Background()

	_ = s.UpsertRegimes(ctx, []regime.Record{
		regimeRec("BTCUSDT", 1000, regime.RegimeTrendUp),
		regimeRec("BTCUSDT", 2000, regime.RegimeRange),
		regimeRec("ETHUSDT", 2500, regime.RegimeHighVol),
	})
	out, err := s.LatestRegimes(ctx, []RegimeMatrixKey{
		{Venue: "binance", Symbol: "BTCUSDT", Interval: "1m", Method: regime.MethodThreshold},
		{Venue: "binance", Symbol: "ETHUSDT", Interval: "1m", Method: regime.MethodThreshold},
		{Venue: "binance", Symbol: "SOLUSDT", Interval: "1m", Method: regime.MethodThreshold}, // missing
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("len=%d want 2 (missing key omitted)", len(out))
	}
	byMap := map[string]regime.Record{}
	for _, r := range out {
		byMap[r.Symbol] = r
	}
	if byMap["BTCUSDT"].Regime != regime.RegimeRange {
		t.Fatalf("BTC latest=%s want range", byMap["BTCUSDT"].Regime)
	}
	if byMap["ETHUSDT"].Regime != regime.RegimeHighVol {
		t.Fatalf("ETH latest=%s want high_vol", byMap["ETHUSDT"].Regime)
	}
}

func TestInMemoryRegimeStore_QueryRangeAndLimit(t *testing.T) {
	s := NewInMemoryRegimeStore()
	ctx := context.Background()
	recs := make([]regime.Record, 0, 10)
	for i := int64(0); i < 10; i++ {
		recs = append(recs, regimeRec("BTCUSDT", i*1000, regime.RegimeRange))
	}
	_ = s.UpsertRegimes(ctx, recs)

	got, _ := s.QueryRegimes(ctx, RegimeQuery{
		Symbol: "BTCUSDT", Interval: "1m", StartMS: 3000, EndMS: 7000,
	})
	if len(got) != 5 {
		t.Fatalf("len=%d want 5", len(got))
	}

	limited, _ := s.QueryRegimes(ctx, RegimeQuery{
		Symbol: "BTCUSDT", Interval: "1m", Limit: 3,
	})
	if len(limited) != 3 {
		t.Fatalf("limited len=%d want 3", len(limited))
	}
}

func TestInMemoryRegimeStore_FilterByMethod(t *testing.T) {
	s := NewInMemoryRegimeStore()
	ctx := context.Background()

	a := regimeRec("BTCUSDT", 1000, regime.RegimeTrendUp)
	a.Method = regime.MethodThreshold
	b := regimeRec("BTCUSDT", 2000, regime.RegimeRange)
	b.Method = regime.MethodHMM
	_ = s.UpsertRegimes(ctx, []regime.Record{a, b})

	thresh, _ := s.QueryRegimes(ctx, RegimeQuery{
		Symbol: "BTCUSDT", Interval: "1m", Method: regime.MethodThreshold,
	})
	if len(thresh) != 1 || thresh[0].Regime != regime.RegimeTrendUp {
		t.Fatalf("threshold result=%+v", thresh)
	}
	hmm, _ := s.QueryRegimes(ctx, RegimeQuery{
		Symbol: "BTCUSDT", Interval: "1m", Method: regime.MethodHMM,
	})
	if len(hmm) != 1 || hmm[0].Regime != regime.RegimeRange {
		t.Fatalf("hmm result=%+v", hmm)
	}
}
