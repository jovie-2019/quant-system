package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/nats-io/nats.go"

	"quant-system/internal/adminstore"
	"quant-system/internal/bus/natsbus"
	"quant-system/internal/core"
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
type Persister = core.Persister

// Config controls the pipeline behaviour.
type Config struct {
	AccountID     string // Account ID for position tracking, default "default"
	Subject       string // NATS subject to subscribe, default "strategy.intent.>"
	Durable       string // Durable consumer name, default "engine-core"
	Queue         string // Queue group, default "engine-core"
	DeliverPolicy string // "all", "new", "last"
	SimulateFill  bool   // If true, treat every ack as an immediate full fill (paper trading)
	Logger        *slog.Logger
	GatewayPool   *GatewayPool      // dynamic gateway resolution (optional)
	AdminStore    *adminstore.Store // for strategy config lookup (optional)
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

// Pipeline wires NATS intent consumption through the shared core.Engine.
// It is intentionally a thin adapter: all orchestration lives in core.Engine so
// the same flow can drive a backtest.
type Pipeline struct {
	bus        *natsbus.Client
	engine     *core.Engine
	defaultExec execution.Executor
	cfg        Config
}

// New creates a Pipeline. persister may be nil. defaultExec may be nil when
// cfg.GatewayPool is provided (dynamic resolution per intent).
func New(
	bus *natsbus.Client,
	riskEngine risk.RiskEngine,
	defaultExec execution.Executor,
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
	if defaultExec == nil && cfg.GatewayPool == nil {
		return nil, ErrExecNil
	}
	if fsm == nil {
		return nil, ErrFSMNil
	}
	if ledger == nil {
		return nil, ErrLedgerNil
	}
	cfg.defaults()

	p := &Pipeline{
		bus:         bus,
		defaultExec: defaultExec,
		cfg:         cfg,
	}

	coreCfg := core.Config{
		AccountID:    cfg.AccountID,
		SimulateFill: cfg.SimulateFill,
		Logger:       cfg.Logger,
		Clock:        core.RealClock{},
		Sink:         newNATSSink(bus),
		Persister:    persister,
		ExecResolver: p.makeExecResolver(),
	}
	engine, err := core.NewEngine(riskEngine, defaultExec, fsm, ledger, coreCfg)
	if err != nil {
		return nil, fmt.Errorf("pipeline: build core engine: %w", err)
	}
	p.engine = engine
	return p, nil
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
	_, err := p.engine.HandleIntent(ctx, intent)
	return err
}

// ApplyFill processes an external fill event (from exchange execution reports).
// It synthesises the TradeFillEvent from intent fields for backwards
// compatibility; callers needing full control should use core.Engine directly.
func (p *Pipeline) ApplyFill(ctx context.Context, intent contracts.OrderIntent, submit execution.SubmitResult) error {
	fill := contracts.TradeFillEvent{
		TradeID:    "fill-" + submit.ClientOrderID,
		AccountID:  p.cfg.AccountID,
		Symbol:     intent.Symbol,
		Side:       intent.Side,
		FillQty:    intent.Quantity,
		FillPrice:  intent.Price,
		SourceTSMS: time.Now().UnixMilli(),
	}
	return p.engine.ApplyFill(ctx, intent, submit, fill)
}

// makeExecResolver returns a core.ExecutorResolver that routes an intent to the
// per-API-key executor from the GatewayPool when StrategyID has a configured
// api_key_id. Returns (nil, nil) to signal fallback to the default executor.
func (p *Pipeline) makeExecResolver() core.ExecutorResolver {
	if p.cfg.GatewayPool == nil || p.cfg.AdminStore == nil {
		return nil
	}
	return func(ctx context.Context, intent contracts.OrderIntent) (execution.Executor, error) {
		if strings.TrimSpace(intent.StrategyID) == "" {
			return nil, nil
		}
		cfg, found, err := p.cfg.AdminStore.GetStrategyConfigByStrategyID(ctx, intent.StrategyID)
		if err != nil {
			p.cfg.Logger.Warn("pipeline: strategy config lookup failed, using default executor",
				"strategy_id", intent.StrategyID, "error", err)
			return nil, nil
		}
		if !found || cfg.APIKeyID <= 0 {
			return nil, nil
		}
		exec, err := p.cfg.GatewayPool.GetExecutor(ctx, cfg.APIKeyID)
		if err != nil {
			return nil, fmt.Errorf("pipeline: get executor for api_key_id %d: %w", cfg.APIKeyID, err)
		}
		return exec, nil
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
