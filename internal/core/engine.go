package core

import (
	"context"
	"errors"
	"log/slog"

	"quant-system/internal/execution"
	"quant-system/internal/orderfsm"
	"quant-system/internal/position"
	"quant-system/internal/risk"
	"quant-system/pkg/contracts"
)

var (
	// ErrRiskNil is returned when NewEngine is called without a risk engine.
	ErrRiskNil = errors.New("core: risk engine is nil")
	// ErrExecNil is returned when neither a default executor nor an
	// ExecutorResolver has been provided.
	ErrExecNil = errors.New("core: executor is nil")
	// ErrFSMNil is returned when NewEngine is called without a state machine.
	ErrFSMNil = errors.New("core: fsm is nil")
	// ErrLedgerNil is returned when NewEngine is called without a ledger.
	ErrLedgerNil = errors.New("core: ledger is nil")
	// ErrNoExecutor is returned at Submit time when the resolver yields no executor.
	ErrNoExecutor = errors.New("core: no executor available")
)

// Persister is the optional persistence hook used by both live trading and
// backtest (e.g. to record runs to MySQL / ClickHouse).
type Persister interface {
	SaveRiskDecision(ctx context.Context, decision contracts.RiskDecision) error
	UpsertOrder(ctx context.Context, order contracts.Order) error
	UpsertPosition(ctx context.Context, snapshot contracts.PositionSnapshot) error
}

// ExecutorResolver selects the executor that should handle a given intent.
// Return (nil, nil) to fall back to the Engine's default executor.
type ExecutorResolver func(ctx context.Context, intent contracts.OrderIntent) (execution.Executor, error)

// Config controls Engine behaviour.
type Config struct {
	// AccountID is used for event subject routing and ledger keys. Defaults to "default".
	AccountID string
	// SimulateFill, when true, treats every ack as an immediate full fill at
	// the intent's price. Useful for paper trading and for strategies using
	// IOC semantics in a simplified harness.
	SimulateFill bool
	// Logger is the slog logger used by the Engine. Defaults to slog.Default().
	Logger *slog.Logger
	// Clock is the clock used to stamp lifecycle events. Defaults to RealClock.
	Clock Clock
	// Sink receives risk / order / fill events. Defaults to NopSink.
	Sink EventSink
	// Persister is an optional durable store for decisions, orders, positions.
	Persister Persister
	// ExecResolver allows callers to route intents to different executors (e.g.
	// per-API-key gateway pools). If it returns (nil, nil) the Engine falls
	// back to the default executor.
	ExecResolver ExecutorResolver
}

// IntentResult summarises the outcome of HandleIntent so callers (live pipeline
// or backtest engine) can observe what happened without inspecting the sink.
type IntentResult struct {
	Decision  contracts.RiskDecision
	Submit    execution.SubmitResult
	Rejected  bool
	Filled    bool
	FillEvent contracts.TradeFillEvent
}

// Engine orchestrates risk → execution → FSM → ledger. It is transport-agnostic
// and used by both the live NATS-driven pipeline and the event-driven backtest.
type Engine struct {
	risk   risk.RiskEngine
	exec   execution.Executor
	fsm    orderfsm.OrderStateMachine
	ledger position.PositionLedger
	cfg    Config
}

// NewEngine builds an Engine. exec may be nil when cfg.ExecResolver is
// provided and always returns a non-nil executor.
func NewEngine(
	riskEngine risk.RiskEngine,
	exec execution.Executor,
	fsm orderfsm.OrderStateMachine,
	ledger position.PositionLedger,
	cfg Config,
) (*Engine, error) {
	if riskEngine == nil {
		return nil, ErrRiskNil
	}
	if exec == nil && cfg.ExecResolver == nil {
		return nil, ErrExecNil
	}
	if fsm == nil {
		return nil, ErrFSMNil
	}
	if ledger == nil {
		return nil, ErrLedgerNil
	}
	if cfg.AccountID == "" {
		cfg.AccountID = "default"
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Clock == nil {
		cfg.Clock = RealClock{}
	}
	if cfg.Sink == nil {
		cfg.Sink = NopSink{}
	}
	return &Engine{risk: riskEngine, exec: exec, fsm: fsm, ledger: ledger, cfg: cfg}, nil
}

// HandleIntent runs a single OrderIntent through risk evaluation, executor
// submission, FSM ack, and (if configured) simulated fill. Returned errors are
// unwrapped; persistence / sink failures are logged but do not abort the flow.
func (e *Engine) HandleIntent(ctx context.Context, intent contracts.OrderIntent) (IntentResult, error) {
	decision := e.risk.Evaluate(ctx, intent)

	if e.cfg.Persister != nil {
		if err := e.cfg.Persister.SaveRiskDecision(ctx, decision); err != nil {
			e.cfg.Logger.Error("core: persist risk decision", "error", err, "intent_id", intent.IntentID)
		}
	}
	if err := e.cfg.Sink.PublishRiskDecision(ctx, e.cfg.AccountID, decision); err != nil {
		e.cfg.Logger.Error("core: publish risk decision", "error", err, "intent_id", intent.IntentID)
	}

	if decision.Decision != risk.DecisionAllow {
		e.cfg.Logger.Info("core: intent rejected",
			"intent_id", intent.IntentID,
			"rule_id", decision.RuleID,
			"reason", decision.ReasonCode,
		)
		return IntentResult{Decision: decision, Rejected: true}, nil
	}

	exec, err := e.resolveExecutor(ctx, intent)
	if err != nil {
		return IntentResult{Decision: decision}, err
	}
	submit, err := exec.Submit(ctx, decision)
	if err != nil {
		return IntentResult{Decision: decision}, err
	}

	if _, err := e.applyAck(ctx, intent, submit); err != nil {
		return IntentResult{Decision: decision, Submit: submit}, err
	}

	if e.cfg.SimulateFill {
		fill, err := e.applySimulatedFill(ctx, intent, submit)
		if err != nil {
			return IntentResult{Decision: decision, Submit: submit}, err
		}
		return IntentResult{Decision: decision, Submit: submit, Filled: true, FillEvent: fill}, nil
	}

	return IntentResult{Decision: decision, Submit: submit}, nil
}

// ApplyFill applies an externally-sourced fill (e.g. from exchange execution
// reports arriving via a separate NATS subscription, or synthesised by the
// backtest SimMatcher).
func (e *Engine) ApplyFill(ctx context.Context, intent contracts.OrderIntent, submit execution.SubmitResult, fill contracts.TradeFillEvent) error {
	_, err := e.applyFillDirect(ctx, intent, submit, fill)
	return err
}

func (e *Engine) resolveExecutor(ctx context.Context, intent contracts.OrderIntent) (execution.Executor, error) {
	if e.cfg.ExecResolver != nil {
		exec, err := e.cfg.ExecResolver(ctx, intent)
		if err != nil {
			return nil, err
		}
		if exec != nil {
			return exec, nil
		}
	}
	if e.exec != nil {
		return e.exec, nil
	}
	return nil, ErrNoExecutor
}

func (e *Engine) applyAck(ctx context.Context, intent contracts.OrderIntent, submit execution.SubmitResult) (contracts.Order, error) {
	if existing, ok := e.fsm.Get(submit.ClientOrderID); ok {
		switch existing.State {
		case orderfsm.StateAck, orderfsm.StatePartial, orderfsm.StateFilled:
			return existing, nil
		}
	}
	order, err := e.fsm.Apply(orderfsm.Event{
		ClientOrderID: submit.ClientOrderID,
		VenueOrderID:  submit.VenueOrderID,
		Symbol:        intent.Symbol,
		State:         orderfsm.StateAck,
	})
	if err != nil {
		return contracts.Order{}, err
	}
	e.persistOrder(ctx, order)
	e.publishLifecycle(ctx, intent, submit, string(orderfsm.StateAck), 0, 0)
	return order, nil
}

func (e *Engine) applySimulatedFill(ctx context.Context, intent contracts.OrderIntent, submit execution.SubmitResult) (contracts.TradeFillEvent, error) {
	fill := contracts.TradeFillEvent{
		TradeID:    "fill-" + submit.ClientOrderID,
		AccountID:  e.cfg.AccountID,
		Symbol:     intent.Symbol,
		Side:       intent.Side,
		FillQty:    intent.Quantity,
		FillPrice:  intent.Price,
		SourceTSMS: e.cfg.Clock.UnixMilli(),
	}
	return e.applyFillDirect(ctx, intent, submit, fill)
}

func (e *Engine) applyFillDirect(ctx context.Context, intent contracts.OrderIntent, submit execution.SubmitResult, fill contracts.TradeFillEvent) (contracts.TradeFillEvent, error) {
	if existing, ok := e.fsm.Get(submit.ClientOrderID); !ok || existing.State != orderfsm.StateFilled {
		order, err := e.fsm.Apply(orderfsm.Event{
			ClientOrderID: submit.ClientOrderID,
			VenueOrderID:  submit.VenueOrderID,
			Symbol:        intent.Symbol,
			State:         orderfsm.StateFilled,
			FilledQty:     fill.FillQty,
			AvgPrice:      fill.FillPrice,
		})
		if err != nil {
			return contracts.TradeFillEvent{}, err
		}
		e.persistOrder(ctx, order)
		e.publishLifecycle(ctx, intent, submit, string(orderfsm.StateFilled), fill.FillQty, fill.FillPrice)
	}

	snapshot, err := e.ledger.ApplyFill(ctx, fill)
	if err != nil {
		return contracts.TradeFillEvent{}, err
	}
	if e.cfg.Persister != nil {
		if err := e.cfg.Persister.UpsertPosition(ctx, snapshot); err != nil {
			e.cfg.Logger.Error("core: persist position", "error", err,
				"account_id", fill.AccountID, "symbol", fill.Symbol)
		}
	}

	if err := e.cfg.Sink.PublishTradeFill(ctx, e.cfg.AccountID, fill); err != nil {
		e.cfg.Logger.Error("core: publish trade fill", "error", err,
			"client_order_id", submit.ClientOrderID)
	}

	e.cfg.Logger.Info("core: fill applied",
		"client_order_id", submit.ClientOrderID,
		"symbol", intent.Symbol,
		"side", intent.Side,
		"qty", fill.FillQty,
		"price", fill.FillPrice,
	)
	return fill, nil
}

func (e *Engine) persistOrder(ctx context.Context, order contracts.Order) {
	if e.cfg.Persister == nil {
		return
	}
	if err := e.cfg.Persister.UpsertOrder(ctx, order); err != nil {
		e.cfg.Logger.Error("core: persist order", "error", err, "client_order_id", order.ClientOrderID)
	}
}

func (e *Engine) publishLifecycle(ctx context.Context, intent contracts.OrderIntent, submit execution.SubmitResult, state string, filledQty, avgPrice float64) {
	evt := contracts.OrderLifecycleEvent{
		Symbol:        intent.Symbol,
		ClientOrderID: submit.ClientOrderID,
		VenueOrderID:  submit.VenueOrderID,
		State:         state,
		FilledQty:     filledQty,
		AvgPrice:      avgPrice,
		EmitTSMS:      e.cfg.Clock.UnixMilli(),
	}
	if err := e.cfg.Sink.PublishOrderLifecycle(ctx, e.cfg.AccountID, evt); err != nil {
		e.cfg.Logger.Error("core: publish order lifecycle", "error", err,
			"client_order_id", submit.ClientOrderID, "state", state)
	}
}
