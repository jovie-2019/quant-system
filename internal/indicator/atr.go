package indicator

import "math"

// ATR computes the Average True Range using Wilder's smoothing (the standard
// method). Returns a slice the same length as the inputs; the first
// (period-1) values are zero because the smoothing requires a full window.
//
// True Range for bar i is max(high-low, |high-prevClose|, |low-prevClose|).
// Wilder smoothing: ATR[i] = (ATR[i-1]*(period-1) + TR[i]) / period.
//
// Panics-free: mismatched slice lengths or a non-positive period yield a
// zero-filled output so callers can skip guards.
func ATR(highs, lows, closes []float64, period int) []float64 {
	n := len(closes)
	out := make([]float64, n)
	if n == 0 || len(highs) != n || len(lows) != n || period <= 0 || period > n {
		return out
	}

	tr := make([]float64, n)
	tr[0] = highs[0] - lows[0]
	for i := 1; i < n; i++ {
		a := highs[i] - lows[i]
		b := math.Abs(highs[i] - closes[i-1])
		c := math.Abs(lows[i] - closes[i-1])
		tr[i] = math.Max(a, math.Max(b, c))
	}

	// Seed ATR with the simple average of the first `period` TRs.
	var sum float64
	for i := 0; i < period; i++ {
		sum += tr[i]
	}
	out[period-1] = sum / float64(period)
	for i := period; i < n; i++ {
		out[i] = (out[i-1]*float64(period-1) + tr[i]) / float64(period)
	}
	return out
}
