// Package regime classifies a price series into one of a small set of
// market-state labels (trend up, trend down, range, high volatility, low
// liquidity). Classification is driven by interpretable features
// (ADX, ATR-as-percent-of-price, Bollinger bandwidth, Hurst exponent).
//
// This threshold-based classifier is the MVP; it lives next to a future
// Python service (services/research/regime/) that will produce GMM- and
// HMM-derived labels on the same contract. Both paths emit Record objects
// and persist them through marketstore.RegimeStore.
package regime

// Regime is the coarse market-state label produced by a classifier.
type Regime string

const (
	RegimeTrendUp   Regime = "trend_up"
	RegimeTrendDown Regime = "trend_down"
	RegimeRange     Regime = "range"
	RegimeHighVol   Regime = "high_vol"
	RegimeLowLiq    Regime = "low_liq"
	RegimeUnknown   Regime = "unknown"
)

// Method identifies the algorithm that produced a Record. The Go-native
// classifier uses MethodThreshold; GMM and HMM will land with the Python
// service but the values are reserved here so the schema is stable.
type Method string

const (
	MethodThreshold Method = "threshold"
	MethodGMM       Method = "gmm"
	MethodHMM       Method = "hmm"
)

// Features are the numeric inputs a classifier consumes. Storing them
// alongside the label makes it trivial to explain a regime assignment
// after the fact without rerunning the classifier over raw bars.
type Features struct {
	ADX         float64 // trend strength (0-100); >25 → trending
	PlusDI      float64 // +DI component (direction hint)
	MinusDI     float64 // -DI component (direction hint)
	ATR         float64 // raw Average True Range
	ATRPercent  float64 // ATR / Close — volatility expressed as percent of price
	BBW         float64 // Bollinger band width / mid — (upper-lower)/mid
	Hurst       float64 // Hurst exponent on log-returns
	LastClose   float64 // reference price; zero if not provided
	ReturnLast  float64 // log(Close[i]/Close[i-1]) for direction hint on trending regimes
	VolumeLast  float64 // last bar volume; zero if not provided
	VolumeMean  float64 // rolling mean volume over the feature window
}

// Record is the per-bar output of a classifier, exactly matching the
// quant.regime_history ClickHouse schema so persistence is a straight copy.
type Record struct {
	Venue      string
	Symbol     string
	Interval   string
	BarTime    int64 // close_time in ms UTC
	Method     Method
	Regime     Regime
	Confidence float64 // 0..1 — for threshold method this is a margin score
	Features   Features
}
