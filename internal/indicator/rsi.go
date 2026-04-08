package indicator

// RSI calculates Relative Strength Index using Wilder's smoothing method.
// Returns values between 0 and 100. First `period` values are 0.
func RSI(closes []float64, period int) []float64 {
	n := len(closes)
	if n == 0 || period <= 0 || period >= n {
		return make([]float64, n)
	}

	result := make([]float64, n)

	// Step 1: Calculate price changes.
	gains := make([]float64, n)
	losses := make([]float64, n)
	for i := 1; i < n; i++ {
		change := closes[i] - closes[i-1]
		if change > 0 {
			gains[i] = change
		} else {
			losses[i] = -change
		}
	}

	// Step 2: First average gain/loss = SMA of first `period` changes.
	var avgGain, avgLoss float64
	for i := 1; i <= period; i++ {
		avgGain += gains[i]
		avgLoss += losses[i]
	}
	avgGain /= float64(period)
	avgLoss /= float64(period)

	if avgLoss == 0 {
		result[period] = 100
	} else {
		rs := avgGain / avgLoss
		result[period] = 100 - 100/(1+rs)
	}

	// Step 3: Wilder's smoothing for subsequent values.
	for i := period + 1; i < n; i++ {
		avgGain = (avgGain*float64(period-1) + gains[i]) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + losses[i]) / float64(period)

		if avgLoss == 0 {
			result[i] = 100
		} else {
			rs := avgGain / avgLoss
			result[i] = 100 - 100/(1+rs)
		}
	}

	return result
}
