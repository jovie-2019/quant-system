package optimizer

import (
	"math"

	v2 "quant-system/internal/backtest/v2"
)

// ObjectiveFn scores a completed backtest. Higher is better. The optimiser
// treats +Inf/NaN as invalid and ranks such trials below all finite scores.
type ObjectiveFn func(v2.Metrics) float64

// ObjectivePreset names the handful of canned scoring functions exposed
// to the UI. Strings — not enum values — keep the JSON API forward-
// compatible with new presets.
type ObjectivePreset string

const (
	// ObjectiveSharpePenaltyDD is the default: Sharpe minus a penalty that
	// switches on when MDD exceeds 20%, scaled by 2. Rewards smooth
	// curves, disincentivises large drawdowns without discarding them.
	ObjectiveSharpePenaltyDD ObjectivePreset = "sharpe_penalty_dd"

	// ObjectiveTotalReturn is unadjusted total return. Use when the
	// dataset is short enough that Sharpe is unreliable.
	ObjectiveTotalReturn ObjectivePreset = "total_return"

	// ObjectiveCalmar = total return / |MDD|. Favours return per unit of
	// worst-case pain.
	ObjectiveCalmar ObjectivePreset = "calmar"

	// ObjectiveProfitFactor = sum(wins) / |sum(losses)|. Capped to avoid
	// +Inf when there are no losses.
	ObjectiveProfitFactor ObjectivePreset = "profit_factor"
)

// MinTrades is the minimum number of round-trip trades required for a
// trial to be eligible for a positive objective score. Below this, the
// objective returns a large negative sentinel so the optimiser never
// picks a "lucky no-trade" configuration as optimal.
const MinTrades = 5

// isFinite reports whether x is a finite number (neither ±Inf nor NaN).
// Go's math package lacks a built-in helper so we define one locally.
func isFinite(x float64) bool {
	return !math.IsNaN(x) && !math.IsInf(x, 0)
}

// NewObjective returns the ObjectiveFn for the given preset.
func NewObjective(preset ObjectivePreset) ObjectiveFn {
	switch preset {
	case ObjectiveTotalReturn:
		return totalReturn
	case ObjectiveCalmar:
		return calmar
	case ObjectiveProfitFactor:
		return profitFactor
	case ObjectiveSharpePenaltyDD, "":
		return sharpePenaltyDD
	}
	return sharpePenaltyDD
}

// invalidScore is the floor for disqualified trials. It sorts below any
// realistic finite score so skipped configs never win by default.
const invalidScore = -1e9

func sharpePenaltyDD(m v2.Metrics) float64 {
	if m.NumTrades < MinTrades {
		return invalidScore
	}
	if !isFinite(m.Sharpe) {
		return invalidScore
	}
	ddPenalty := 2 * math.Max(m.MaxDrawdown-0.20, 0)
	return m.Sharpe - ddPenalty
}

func totalReturn(m v2.Metrics) float64 {
	if m.NumTrades < MinTrades || !isFinite(m.TotalReturn) {
		return invalidScore
	}
	return m.TotalReturn
}

func calmar(m v2.Metrics) float64 {
	if m.NumTrades < MinTrades {
		return invalidScore
	}
	// Metrics.Calmar already sanitised to 1e18 on Inf — treat those as
	// invalid to avoid biasing toward zero-drawdown fluke runs.
	if !isFinite(m.Calmar) || math.Abs(m.Calmar) >= 1e17 {
		return invalidScore
	}
	return m.Calmar
}

func profitFactor(m v2.Metrics) float64 {
	if m.NumTrades < MinTrades {
		return invalidScore
	}
	if !isFinite(m.ProfitFactor) || math.Abs(m.ProfitFactor) >= 1e17 {
		return 10.0 // cap "infinite PF" so it still beats typical values but is finite
	}
	return m.ProfitFactor
}
