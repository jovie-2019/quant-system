package v2

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"quant-system/internal/core"
	"quant-system/internal/execution"
	"quant-system/internal/orderfsm"
	"quant-system/internal/position"
	"quant-system/internal/risk"
	"quant-system/internal/strategy"
	"quant-system/pkg/contracts"
)

// Dataset is the chronological event stream driving one backtest.
type Dataset struct {
	Name   string
	Events []contracts.MarketNormalizedEvent
}

// Config controls a single backtest run. Zero values pick sensible defaults:
// AccountID "bt-default", StartEquity 10_000, no risk limits, no slippage/fee.
type Config struct {
	AccountID   string
	Strategy    strategy.Strategy
	Dataset     Dataset
	StartEquity float64
	Risk        risk.Config
	Matcher     SimMatcherConfig
}

// EquityPoint captures equity at one timestamp along the backtest timeline.
// Cash is the currency balance; MarkToMarket adds open positions valued at
// the most recent observed price.
type EquityPoint struct {
	TSMS         int64
	Cash         float64
	MarkToMarket float64
}

// Result summarises one backtest run: aggregate counters, the full equity
// curve, raw trade fills, risk decisions, and derived metrics.
type Result struct {
	StrategyID string
	Dataset    string

	Events  int
	Intents int
	Rejects int
	Fills   int

	StartedAt  time.Time
	FinishedAt time.Time
	Duration   time.Duration

	Equity    []EquityPoint
	Trades    []contracts.TradeFillEvent
	Decisions []contracts.RiskDecision
	Metrics   Metrics
}

// ErrStrategyNil is returned when Config.Strategy is not set.
var ErrStrategyNil = errors.New("backtest/v2: strategy is nil")

// ErrEmptyDataset is returned when Config.Dataset has no events.
var ErrEmptyDataset = errors.New("backtest/v2: empty dataset")

// Run executes a backtest end-to-end. It builds a disposable core.Engine
// wired to a SimMatcher + SimClock + MemorySink, replays the dataset, and
// returns aggregated metrics along with raw artifacts for deeper inspection.
func Run(ctx context.Context, cfg Config) (Result, error) {
	if cfg.Strategy == nil {
		return Result{}, ErrStrategyNil
	}
	if len(cfg.Dataset.Events) == 0 {
		return Result{}, ErrEmptyDataset
	}
	if cfg.AccountID == "" {
		cfg.AccountID = "bt-default"
	}
	if cfg.StartEquity <= 0 {
		cfg.StartEquity = 10_000
	}

	startedAt := time.Now()

	matcher := NewSimMatcher(cfg.Matcher)
	executor, err := execution.NewInMemoryExecutor(matcher)
	if err != nil {
		return Result{}, fmt.Errorf("backtest/v2: new executor: %w", err)
	}
	sink := core.NewMemorySink()
	ledger := position.NewInMemoryLedger()
	fsm := orderfsm.NewInMemoryStateMachine()
	riskEng := risk.NewInMemoryEngine(cfg.Risk)

	firstTS := cfg.Dataset.Events[0].EmitTSMS
	clock := core.NewSimClock(time.UnixMilli(firstTS))

	engine, err := core.NewEngine(riskEng, executor, fsm, ledger, core.Config{
		AccountID: cfg.AccountID,
		Sink:      sink,
		Clock:     clock,
	})
	if err != nil {
		return Result{}, fmt.Errorf("backtest/v2: new engine: %w", err)
	}

	res := Result{
		StrategyID: cfg.Strategy.ID(),
		Dataset:    strings.TrimSpace(cfg.Dataset.Name),
		Events:     len(cfg.Dataset.Events),
		StartedAt:  startedAt,
		Equity:     make([]EquityPoint, 0, len(cfg.Dataset.Events)+1),
	}

	cash := cfg.StartEquity
	positionQty := map[string]float64{}
	lastPx := map[string]float64{}

	// Seed equity with the starting cash at the first event's timestamp.
	res.Equity = append(res.Equity, EquityPoint{
		TSMS:         firstTS,
		Cash:         cash,
		MarkToMarket: cash,
	})

	for _, evt := range cfg.Dataset.Events {
		select {
		case <-ctx.Done():
			return finalize(res, sink, cash, positionQty, lastPx, startedAt), ctx.Err()
		default:
		}

		matcher.UpdateMarket(evt)
		clock.Set(time.UnixMilli(evt.EmitTSMS))
		sym := normSym(evt.Symbol)
		if evt.LastPX > 0 {
			lastPx[sym] = evt.LastPX
		}

		intents := cfg.Strategy.OnMarket(evt)
		for _, intent := range intents {
			// Ensure downstream sees a populated StrategyID, mirroring the
			// strategy-runtime live path.
			intent.StrategyID = cfg.Strategy.ID()
			res.Intents++

			intentResult, err := engine.HandleIntent(ctx, intent)
			if err != nil {
				// A single order error should not abort the entire backtest;
				// it is recorded via the sink's decision log and we continue.
				continue
			}
			if intentResult.Rejected {
				res.Rejects++
				continue
			}

			ticket, ok := matcher.TakeFill(intentResult.Submit.ClientOrderID)
			if !ok {
				continue
			}

			fill := ticket.ToTradeFillEvent(cfg.AccountID)
			if err := engine.ApplyFill(ctx, intent, intentResult.Submit, fill); err != nil {
				continue
			}
			res.Fills++

			// Update cash + inventory for equity tracking.
			switch ticket.Side {
			case "buy":
				cash -= ticket.FillPrice*ticket.FillQty + ticket.Fee
				positionQty[normSym(ticket.Symbol)] += ticket.FillQty
			case "sell":
				cash += ticket.FillPrice*ticket.FillQty - ticket.Fee
				positionQty[normSym(ticket.Symbol)] -= ticket.FillQty
			}
		}

		res.Equity = append(res.Equity, snapEquity(evt.EmitTSMS, cash, positionQty, lastPx))
	}

	return finalize(res, sink, cash, positionQty, lastPx, startedAt), nil
}

func snapEquity(tsMS int64, cash float64, qty, last map[string]float64) EquityPoint {
	mtm := cash
	for sym, q := range qty {
		mtm += q * last[sym]
	}
	return EquityPoint{TSMS: tsMS, Cash: cash, MarkToMarket: mtm}
}

func finalize(r Result, sink *core.MemorySink, cash float64, qty, last map[string]float64, startedAt time.Time) Result {
	// Ensure final MTM is captured even if the loop exited early via ctx.
	if len(r.Equity) > 0 {
		tail := r.Equity[len(r.Equity)-1]
		tail.Cash = cash
		tail.MarkToMarket = snapEquity(tail.TSMS, cash, qty, last).MarkToMarket
		r.Equity[len(r.Equity)-1] = tail
	}
	r.Trades = sink.Fills()
	r.Decisions = sink.Decisions()
	r.Metrics = ComputeMetrics(r.Equity, r.Trades)
	r.FinishedAt = time.Now()
	r.Duration = r.FinishedAt.Sub(startedAt)
	return r
}
