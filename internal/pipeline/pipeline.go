package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/nats-io/nats.go"

	"quant-system/internal/bus/natsbus"
	"quant-system/internal/execution"
	"quant-system/internal/orderfsm"
	"quant-system/internal/position"
	"quant-system/internal/risk"
	"quant-system/pkg/contracts"
)

var (
	ErrBusNil    = errors.New("pipeline: bus client is nil")
	ErrRiskNil   = errors.New("pipeline: risk engine is nil")
	ErrExecNil   = errors.New("pipeline: executor is nil")
	ErrFSMNil    = errors.New("pipeline: fsm is nil")
	ErrLedgerNil = errors.New("pipeline: ledger is nil")
)

// Persister is an optional persistence layer (e.g. mysqlstore.Repository).
type Persister interface {
	SaveRiskDecision(ctx context.Context, decision contracts.RiskDecision) error
	UpsertOrder(ctx context.Context, order contracts.Order) error
	UpsertPosition(ctx context.Context, snapshot contracts.PositionSnapshot) error
}

// Config controls the pipeline behaviour.
type Config struct {
	AccountID     string // Account ID for position tracking, default "default"
	Subject       string // NATS subject to subscribe, default "strategy.intent.>"
	Durable       string // Durable consumer name, default "engine-core"
	Queue         string // Queue group, default "engine-core"
	DeliverPolicy string // "all", "new", "last"
	SimulateFill  bool   // If true, treat every ack as an immediate full fill (paper trading)
	Logger        *slog.Logger
}

func (c *Config) defaults() {
	if c.AccountID == "" {
		c.AccountID = "default"
	}
	if c.Subject == "" {
		c.Subject = "strategy.intent.>"
	}
	if c.Durable == "" {
		c.Durable = "engine-core"
	}
	if c.Queue == "" {
		c.Queue = "engine-core"
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
}

// Pipeline wires NATS intent consumption through the full trading flow.
type Pipeline struct {
	bus       *natsbus.Client
	risk      risk.RiskEngine
	exec      execution.Executor
	fsm       orderfsm.OrderStateMachine
	ledger    position.PositionLedger
	persister Persister
	cfg       Config
}

// New creates a Pipeline. Persister may be nil (no MySQL).
func New(
	bus *natsbus.Client,
	riskEngine risk.RiskEngine,
	exec execution.Executor,
	fsm orderfsm.OrderStateMachine,
	ledger position.PositionLedger,
	persister Persister,
	cfg Config,
) (*Pipeline, error) {
	if bus == nil {
		return nil, ErrBusNil
	}
	if riskEngine == nil {
		return nil, ErrRiskNil
	}
	if exec == nil {
		return nil, ErrExecNil
	}
	if fsm == nil {
		return nil, ErrFSMNil
	}
	if ledger == nil {
		return nil, ErrLedgerNil
	}
	cfg.defaults()
	return &Pipeline{
		bus:       bus,
		risk:      riskEngine,
		exec:      exec,
		fsm:       fsm,
		ledger:    ledger,
		persister: persister,
		cfg:       cfg,
	}, nil
}

// Start subscribes to strategy intents and processes them through the pipeline.
// Returns the subscription handle (for Unsubscribe on shutdown).
func (p *Pipeline) Start(ctx context.Context) (*nats.Subscription, error) {
	return p.bus.Subscribe(ctx, p.cfg.Subject, natsbus.SubscribeConfig{
		Durable:       p.cfg.Durable,
		Queue:         p.cfg.Queue,
		AckWait:       10 * time.Second,
		MaxDeliver:    5,
		DeliverPolicy: p.cfg.DeliverPolicy,
	}, func(ctx context.Context, msg natsbus.Message) error {
		return p.handleIntent(ctx, msg)
	})
}

func (p *Pipeline) handleIntent(ctx context.Context, msg natsbus.Message) error {
	var intent contracts.OrderIntent
	if err := json.Unmarshal(msg.Data, &intent); err != nil {
		p.cfg.Logger.Error("pipeline: unmarshal intent", "error", err, "subject", msg.Subject)
		return err
	}

	// 1. Risk evaluation
	decision := p.risk.Evaluate(ctx, intent)

	if p.persister != nil {
		if err := p.persister.SaveRiskDecision(ctx, decision); err != nil {
			p.cfg.Logger.Error("pipeline: persist risk decision", "error", err, "intent_id", intent.IntentID)
		}
	}

	// Publish risk decision to NATS (best-effort).
	if err := natsbus.PublishRiskDecision(ctx, p.bus, p.cfg.AccountID, decision, nil); err != nil {
		p.cfg.Logger.Error("pipeline: publish risk decision", "error", err, "intent_id", intent.IntentID)
	}

	if decision.Decision != risk.DecisionAllow {
		p.cfg.Logger.Info("pipeline: intent rejected",
			"intent_id", intent.IntentID,
			"rule_id", decision.RuleID,
			"reason", decision.ReasonCode,
		)
		return nil
	}

	// 2. Execution
	submit, err := p.exec.Submit(ctx, decision)
	if err != nil {
		p.cfg.Logger.Error("pipeline: execution submit", "error", err, "intent_id", intent.IntentID)
		return err
	}

	// 3. FSM — apply ack state
	_, err = p.applyAck(ctx, intent, submit)
	if err != nil {
		p.cfg.Logger.Error("pipeline: fsm ack", "error", err, "client_order_id", submit.ClientOrderID)
		return err
	}

	// 4. Simulate fill if configured (paper trading / IOC orders)
	if p.cfg.SimulateFill {
		if err := p.applyFill(ctx, intent, submit); err != nil {
			p.cfg.Logger.Error("pipeline: simulate fill", "error", err, "client_order_id", submit.ClientOrderID)
			return err
		}
	}

	return nil
}

// ApplyFill processes an external fill event (from exchange execution reports).
// This is the entry point for real fills coming from a separate subscription.
func (p *Pipeline) ApplyFill(ctx context.Context, intent contracts.OrderIntent, submit execution.SubmitResult) error {
	return p.applyFill(ctx, intent, submit)
}

func (p *Pipeline) applyFill(ctx context.Context, intent contracts.OrderIntent, submit execution.SubmitResult) error {
	if existing, ok := p.fsm.Get(submit.ClientOrderID); !ok || existing.State != orderfsm.StateFilled {
		// FSM → filled
		order, err := p.fsm.Apply(orderfsm.Event{
			ClientOrderID: submit.ClientOrderID,
			VenueOrderID:  submit.VenueOrderID,
			Symbol:        intent.Symbol,
			State:         orderfsm.StateFilled,
			FilledQty:     intent.Quantity,
			AvgPrice:      intent.Price,
		})
		if err != nil {
			return err
		}
		p.persistOrder(ctx, order)
		p.publishOrderLifecycle(ctx, intent, submit, string(orderfsm.StateFilled), intent.Quantity, intent.Price)
	}

	// Position
	tradeID := "fill-" + submit.ClientOrderID
	snapshot, err := p.ledger.ApplyFill(ctx, contracts.TradeFillEvent{
		TradeID:    tradeID,
		AccountID:  p.cfg.AccountID,
		Symbol:     intent.Symbol,
		Side:       intent.Side,
		FillQty:    intent.Quantity,
		FillPrice:  intent.Price,
		SourceTSMS: time.Now().UnixMilli(),
	})
	if err != nil {
		return err
	}

	if p.persister != nil {
		if err := p.persister.UpsertPosition(ctx, snapshot); err != nil {
			p.cfg.Logger.Error("pipeline: persist position", "error", err,
				"account_id", p.cfg.AccountID, "symbol", intent.Symbol)
		}
	}

	// Publish trade fill to NATS.
	_ = natsbus.PublishTradeFill(ctx, p.bus, p.cfg.AccountID, contracts.TradeFillEvent{
		TradeID:    tradeID,
		AccountID:  p.cfg.AccountID,
		Symbol:     intent.Symbol,
		Side:       intent.Side,
		FillQty:    intent.Quantity,
		FillPrice:  intent.Price,
		SourceTSMS: time.Now().UnixMilli(),
	}, nil)

	p.cfg.Logger.Info("pipeline: fill applied",
		"client_order_id", submit.ClientOrderID,
		"symbol", intent.Symbol,
		"side", intent.Side,
		"qty", intent.Quantity,
		"price", intent.Price,
	)
	return nil
}

func (p *Pipeline) applyAck(ctx context.Context, intent contracts.OrderIntent, submit execution.SubmitResult) (contracts.Order, error) {
	if existing, ok := p.fsm.Get(submit.ClientOrderID); ok {
		switch existing.State {
		case orderfsm.StateAck, orderfsm.StatePartial, orderfsm.StateFilled:
			return existing, nil
		}
	}

	order, err := p.fsm.Apply(orderfsm.Event{
		ClientOrderID: submit.ClientOrderID,
		VenueOrderID:  submit.VenueOrderID,
		Symbol:        intent.Symbol,
		State:         orderfsm.StateAck,
	})
	if err != nil {
		return contracts.Order{}, err
	}
	p.persistOrder(ctx, order)
	p.publishOrderLifecycle(ctx, intent, submit, string(orderfsm.StateAck), 0, 0)
	return order, nil
}

func (p *Pipeline) persistOrder(ctx context.Context, order contracts.Order) {
	if p.persister == nil {
		return
	}
	if err := p.persister.UpsertOrder(ctx, order); err != nil {
		p.cfg.Logger.Error("pipeline: persist order", "error", err, "client_order_id", order.ClientOrderID)
	}
}

func (p *Pipeline) publishOrderLifecycle(ctx context.Context, intent contracts.OrderIntent, submit execution.SubmitResult, state string, filledQty, avgPrice float64) {
	evt := contracts.OrderLifecycleEvent{
		Symbol:        intent.Symbol,
		ClientOrderID: submit.ClientOrderID,
		VenueOrderID:  submit.VenueOrderID,
		State:         state,
		FilledQty:     filledQty,
		AvgPrice:      avgPrice,
		EmitTSMS:      time.Now().UnixMilli(),
	}
	if err := natsbus.PublishOrderLifecycle(ctx, p.bus, p.cfg.AccountID, evt, nil); err != nil {
		p.cfg.Logger.Error("pipeline: publish order lifecycle", "error", err,
			"client_order_id", submit.ClientOrderID, "state", state)
	}
}

// EnsureStreams creates the NATS JetStream streams needed by the pipeline.
func EnsureStreams(ctx context.Context, client *natsbus.Client) error {
	streams := []natsbus.StreamConfig{
		{Name: "STREAM_MARKET", Subjects: []string{"market.normalized.spot.>"}, MaxAge: 24 * time.Hour, MaxBytes: 500 * 1024 * 1024}, // 500MB, 24h
		{Name: "STREAM_TRADING", Subjects: []string{"strategy.intent.>"}},
		{Name: "STREAM_RISK", Subjects: []string{"risk.decision.>"}},
		{Name: "STREAM_ORDERS", Subjects: []string{"order.lifecycle.>"}},
		{Name: "STREAM_FILLS", Subjects: []string{"trade.fill.>"}},
	}
	for _, sc := range streams {
		if err := client.EnsureStream(ctx, sc); err != nil {
			// Stream may already exist with different subjects — log and continue.
			if !strings.Contains(err.Error(), "subjects overlap") {
				return err
			}
			slog.Warn("pipeline: stream overlap, skipping", "stream", sc.Name, "error", err)
		}
	}
	return nil
}
