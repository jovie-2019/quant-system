package v2

import (
	"math"
	"testing"

	"quant-system/pkg/contracts"
)

func TestComputeMetrics_Empty(t *testing.T) {
	m := ComputeMetrics(nil, nil)
	if m.NumTrades != 0 || m.FinalEquity != 0 || m.Sharpe != 0 {
		t.Fatalf("empty metrics non-zero: %+v", m)
	}
}

func TestComputeMetrics_MonotonicGrowth(t *testing.T) {
	eq := []EquityPoint{
		{TSMS: 0, MarkToMarket: 100},
		{TSMS: 86_400_000, MarkToMarket: 101}, // +1 day
		{TSMS: 172_800_000, MarkToMarket: 102},
		{TSMS: 259_200_000, MarkToMarket: 103},
	}
	m := ComputeMetrics(eq, nil)

	if m.FinalEquity != 103 {
		t.Fatalf("final=%v want 103", m.FinalEquity)
	}
	if math.Abs(m.TotalReturn-0.03) > 1e-9 {
		t.Fatalf("total return=%v want 0.03", m.TotalReturn)
	}
	// No drawdown on monotonic rise.
	if m.MaxDrawdown != 0 {
		t.Fatalf("mdd=%v want 0", m.MaxDrawdown)
	}
	// Monotonic positive returns → positive Sharpe (very high for this toy curve).
	if m.Sharpe <= 0 {
		t.Fatalf("sharpe=%v want >0", m.Sharpe)
	}
}

func TestComputeMetrics_Drawdown(t *testing.T) {
	eq := []EquityPoint{
		{TSMS: 0, MarkToMarket: 100},
		{TSMS: 1, MarkToMarket: 120},
		{TSMS: 2, MarkToMarket: 90}, // 25% drawdown from peak 120
		{TSMS: 3, MarkToMarket: 110},
	}
	m := ComputeMetrics(eq, nil)
	wantDD := 1.0 - 90.0/120.0
	if math.Abs(m.MaxDrawdown-wantDD) > 1e-9 {
		t.Fatalf("mdd=%v want %v", m.MaxDrawdown, wantDD)
	}
	// Calmar = total return / mdd = 0.10 / 0.25 = 0.4
	wantCalmar := 0.10 / wantDD
	if math.Abs(m.Calmar-wantCalmar) > 1e-9 {
		t.Fatalf("calmar=%v want %v", m.Calmar, wantCalmar)
	}
}

func TestComputeMetrics_WinRateAndProfitFactor(t *testing.T) {
	trades := []contracts.TradeFillEvent{
		{Symbol: "X", Side: "buy", FillPrice: 100, FillQty: 1, Fee: 0},
		{Symbol: "X", Side: "sell", FillPrice: 110, FillQty: 1, Fee: 0}, // +10
		{Symbol: "X", Side: "buy", FillPrice: 100, FillQty: 1, Fee: 0},
		{Symbol: "X", Side: "sell", FillPrice: 95, FillQty: 1, Fee: 0}, // -5
		{Symbol: "X", Side: "buy", FillPrice: 100, FillQty: 1, Fee: 0},
		{Symbol: "X", Side: "sell", FillPrice: 103, FillQty: 1, Fee: 0}, // +3
	}
	m := ComputeMetrics(nil, trades)
	// 2 wins out of 3 round trips.
	if math.Abs(m.WinRate-2.0/3.0) > 1e-9 {
		t.Fatalf("win rate=%v want 2/3", m.WinRate)
	}
	// PF = (10 + 3) / 5 = 2.6
	if math.Abs(m.ProfitFactor-2.6) > 1e-9 {
		t.Fatalf("pf=%v want 2.6", m.ProfitFactor)
	}
}

func TestComputeMetrics_ProfitFactorInfiniteWhenNoLosses(t *testing.T) {
	trades := []contracts.TradeFillEvent{
		{Symbol: "X", Side: "buy", FillPrice: 100, FillQty: 1},
		{Symbol: "X", Side: "sell", FillPrice: 110, FillQty: 1},
	}
	m := ComputeMetrics(nil, trades)
	if !math.IsInf(m.ProfitFactor, 1) {
		t.Fatalf("pf=%v want +Inf (no losses)", m.ProfitFactor)
	}
}

func TestComputeMetrics_TurnoverScalesWithNotional(t *testing.T) {
	eq := []EquityPoint{
		{TSMS: 0, MarkToMarket: 1000},
		{TSMS: 1, MarkToMarket: 1000},
		{TSMS: 2, MarkToMarket: 1000},
	}
	trades := []contracts.TradeFillEvent{
		{Symbol: "X", Side: "buy", FillPrice: 100, FillQty: 5},  // notional 500
		{Symbol: "X", Side: "sell", FillPrice: 100, FillQty: 5}, // notional 500
	}
	m := ComputeMetrics(eq, trades)
	// Turnover = 1000 / 1000 = 1.0
	if math.Abs(m.Turnover-1.0) > 1e-9 {
		t.Fatalf("turnover=%v want 1.0", m.Turnover)
	}
}
