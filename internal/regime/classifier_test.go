package regime

import (
	"math"
	"math/rand"
	"testing"

	"quant-system/pkg/contracts"
)

// makeKlines generates n consecutive 1-minute klines with OHLC derived
// from the supplied price path. High/Low are set symmetrically around
// Close with a tiny noise envelope so ATR is non-zero.
func makeKlines(path []float64, symbol string) []contracts.Kline {
	n := len(path)
	out := make([]contracts.Kline, n)
	baseTS := int64(1_700_000_000_000)
	for i, px := range path {
		env := math.Abs(px) * 0.0005 // 5bps envelope
		if env == 0 {
			env = 0.5
		}
		out[i] = contracts.Kline{
			Venue:     contracts.VenueBinance,
			Symbol:    symbol,
			Interval:  "1m",
			OpenTime:  baseTS + int64(i)*60_000,
			CloseTime: baseTS + int64(i)*60_000 + 59_999,
			Open:      px,
			High:      px + env,
			Low:       px - env,
			Close:     px,
			Volume:    1,
			Closed:    true,
		}
	}
	return out
}

func TestClassifier_WarmsUpThenEmits(t *testing.T) {
	// Below warmup → zero records.
	short := makeKlines(make([]float64, 10), "X")
	if recs := ClassifyKlines(short, ClassifyConfig{}); len(recs) != 0 {
		t.Fatalf("expected 0 records in warmup, got %d", len(recs))
	}

	// Enough bars → at least one record.
	path := make([]float64, 200)
	for i := range path {
		path[i] = 100 + float64(i)
	}
	recs := ClassifyKlines(makeKlines(path, "X"), ClassifyConfig{})
	if len(recs) == 0 {
		t.Fatal("expected records after warmup")
	}
}

func TestClassifier_SustainedUptrendIsTrendUp(t *testing.T) {
	// Monotone rising close → ADX high, +DI > -DI, Hurst on returns
	// is actually 0 for truly constant returns (std=0 invalidates R/S),
	// so we add a tiny deterministic noise to keep Hurst defined.
	rng := rand.New(rand.NewSource(1))
	n := 400
	path := make([]float64, n)
	path[0] = 100
	for i := 1; i < n; i++ {
		path[i] = path[i-1] * math.Exp(0.0015+0.0001*rng.NormFloat64()) // strong upward drift
	}
	recs := ClassifyKlines(makeKlines(path, "X"), ClassifyConfig{})
	if len(recs) == 0 {
		t.Fatal("no records")
	}

	// At least 60% of the late-stage records should be trend_up.
	tail := recs[len(recs)/2:]
	hits := 0
	for _, r := range tail {
		if r.Regime == RegimeTrendUp {
			hits++
		}
	}
	if float64(hits)/float64(len(tail)) < 0.6 {
		t.Fatalf("uptrend tail only %d/%d trend_up (want >=60%%)", hits, len(tail))
	}
}

func TestClassifier_ChoppySeriesIsRangeOrUnknown(t *testing.T) {
	// Mean-reverting log-returns around zero → neither strong ADX nor
	// trending Hurst. Result should be overwhelmingly range or unknown
	// (not trend_up/trend_down).
	rng := rand.New(rand.NewSource(42))
	n := 400
	path := make([]float64, n)
	path[0] = 100
	for i := 1; i < n; i++ {
		// OU-ish: pull toward 100 with small noise.
		path[i] = path[i-1] + 0.1*(100-path[i-1]) + rng.NormFloat64()*0.5
	}
	recs := ClassifyKlines(makeKlines(path, "X"), ClassifyConfig{})
	if len(recs) == 0 {
		t.Fatal("no records")
	}
	trend := 0
	for _, r := range recs {
		if r.Regime == RegimeTrendUp || r.Regime == RegimeTrendDown {
			trend++
		}
	}
	if float64(trend)/float64(len(recs)) > 0.2 {
		t.Fatalf("mean-reverting series classified as trend too often: %d/%d", trend, len(recs))
	}
}

func TestClassifier_HighVolOverridesTrend(t *testing.T) {
	// Features with a clearly elevated ATRPercent should be classified as
	// high_vol regardless of ADX/Hurst values.
	feat := Features{
		ADX:        40,
		PlusDI:     30,
		MinusDI:    10,
		Hurst:      0.7,
		ATRPercent: 0.05, // 5%
	}
	label, conf := ClassifyFeatures(feat, DefaultThresholds())
	if label != RegimeHighVol {
		t.Fatalf("label=%s want high_vol", label)
	}
	if conf <= 0 {
		t.Fatalf("conf=%v want >0", conf)
	}
}

func TestClassifier_LowLiquidityShortCircuits(t *testing.T) {
	feat := Features{
		ADX:        40,
		Hurst:      0.7,
		ATRPercent: 0.005,
		VolumeLast: 1,
		VolumeMean: 100, // 1/100 = 1% << 25% threshold
	}
	label, _ := ClassifyFeatures(feat, DefaultThresholds())
	if label != RegimeLowLiq {
		t.Fatalf("label=%s want low_liq", label)
	}
}

func TestClassifier_RecordPopulatesAllFields(t *testing.T) {
	path := make([]float64, 300)
	for i := range path {
		path[i] = 100 + float64(i)*0.1
	}
	recs := ClassifyKlines(makeKlines(path, "BTCUSDT"), ClassifyConfig{})
	if len(recs) == 0 {
		t.Fatal("no records")
	}
	r := recs[0]
	if r.Symbol != "BTCUSDT" {
		t.Fatalf("symbol=%q", r.Symbol)
	}
	if r.Method != MethodThreshold {
		t.Fatalf("method=%s", r.Method)
	}
	if r.BarTime == 0 {
		t.Fatal("bar time not set")
	}
	if r.Features.LastClose == 0 {
		t.Fatal("last close not set")
	}
}
