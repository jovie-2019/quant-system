package indicator

import (
	"math"
	"math/rand"
	"testing"
)

// --------------------- ATR ---------------------

func TestATR_ZeroOnMismatchedInputs(t *testing.T) {
	out := ATR([]float64{1, 2}, []float64{0.5}, []float64{1, 2}, 2)
	if len(out) != 2 || out[0] != 0 || out[1] != 0 {
		t.Fatalf("ATR mismatched inputs should be zero slice, got %+v", out)
	}
}

func TestATR_ConstantRangeGivesConstantATR(t *testing.T) {
	// A ramp with constant daily range 2: H - L = 2 every bar, no gaps.
	highs := []float64{12, 13, 14, 15, 16, 17, 18, 19, 20, 21}
	lows := []float64{10, 11, 12, 13, 14, 15, 16, 17, 18, 19}
	closes := []float64{11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	atr := ATR(highs, lows, closes, 3)

	// First (period-1)=2 values are zero.
	for i := 0; i < 2; i++ {
		if atr[i] != 0 {
			t.Fatalf("warmup atr[%d]=%v want 0", i, atr[i])
		}
	}
	for i := 2; i < len(atr); i++ {
		// Because ranges gap is 2 (H-L=2, gap from prev close = 1 so TR=max(2,1,1)=2).
		if math.Abs(atr[i]-2) > 1e-9 {
			t.Fatalf("atr[%d]=%v want ~2 (constant-range series)", i, atr[i])
		}
	}
}

// --------------------- ADX ---------------------

func TestADX_SustainedUptrendIsStrong(t *testing.T) {
	// Strict upward progression. Every bar's high sets a new high, low stays flat.
	n := 60
	highs := make([]float64, n)
	lows := make([]float64, n)
	closes := make([]float64, n)
	for i := 0; i < n; i++ {
		highs[i] = 100 + float64(i) + 0.5
		lows[i] = 100 + float64(i) - 0.5
		closes[i] = 100 + float64(i)
	}
	res := ADX(highs, lows, closes, 14)
	// After warmup, ADX should be firmly above 25 for a clean trend.
	last := res.ADX[n-1]
	if last < 25 {
		t.Fatalf("ADX on strict uptrend=%v want > 25", last)
	}
	if res.PlusDI[n-1] <= res.MinusDI[n-1] {
		t.Fatalf("+DI=%v must exceed -DI=%v on uptrend", res.PlusDI[n-1], res.MinusDI[n-1])
	}
}

func TestADX_FlatSeriesIsWeak(t *testing.T) {
	// Constant price -> no directional movement -> ADX near 0.
	n := 60
	highs := make([]float64, n)
	lows := make([]float64, n)
	closes := make([]float64, n)
	for i := range highs {
		highs[i] = 100
		lows[i] = 99
		closes[i] = 99.5
	}
	res := ADX(highs, lows, closes, 14)
	if res.ADX[n-1] > 5 {
		t.Fatalf("ADX on flat series=%v want near 0", res.ADX[n-1])
	}
}

// --------------------- Hurst ---------------------

func TestHurst_IIDReturnsNearHalf(t *testing.T) {
	// i.i.d. Gaussian returns ⇒ H ≈ 0.5 (uncorrelated).
	rng := rand.New(rand.NewSource(42))
	n := 4096
	returns := make([]float64, n)
	for i := range returns {
		returns[i] = rng.NormFloat64()
	}
	h := Hurst(returns, 8)
	if h < 0.35 || h > 0.65 {
		t.Fatalf("Hurst(i.i.d. returns)=%v want ~0.5 (+/- 0.15)", h)
	}
}

func TestHurst_PersistentReturnsAboveHalf(t *testing.T) {
	// AR(1) with positive coefficient ⇒ persistent returns ⇒ H > 0.5.
	rng := rand.New(rand.NewSource(7))
	n := 4096
	returns := make([]float64, n)
	returns[0] = rng.NormFloat64()
	for i := 1; i < n; i++ {
		returns[i] = 0.75*returns[i-1] + rng.NormFloat64()
	}
	h := Hurst(returns, 8)
	if h <= 0.55 {
		t.Fatalf("Hurst(AR(1) phi=0.75)=%v want > 0.55", h)
	}
}

func TestHurstFromPrices_RandomWalkNearHalf(t *testing.T) {
	// Price = cumsum of i.i.d. Gaussian (random walk).
	// HurstFromPrices should recover H ≈ 0.5 by diffing internally.
	rng := rand.New(rand.NewSource(42))
	n := 4096
	prices := make([]float64, n)
	prices[0] = 100
	for i := 1; i < n; i++ {
		prices[i] = prices[i-1] * math.Exp(0.001*rng.NormFloat64())
	}
	h := HurstFromPrices(prices, 8)
	if h < 0.35 || h > 0.65 {
		t.Fatalf("HurstFromPrices(random walk)=%v want ~0.5 (+/- 0.15)", h)
	}
}

func TestHurst_TooShortReturnsZero(t *testing.T) {
	if Hurst([]float64{1, 2, 3}, 8) != 0 {
		t.Fatal("short series should return 0")
	}
}
