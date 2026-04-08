package indicator

import "math"

// BollingerResult holds the three Bollinger Band lines.
type BollingerResult struct {
	Upper  []float64
	Middle []float64 // SMA
	Lower  []float64
}

// Bollinger calculates Bollinger Bands.
// Standard parameters: period=20, stddev=2.0.
// Middle = SMA(period), Upper = Middle + stddev*StdDev, Lower = Middle - stddev*StdDev.
func Bollinger(closes []float64, period int, stddev float64) BollingerResult {
	n := len(closes)
	if n == 0 || period <= 0 || period > n {
		return BollingerResult{
			Upper:  make([]float64, n),
			Middle: make([]float64, n),
			Lower:  make([]float64, n),
		}
	}

	middle := SMA(closes, period)
	upper := make([]float64, n)
	lower := make([]float64, n)

	for i := period - 1; i < n; i++ {
		// Calculate population standard deviation over the window.
		var sumSq float64
		for j := i - period + 1; j <= i; j++ {
			diff := closes[j] - middle[i]
			sumSq += diff * diff
		}
		sd := math.Sqrt(sumSq / float64(period))

		upper[i] = middle[i] + stddev*sd
		lower[i] = middle[i] - stddev*sd
	}

	return BollingerResult{
		Upper:  upper,
		Middle: middle,
		Lower:  lower,
	}
}
