package v2

import (
	"context"
	"errors"
	"math"
	"testing"

	"quant-system/internal/risk"
	"quant-system/pkg/contracts"
)

// scriptedStrategy emits pre-programmed intents at specific event indices.
// This lets tests decouple backtest orchestration from any particular
// signal-generation logic. Intent.Price is auto-filled from the event's
// LastPX so risk (which rejects non-positive prices) treats the scripted
// intent like a real one.
type scriptedStrategy struct {
	id      string
	actions map[int]contracts.OrderIntent // event index → intent template
	cursor  int
}

func (s *scriptedStrategy) ID() string { return s.id }

func (s *scriptedStrategy) OnMarket(evt contracts.MarketNormalizedEvent) []contracts.OrderIntent {
	i := s.cursor
	s.cursor++
	tmpl, ok := s.actions[i]
	if !ok {
		return nil
	}
	intent := tmpl
	if intent.Price <= 0 {
		intent.Price = evt.LastPX
	}
	if intent.Symbol == "" {
		intent.Symbol = evt.Symbol
	}
	return []contracts.OrderIntent{intent}
}

// linearRampDataset builds events with monotonically rising LastPX to give
// the strategy a predictable profit on a held long position.
func linearRampDataset(n int, startPx, stepPx float64) Dataset {
	evs := make([]contracts.MarketNormalizedEvent, n)
	baseTS := int64(1_700_000_000_000) // arbitrary fixed epoch
	for i := 0; i < n; i++ {
		px := startPx + float64(i)*stepPx
		evs[i] = contracts.MarketNormalizedEvent{
			Symbol:   "BTCUSDT",
			BidPX:    px - 0.5,
			AskPX:    px + 0.5,
			LastPX:   px,
			EmitTSMS: baseTS + int64(i)*60_000, // 1 minute apart
		}
	}
	return Dataset{Name: "linear-ramp", Events: evs}
}

func TestRun_EmptyDatasetRejected(t *testing.T) {
	_, err := Run(context.Background(), Config{
		Strategy: &scriptedStrategy{id: "s"},
		Dataset:  Dataset{},
	})
	if !errors.Is(err, ErrEmptyDataset) {
		t.Fatalf("err=%v want ErrEmptyDataset", err)
	}
}

func TestRun_NilStrategyRejected(t *testing.T) {
	_, err := Run(context.Background(), Config{
		Dataset: linearRampDataset(2, 100, 0),
	})
	if !errors.Is(err, ErrStrategyNil) {
		t.Fatalf("err=%v want ErrStrategyNil", err)
	}
}

func TestRun_BuyHoldSell_ProfitOnUptrend(t *testing.T) {
	ds := linearRampDataset(20, 100, 1) // 100 → 119

	strat := &scriptedStrategy{
		id: "buy-hold-sell",
		actions: map[int]contracts.OrderIntent{
			2: {IntentID: "buy-1", Symbol: "BTCUSDT", Side: "buy", Price: 0, Quantity: 1},
			18: {IntentID: "sell-1", Symbol: "BTCUSDT", Side: "sell", Price: 0, Quantity: 1},
		},
	}

	res, err := Run(context.Background(), Config{
		AccountID:   "acct",
		Strategy:    strat,
		Dataset:     ds,
		StartEquity: 1000,
		Risk:        risk.Config{}, // no limits
		// no slippage, no fee — tests the bookkeeping math
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if res.Intents != 2 || res.Fills != 2 || res.Rejects != 0 {
		t.Fatalf("counts: intents=%d fills=%d rejects=%d want 2/2/0",
			res.Intents, res.Fills, res.Rejects)
	}
	if len(res.Trades) != 2 {
		t.Fatalf("trades=%d want 2", len(res.Trades))
	}

	// Buy @ ask(102+0.5)=102.5, sell @ bid(118-0.5)=117.5 → realised = 15 per unit.
	buy := res.Trades[0]
	sell := res.Trades[1]
	if math.Abs(buy.FillPrice-102.5) > 1e-9 {
		t.Fatalf("buy px=%v want 102.5", buy.FillPrice)
	}
	if math.Abs(sell.FillPrice-117.5) > 1e-9 {
		t.Fatalf("sell px=%v want 117.5", sell.FillPrice)
	}

	// Final equity: start 1000 - 102.5 + 117.5 = 1015.
	if math.Abs(res.Metrics.FinalEquity-1015) > 1e-9 {
		t.Fatalf("final equity=%v want 1015", res.Metrics.FinalEquity)
	}
	if res.Metrics.TotalReturn <= 0 {
		t.Fatalf("total return=%v want >0", res.Metrics.TotalReturn)
	}
	if res.Metrics.WinRate != 1.0 {
		t.Fatalf("win rate=%v want 1.0", res.Metrics.WinRate)
	}
	if res.Metrics.NumTrades != 2 {
		t.Fatalf("num trades=%d want 2", res.Metrics.NumTrades)
	}

	// Equity curve has initial + one per event.
	if len(res.Equity) != len(ds.Events)+1 {
		t.Fatalf("equity points=%d want %d", len(res.Equity), len(ds.Events)+1)
	}
	// First point matches StartEquity.
	if res.Equity[0].Cash != 1000 || res.Equity[0].MarkToMarket != 1000 {
		t.Fatalf("seed equity %+v", res.Equity[0])
	}
	// Right after the buy (event index 2), we hold 1 unit of BTC. Cash should
	// have dropped; MTM reflects the held inventory at the latest LastPX.
	// Equity slice index = event_index + 1 (because seed point is at index 0).
	afterBuy := res.Equity[3]
	if afterBuy.Cash > 900 {
		t.Fatalf("cash after buy=%v, should have dropped significantly", afterBuy.Cash)
	}
}

func TestRun_RiskRejection(t *testing.T) {
	ds := linearRampDataset(5, 100, 0)
	strat := &scriptedStrategy{
		id: "reject-test",
		actions: map[int]contracts.OrderIntent{
			1: {IntentID: "too-big", Symbol: "BTCUSDT", Side: "buy", Price: 0, Quantity: 10},
		},
	}
	res, err := Run(context.Background(), Config{
		Strategy:    strat,
		Dataset:     ds,
		StartEquity: 1000,
		Risk:        risk.Config{MaxOrderQty: 1},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.Intents != 1 || res.Rejects != 1 || res.Fills != 0 {
		t.Fatalf("counts: intents=%d rejects=%d fills=%d", res.Intents, res.Rejects, res.Fills)
	}
	if len(res.Trades) != 0 {
		t.Fatalf("trades=%d want 0", len(res.Trades))
	}
}

func TestRun_FeesReduceReturn(t *testing.T) {
	ds := linearRampDataset(20, 100, 1)
	strat := &scriptedStrategy{
		id: "fee-test",
		actions: map[int]contracts.OrderIntent{
			2:  {IntentID: "b", Symbol: "BTCUSDT", Side: "buy", Price: 0, Quantity: 1},
			18: {IntentID: "s", Symbol: "BTCUSDT", Side: "sell", Price: 0, Quantity: 1},
		},
	}
	resNoFee, _ := Run(context.Background(), Config{
		Strategy:    strat,
		Dataset:     ds,
		StartEquity: 1000,
	})

	strat2 := &scriptedStrategy{
		id:      "fee-test",
		actions: map[int]contracts.OrderIntent{
			2:  {IntentID: "b", Symbol: "BTCUSDT", Side: "buy", Price: 0, Quantity: 1},
			18: {IntentID: "s", Symbol: "BTCUSDT", Side: "sell", Price: 0, Quantity: 1},
		},
	}
	resWithFee, _ := Run(context.Background(), Config{
		Strategy:    strat2,
		Dataset:     ds,
		StartEquity: 1000,
		Matcher:     SimMatcherConfig{TakerFeeBps: 100}, // 1%
	})

	if resWithFee.Metrics.FinalEquity >= resNoFee.Metrics.FinalEquity {
		t.Fatalf("fees should reduce equity: no-fee=%v with-fee=%v",
			resNoFee.Metrics.FinalEquity, resWithFee.Metrics.FinalEquity)
	}
}
