package indicator

// EMA calculates Exponential Moving Average.
// Uses multiplier = 2 / (period + 1).
// First value is the SMA of the first `period` values.
func EMA(closes []float64, period int) []float64 {
	n := len(closes)
	if n == 0 || period <= 0 || period > n {
		return make([]float64, n)
	}

	result := make([]float64, n)
	multiplier := 2.0 / float64(period+1)

	// Seed with SMA of the first `period` values.
	var sum float64
	for i := 0; i < period; i++ {
		sum += closes[i]
	}
	result[period-1] = sum / float64(period)

	// Apply EMA formula for subsequent values.
	for i := period; i < n; i++ {
		result[i] = (closes[i]-result[i-1])*multiplier + result[i-1]
	}

	return result
}
