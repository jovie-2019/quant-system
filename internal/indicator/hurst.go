package indicator

import "math"

// HurstFromPrices converts prices to log-returns and delegates to Hurst.
// Use this when the caller has a price series and wants regime-style
// interpretation (H>0.5 → momentum, H<0.5 → mean reversion).
func HurstFromPrices(prices []float64, minN int) float64 {
	if len(prices) < 2 {
		return 0
	}
	returns := make([]float64, 0, len(prices)-1)
	for i := 1; i < len(prices); i++ {
		if prices[i-1] <= 0 || prices[i] <= 0 {
			continue
		}
		returns = append(returns, math.Log(prices[i]/prices[i-1]))
	}
	return Hurst(returns, minN)
}

// Hurst estimates the Hurst exponent of a stationary series via
// rescaled-range (R/S) analysis. Interpreting the result requires the
// input to be stationary:
//
//	H ≈ 0.5  — uncorrelated (e.g. i.i.d. returns from a random walk)
//	H > 0.5  — persistent (today's move tends to continue)
//	H < 0.5  — anti-persistent (mean-reverting)
//
// CALLER CONTRACT: for a price series, convert to log-returns first —
// applying R/S to non-stationary levels (raw prices) yields values close
// to 1 regardless of the underlying dynamics. The helper HurstFromPrices
// in this package performs the conversion when a prices-in convenience is
// desired.
//
// The implementation splits the input into non-overlapping windows of
// length n ∈ {minN, 2*minN, ...} up to len(series)/2, computes the
// rescaled range for each window size, and fits log(R/S) ≈ H·log(n) by
// ordinary least squares. Typical minN in the literature is 8 or 10.
func Hurst(series []float64, minN int) float64 {
	n := len(series)
	if minN < 4 {
		minN = 4
	}
	if n < 2*minN {
		return 0
	}

	var logs, lrs []float64
	for size := minN; size <= n/2; size *= 2 {
		rs := meanRescaledRange(series, size)
		if rs <= 0 {
			continue
		}
		logs = append(logs, math.Log(float64(size)))
		lrs = append(lrs, math.Log(rs))
	}
	if len(logs) < 2 {
		return 0
	}

	// OLS slope of lrs vs logs.
	var sumX, sumY, sumXY, sumXX float64
	k := float64(len(logs))
	for i := range logs {
		sumX += logs[i]
		sumY += lrs[i]
		sumXY += logs[i] * lrs[i]
		sumXX += logs[i] * logs[i]
	}
	denom := k*sumXX - sumX*sumX
	if denom == 0 {
		return 0
	}
	return (k*sumXY - sumX*sumY) / denom
}

// meanRescaledRange averages the rescaled range R/S over non-overlapping
// windows of the given size.
func meanRescaledRange(series []float64, size int) float64 {
	numWindows := len(series) / size
	if numWindows == 0 {
		return 0
	}
	var total float64
	var count int
	for w := 0; w < numWindows; w++ {
		window := series[w*size : (w+1)*size]
		rs := rescaledRange(window)
		if rs > 0 {
			total += rs
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return total / float64(count)
}

// rescaledRange computes R/S for a single window: the range of cumulative
// mean-adjusted series divided by its standard deviation.
func rescaledRange(window []float64) float64 {
	n := len(window)
	if n < 2 {
		return 0
	}

	// Mean.
	var mean float64
	for _, v := range window {
		mean += v
	}
	mean /= float64(n)

	// Cumulative deviations and sum of squared deviations (for stddev).
	var minC, maxC, cumDev, sse float64
	minC = math.Inf(1)
	maxC = math.Inf(-1)
	for _, v := range window {
		dev := v - mean
		cumDev += dev
		if cumDev < minC {
			minC = cumDev
		}
		if cumDev > maxC {
			maxC = cumDev
		}
		sse += dev * dev
	}
	std := math.Sqrt(sse / float64(n))
	if std == 0 {
		return 0
	}
	return (maxC - minC) / std
}
