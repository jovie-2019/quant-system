package backtest

import (
	"context"
	"errors"
	"strings"
	"time"

	"quant-system/pkg/contracts"
)

var (
	ErrStrategyNil = errors.New("backtest: strategy is nil")
)

// Strategy is the minimal strategy contract required by backtest engine.
type Strategy interface {
	ID() string
	OnMarket(evt contracts.MarketNormalizedEvent) []contracts.OrderIntent
}

// Engine executes deterministic strategy replay over a fixed market dataset.
type Engine struct {
	strategy Strategy
}

// Dataset is the input event sequence for one backtest run.
type Dataset struct {
	Name   string
	Events []contracts.MarketNormalizedEvent
}

// Signal captures one strategy output at a specific event index.
type Signal struct {
	EventIndex int
	Symbol     string
	Side       string
	Price      float64
	Quantity   float64
}

// Result is the compact, machine-checkable report from one backtest run.
type Result struct {
	StrategyID string
	Dataset    string
	Events     int
	Intents    int
	BySide     map[string]int
	BySymbol   map[string]int
	Signals    []Signal

	StartedAt  time.Time
	FinishedAt time.Time
	Duration   time.Duration
}

func NewEngine(strategy Strategy) (*Engine, error) {
	if strategy == nil {
		return nil, ErrStrategyNil
	}
	return &Engine{strategy: strategy}, nil
}

func (e *Engine) Run(ctx context.Context, dataset Dataset) (Result, error) {
	startedAt := time.Now()

	res := Result{
		StrategyID: e.strategy.ID(),
		Dataset:    strings.TrimSpace(dataset.Name),
		Events:     len(dataset.Events),
		BySide:     make(map[string]int),
		BySymbol:   make(map[string]int),
		Signals:    make([]Signal, 0),
		StartedAt:  startedAt,
	}

	for i, evt := range dataset.Events {
		select {
		case <-ctx.Done():
			res.FinishedAt = time.Now()
			res.Duration = res.FinishedAt.Sub(startedAt)
			return res, ctx.Err()
		default:
		}

		intents := e.strategy.OnMarket(evt)
		for _, intent := range intents {
			side := strings.ToLower(strings.TrimSpace(intent.Side))
			if side == "" {
				side = "unknown"
			}

			res.Intents++
			res.BySide[side]++
			res.BySymbol[intent.Symbol]++
			res.Signals = append(res.Signals, Signal{
				EventIndex: i,
				Symbol:     intent.Symbol,
				Side:       side,
				Price:      intent.Price,
				Quantity:   intent.Quantity,
			})
		}
	}

	res.FinishedAt = time.Now()
	res.Duration = res.FinishedAt.Sub(startedAt)
	return res, nil
}
