package indicator

// MACDResult holds the three MACD output lines.
type MACDResult struct {
	MACD      []float64 // MACD line (fast EMA - slow EMA)
	Signal    []float64 // Signal line (EMA of MACD)
	Histogram []float64 // MACD - Signal
}

// MACD calculates Moving Average Convergence Divergence.
// Standard parameters: fast=12, slow=26, signal=9.
func MACD(closes []float64, fast, slow, signal int) MACDResult {
	n := len(closes)
	if n == 0 {
		return MACDResult{
			MACD:      []float64{},
			Signal:    []float64{},
			Histogram: []float64{},
		}
	}

	fastEMA := EMA(closes, fast)
	slowEMA := EMA(closes, slow)

	// MACD line = fast EMA - slow EMA.
	macdLine := make([]float64, n)
	for i := 0; i < n; i++ {
		macdLine[i] = fastEMA[i] - slowEMA[i]
	}

	// Signal line = EMA of MACD line (using signal period).
	// Only meaningful after slow EMA is seeded (index slow-1 onward).
	signalLine := make([]float64, n)
	signalMultiplier := 2.0 / float64(signal+1)

	// Seed signal EMA with SMA of first `signal` MACD values starting from slow-1.
	startIdx := slow - 1
	if startIdx+signal > n {
		return MACDResult{
			MACD:      macdLine,
			Signal:    signalLine,
			Histogram: make([]float64, n),
		}
	}

	var sum float64
	for i := startIdx; i < startIdx+signal; i++ {
		sum += macdLine[i]
	}
	signalLine[startIdx+signal-1] = sum / float64(signal)

	for i := startIdx + signal; i < n; i++ {
		signalLine[i] = (macdLine[i]-signalLine[i-1])*signalMultiplier + signalLine[i-1]
	}

	// Histogram = MACD - Signal.
	histogram := make([]float64, n)
	for i := startIdx + signal - 1; i < n; i++ {
		histogram[i] = macdLine[i] - signalLine[i]
	}

	return MACDResult{
		MACD:      macdLine,
		Signal:    signalLine,
		Histogram: histogram,
	}
}
