package regime

import (
	"math"

	"quant-system/internal/indicator"
	"quant-system/pkg/contracts"
)

// Thresholds are the knobs of the threshold-based classifier. Defaults
// come from conventional quant literature (Wilder 1978, Hurst 1951) and
// from empirical spot-crypto experience:
//
//	ADXTrend        25  — a 25+ ADX is the classic "trending" cutoff
//	ADXRange        20  — below 20 is widely considered non-trending
//	HurstPersistent 0.55— modest persistence confirms a trend label
//	HurstMeanRevert 0.45— modest anti-persistence confirms a range label
//	ATRPercentHigh  0.02— 2% ATR/price is a loud volatility spike on 1m-1h bars
//	VolumeRatioLow  0.25— last-bar volume under 25% of the window mean flags thin liquidity
//
// Operators can tune per-symbol and persist tuned values alongside the
// strategy config later; for the MVP these constants suffice.
type Thresholds struct {
	ADXTrend        float64
	ADXRange        float64
	HurstPersistent float64
	HurstMeanRevert float64
	ATRPercentHigh  float64
	VolumeRatioLow  float64
}

// DefaultThresholds returns the conventional starting point described above.
func DefaultThresholds() Thresholds {
	return Thresholds{
		ADXTrend:        25,
		ADXRange:        20,
		HurstPersistent: 0.55,
		HurstMeanRevert: 0.45,
		ATRPercentHigh:  0.02,
		VolumeRatioLow:  0.25,
	}
}

// ClassifyConfig parameterises ClassifyKlines.
type ClassifyConfig struct {
	ADXPeriod      int // default 14
	ATRPeriod      int // default 14
	BBPeriod       int // default 20
	BBStdDev       float64
	HurstMinN      int // R/S min window size; default 8
	HurstLookback  int // samples of returns fed to Hurst; default 128
	Thresholds     Thresholds
	// MinWarmupBars is the earliest bar index that produces a Record.
	// Defaults to max(ADXPeriod*2, BBPeriod, HurstLookback+1).
	MinWarmupBars int
}

// defaults applies conventional parameter choices.
func (c *ClassifyConfig) defaults() {
	if c.ADXPeriod <= 0 {
		c.ADXPeriod = 14
	}
	if c.ATRPeriod <= 0 {
		c.ATRPeriod = 14
	}
	if c.BBPeriod <= 0 {
		c.BBPeriod = 20
	}
	if c.BBStdDev <= 0 {
		c.BBStdDev = 2.0
	}
	if c.HurstMinN <= 0 {
		c.HurstMinN = 8
	}
	if c.HurstLookback <= 0 {
		c.HurstLookback = 128
	}
	if c.Thresholds == (Thresholds{}) {
		c.Thresholds = DefaultThresholds()
	}
	if c.MinWarmupBars <= 0 {
		c.MinWarmupBars = maxInt(c.ADXPeriod*2, c.BBPeriod, c.HurstLookback+1)
	}
}

// ClassifyKlines runs the threshold-based classifier over an ordered
// (ASCending OpenTime) slice of closed klines and returns one Record per
// bar that clears the warmup window. The caller is responsible for
// providing bars from a single (venue, symbol, interval) triple.
func ClassifyKlines(klines []contracts.Kline, cfg ClassifyConfig) []Record {
	cfg.defaults()
	n := len(klines)
	if n < cfg.MinWarmupBars {
		return nil
	}

	highs := make([]float64, n)
	lows := make([]float64, n)
	closes := make([]float64, n)
	vols := make([]float64, n)
	for i, k := range klines {
		highs[i] = k.High
		lows[i] = k.Low
		closes[i] = k.Close
		vols[i] = k.Volume
	}

	adx := indicator.ADX(highs, lows, closes, cfg.ADXPeriod)
	atr := indicator.ATR(highs, lows, closes, cfg.ATRPeriod)
	bb := indicator.Bollinger(closes, cfg.BBPeriod, cfg.BBStdDev)

	out := make([]Record, 0, n-cfg.MinWarmupBars+1)
	for i := cfg.MinWarmupBars; i < n; i++ {
		// Hurst over a trailing window of log-returns.
		lbStart := i - cfg.HurstLookback
		if lbStart < 0 {
			lbStart = 0
		}
		hurst := indicator.HurstFromPrices(closes[lbStart:i+1], cfg.HurstMinN)

		bbw := 0.0
		if bb.Middle[i] > 0 {
			bbw = (bb.Upper[i] - bb.Lower[i]) / bb.Middle[i]
		}
		atrPct := 0.0
		if closes[i] > 0 {
			atrPct = atr[i] / closes[i]
		}

		ret := 0.0
		if closes[i-1] > 0 && closes[i] > 0 {
			ret = math.Log(closes[i] / closes[i-1])
		}

		volMean := meanLast(vols, i+1, cfg.BBPeriod)

		feat := Features{
			ADX:        adx.ADX[i],
			PlusDI:     adx.PlusDI[i],
			MinusDI:    adx.MinusDI[i],
			ATR:        atr[i],
			ATRPercent: atrPct,
			BBW:        bbw,
			Hurst:      hurst,
			LastClose:  closes[i],
			ReturnLast: ret,
			VolumeLast: vols[i],
			VolumeMean: volMean,
		}
		label, conf := classifyThreshold(feat, cfg.Thresholds)
		out = append(out, Record{
			Venue:      string(klines[i].Venue),
			Symbol:     klines[i].Symbol,
			Interval:   klines[i].Interval,
			BarTime:    klines[i].CloseTime,
			Method:     MethodThreshold,
			Regime:     label,
			Confidence: conf,
			Features:   feat,
		})
	}
	return out
}

// classifyThreshold is the pure decision function; exported only via
// ClassifyFeatures so tests can exercise the logic without a full kline
// series. Order of checks:
//
//  1. Liquidity — if last bar volume is well under window mean, flag LowLiq.
//  2. Volatility spike — ATR/price above threshold overrides anything else.
//  3. Trend — ADX above the trend threshold + Hurst confirms ⇒ trend_up or
//     trend_down based on +DI vs -DI.
//  4. Range — ADX below the range threshold + Hurst confirms mean reversion.
//  5. Otherwise — unknown (a "transition" state).
func classifyThreshold(f Features, t Thresholds) (Regime, float64) {
	// 1. Liquidity.
	if f.VolumeMean > 0 && t.VolumeRatioLow > 0 {
		if f.VolumeLast/f.VolumeMean < t.VolumeRatioLow {
			// Confidence scales with how far below the ratio we are.
			ratio := f.VolumeLast / f.VolumeMean
			conf := clamp01((t.VolumeRatioLow - ratio) / t.VolumeRatioLow)
			return RegimeLowLiq, conf
		}
	}

	// 2. Volatility spike.
	if t.ATRPercentHigh > 0 && f.ATRPercent > t.ATRPercentHigh {
		conf := clamp01((f.ATRPercent - t.ATRPercentHigh) / t.ATRPercentHigh)
		return RegimeHighVol, conf
	}

	// 3. Trend.
	if f.ADX >= t.ADXTrend && f.Hurst >= t.HurstPersistent {
		// Confidence: average of (ADX margin over threshold normalised to 1)
		// and (Hurst margin above 0.5 normalised to 0.5).
		adxMargin := clamp01((f.ADX - t.ADXTrend) / math.Max(t.ADXTrend, 1))
		hurstMargin := clamp01((f.Hurst - 0.5) / 0.5)
		conf := (adxMargin + hurstMargin) / 2
		if f.PlusDI >= f.MinusDI {
			return RegimeTrendUp, conf
		}
		return RegimeTrendDown, conf
	}

	// 4. Range.
	if f.ADX < t.ADXRange && f.Hurst <= t.HurstMeanRevert {
		adxMargin := clamp01((t.ADXRange - f.ADX) / math.Max(t.ADXRange, 1))
		hurstMargin := clamp01((0.5 - f.Hurst) / 0.5)
		conf := (adxMargin + hurstMargin) / 2
		return RegimeRange, conf
	}

	// 5. Transitional / indeterminate.
	return RegimeUnknown, 0
}

// ClassifyFeatures exposes the pure decision function for callers who have
// precomputed Features (e.g. an adapter fed by the Python service).
func ClassifyFeatures(f Features, t Thresholds) (Regime, float64) {
	return classifyThreshold(f, t)
}

func meanLast(xs []float64, endExclusive, window int) float64 {
	if endExclusive <= 0 || window <= 0 {
		return 0
	}
	start := endExclusive - window
	if start < 0 {
		start = 0
	}
	var sum float64
	cnt := 0
	for i := start; i < endExclusive; i++ {
		sum += xs[i]
		cnt++
	}
	if cnt == 0 {
		return 0
	}
	return sum / float64(cnt)
}

func clamp01(x float64) float64 {
	if math.IsNaN(x) || x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

func maxInt(a, b, c int) int {
	m := a
	if b > m {
		m = b
	}
	if c > m {
		m = c
	}
	return m
}
