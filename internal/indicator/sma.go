package indicator

// SMA calculates Simple Moving Average.
// Returns a slice of same length as input. First (period-1) values are 0.
func SMA(closes []float64, period int) []float64 {
	n := len(closes)
	if n == 0 || period <= 0 || period > n {
		return make([]float64, n)
	}

	result := make([]float64, n)

	// Calculate initial window sum.
	var sum float64
	for i := 0; i < period; i++ {
		sum += closes[i]
	}
	result[period-1] = sum / float64(period)

	// Slide the window.
	for i := period; i < n; i++ {
		sum += closes[i] - closes[i-period]
		result[i] = sum / float64(period)
	}

	return result
}
