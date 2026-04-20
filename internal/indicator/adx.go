package indicator

import "math"

// ADXResult holds ADX together with the two directional indicators so
// callers can derive trend direction (+DI > -DI for up trends) without a
// second pass.
type ADXResult struct {
	ADX     []float64
	PlusDI  []float64
	MinusDI []float64
}

// ADX computes Welles Wilder's Average Directional Index with the
// customary +DI / -DI components. The smoothing follows Wilder's original
// formulation (also used by TradingView): an initial sum over the first
// `period` bars, then an RMA-style update thereafter.
//
// Values before index `2*period-1` are zero because both a +DM/-DM smoothing
// window and a subsequent DX smoothing window must fill before the ADX line
// is meaningful.
func ADX(highs, lows, closes []float64, period int) ADXResult {
	n := len(closes)
	empty := ADXResult{
		ADX: make([]float64, n), PlusDI: make([]float64, n), MinusDI: make([]float64, n),
	}
	if n == 0 || len(highs) != n || len(lows) != n || period <= 0 || 2*period > n {
		return empty
	}

	plusDM := make([]float64, n)
	minusDM := make([]float64, n)
	tr := make([]float64, n)

	for i := 1; i < n; i++ {
		upMove := highs[i] - highs[i-1]
		downMove := lows[i-1] - lows[i]
		if upMove > downMove && upMove > 0 {
			plusDM[i] = upMove
		}
		if downMove > upMove && downMove > 0 {
			minusDM[i] = downMove
		}
		a := highs[i] - lows[i]
		b := math.Abs(highs[i] - closes[i-1])
		c := math.Abs(lows[i] - closes[i-1])
		tr[i] = math.Max(a, math.Max(b, c))
	}

	// Wilder-smoothed running sums over `period` bars.
	smPlus := make([]float64, n)
	smMinus := make([]float64, n)
	smTR := make([]float64, n)

	// Seed the sums at index `period` (indices 1..period inclusive summed).
	var sumP, sumM, sumT float64
	for i := 1; i <= period; i++ {
		sumP += plusDM[i]
		sumM += minusDM[i]
		sumT += tr[i]
	}
	smPlus[period] = sumP
	smMinus[period] = sumM
	smTR[period] = sumT
	for i := period + 1; i < n; i++ {
		smPlus[i] = smPlus[i-1] - smPlus[i-1]/float64(period) + plusDM[i]
		smMinus[i] = smMinus[i-1] - smMinus[i-1]/float64(period) + minusDM[i]
		smTR[i] = smTR[i-1] - smTR[i-1]/float64(period) + tr[i]
	}

	plusDI := make([]float64, n)
	minusDI := make([]float64, n)
	dx := make([]float64, n)
	for i := period; i < n; i++ {
		if smTR[i] > 0 {
			plusDI[i] = 100 * smPlus[i] / smTR[i]
			minusDI[i] = 100 * smMinus[i] / smTR[i]
		}
		sumDI := plusDI[i] + minusDI[i]
		if sumDI > 0 {
			dx[i] = 100 * math.Abs(plusDI[i]-minusDI[i]) / sumDI
		}
	}

	adx := make([]float64, n)
	startADX := 2*period - 1
	// Seed ADX with the simple average of the first `period` DX values.
	var sumDX float64
	for i := period; i < startADX+1 && i < n; i++ {
		sumDX += dx[i]
	}
	if startADX < n {
		adx[startADX] = sumDX / float64(period)
		for i := startADX + 1; i < n; i++ {
			adx[i] = (adx[i-1]*float64(period-1) + dx[i]) / float64(period)
		}
	}

	return ADXResult{ADX: adx, PlusDI: plusDI, MinusDI: minusDI}
}
