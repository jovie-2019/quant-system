package v2

import (
	"encoding/json"
	"math"

	"quant-system/pkg/contracts"
)

// jsonInfSentinel is the value substituted for +Inf when encoding Metrics to
// JSON. Consumers that need to display "∞" can check for values >= this
// threshold. Negative infinity flips sign; NaN becomes zero.
const jsonInfSentinel = 1e18

// Metrics summarise the risk-adjusted performance of a backtest.
//
// Sharpe is annualised using a periods-per-year factor inferred from the
// median inter-event spacing of the equity curve. For backtests whose events
// arrive at irregular cadence the annualisation is approximate; treat Sharpe
// as a relative quality signal across runs on the same dataset, not as an
// absolute prediction of live performance.
type Metrics struct {
	FinalEquity  float64
	TotalReturn  float64
	MaxDrawdown  float64 // positive number; 0.15 means a 15% peak-to-trough drop
	Sharpe       float64
	Calmar       float64
	WinRate      float64 // fraction of round-trip sells with positive realised PnL
	ProfitFactor float64 // sum(positive realised PnL) / |sum(negative realised PnL)|
	Turnover     float64 // cumulative notional traded / average equity
	NumTrades    int     // total fill count (buys + sells)
}

// MarshalJSON substitutes non-finite floats with finite sentinels so the
// encoded JSON is always valid. It does NOT modify the in-memory Metrics,
// so Go-side consumers can still use math.IsInf on the raw field.
func (m Metrics) MarshalJSON() ([]byte, error) {
	type alias Metrics
	cp := alias(m)
	cp.FinalEquity = sanitizeFloat(cp.FinalEquity)
	cp.TotalReturn = sanitizeFloat(cp.TotalReturn)
	cp.MaxDrawdown = sanitizeFloat(cp.MaxDrawdown)
	cp.Sharpe = sanitizeFloat(cp.Sharpe)
	cp.Calmar = sanitizeFloat(cp.Calmar)
	cp.WinRate = sanitizeFloat(cp.WinRate)
	cp.ProfitFactor = sanitizeFloat(cp.ProfitFactor)
	cp.Turnover = sanitizeFloat(cp.Turnover)
	return json.Marshal(cp)
}

// sanitizeFloat maps non-finite values to finite sentinels; +Inf → 1e18,
// -Inf → -1e18, NaN → 0. Finite values are returned unchanged.
func sanitizeFloat(x float64) float64 {
	switch {
	case math.IsNaN(x):
		return 0
	case math.IsInf(x, 1):
		return jsonInfSentinel
	case math.IsInf(x, -1):
		return -jsonInfSentinel
	default:
		return x
	}
}

// ComputeMetrics derives performance metrics from an equity curve and trade
// log. It is pure: safe to call multiple times and with empty inputs. Trade
// stats (WinRate, ProfitFactor, NumTrades) are computed even when the equity
// curve is empty, since they only need the fill log.
func ComputeMetrics(equity []EquityPoint, trades []contracts.TradeFillEvent) Metrics {
	m := Metrics{NumTrades: len(trades)}
	m.WinRate, m.ProfitFactor = tradeStats(trades)

	if len(equity) == 0 {
		return m
	}

	start := equity[0].MarkToMarket
	end := equity[len(equity)-1].MarkToMarket
	m.FinalEquity = end
	if start > 0 {
		m.TotalReturn = end/start - 1.0
	}

	m.MaxDrawdown = maxDrawdown(equity)
	if m.MaxDrawdown > 0 {
		m.Calmar = m.TotalReturn / m.MaxDrawdown
	}

	m.Sharpe = annualisedSharpe(equity)

	if avg := averageMTM(equity); avg > 0 {
		notional := 0.0
		for _, t := range trades {
			notional += t.FillQty * t.FillPrice
		}
		m.Turnover = notional / avg
	}

	return m
}

func maxDrawdown(eq []EquityPoint) float64 {
	if len(eq) == 0 {
		return 0
	}
	peak := eq[0].MarkToMarket
	worst := 0.0
	for _, p := range eq {
		if p.MarkToMarket > peak {
			peak = p.MarkToMarket
		}
		if peak > 0 {
			dd := 1.0 - p.MarkToMarket/peak
			if dd > worst {
				worst = dd
			}
		}
	}
	return worst
}

func annualisedSharpe(eq []EquityPoint) float64 {
	if len(eq) < 3 {
		return 0
	}
	returns := make([]float64, 0, len(eq)-1)
	for i := 1; i < len(eq); i++ {
		prev := eq[i-1].MarkToMarket
		cur := eq[i].MarkToMarket
		if prev <= 0 {
			continue
		}
		returns = append(returns, cur/prev-1.0)
	}
	if len(returns) < 2 {
		return 0
	}
	mean, std := meanStd(returns)
	if std <= 0 {
		return 0
	}
	return (mean / std) * math.Sqrt(inferPeriodsPerYear(eq))
}

func meanStd(xs []float64) (mean, std float64) {
	if len(xs) == 0 {
		return 0, 0
	}
	sum := 0.0
	for _, x := range xs {
		sum += x
	}
	mean = sum / float64(len(xs))
	if len(xs) < 2 {
		return mean, 0
	}
	var sse float64
	for _, x := range xs {
		sse += (x - mean) * (x - mean)
	}
	std = math.Sqrt(sse / float64(len(xs)-1))
	return mean, std
}

func averageMTM(eq []EquityPoint) float64 {
	if len(eq) == 0 {
		return 0
	}
	sum := 0.0
	for _, p := range eq {
		sum += p.MarkToMarket
	}
	return sum / float64(len(eq))
}

// inferPeriodsPerYear estimates periods-per-year from the average inter-event
// spacing. Falls back to 252 (daily) when timestamps are unavailable.
func inferPeriodsPerYear(eq []EquityPoint) float64 {
	if len(eq) < 2 {
		return 252
	}
	totalMS := float64(eq[len(eq)-1].TSMS - eq[0].TSMS)
	if totalMS <= 0 {
		return 252
	}
	stepMS := totalMS / float64(len(eq)-1)
	if stepMS <= 0 {
		return 252
	}
	const yearMS = 365.25 * 24.0 * 3600.0 * 1000.0
	return yearMS / stepMS
}

// tradeStats walks the chronological trade log and derives realised PnL per
// round-trip using a moving weighted-average cost basis. Oversells (selling
// more than currently held, which well-formed strategies should never emit)
// are skipped for metric purposes.
func tradeStats(trades []contracts.TradeFillEvent) (winRate, profitFactor float64) {
	type book struct {
		qty, avgCost float64
	}
	ledger := make(map[string]*book)
	wins, losses := 0.0, 0.0
	winCount, totalRoundTrips := 0, 0

	for _, t := range trades {
		b := ledger[t.Symbol]
		if b == nil {
			b = &book{}
			ledger[t.Symbol] = b
		}
		switch t.Side {
		case "buy":
			totalCost := b.avgCost*b.qty + t.FillPrice*t.FillQty + t.Fee
			b.qty += t.FillQty
			if b.qty > 0 {
				b.avgCost = totalCost / b.qty
			}
		case "sell":
			if b.qty < t.FillQty {
				continue
			}
			realised := (t.FillPrice-b.avgCost)*t.FillQty - t.Fee
			b.qty -= t.FillQty
			if b.qty == 0 {
				b.avgCost = 0
			}
			totalRoundTrips++
			switch {
			case realised > 0:
				wins += realised
				winCount++
			case realised < 0:
				losses += -realised
			}
		}
	}

	if totalRoundTrips > 0 {
		winRate = float64(winCount) / float64(totalRoundTrips)
	}
	switch {
	case losses > 0:
		profitFactor = wins / losses
	case wins > 0:
		profitFactor = math.Inf(1)
	}
	return winRate, profitFactor
}
