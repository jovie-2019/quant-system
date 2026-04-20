package strategyrunner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"

	"quant-system/internal/bus/natsbus"
	"quant-system/internal/strategy"
	"quant-system/pkg/contracts"
)

// ErrControlBusNil is returned when NewControlHandler is called without a bus.
var ErrControlBusNil = errors.New("strategyrunner: control bus is nil")

// ErrControlStrategyEmpty is returned when StrategyID is missing.
var ErrControlStrategyEmpty = errors.New("strategyrunner: control strategy id is empty")

// ControlConfig parameters a ControlHandler.
type ControlConfig struct {
	// StrategyID scopes this handler to commands for a single strategy
	// instance. The runner subscribes to SubjectStrategyControl(id) with
	// a durable consumer named "runner-ctl-<id>" so at most one
	// replica receives each command.
	StrategyID string
	// Durable overrides the consumer name. Optional.
	Durable string
	// Logger is used for audit-grade logging. Nil defaults to slog.Default().
	Logger *slog.Logger
}

// StateSnapshot is the externally-observable runtime state the handler
// toggles. Fields are exposed as atomics so other parts of the process
// (metrics emitter, health check) can read without contending on the
// handler's internal mutex.
type StateSnapshot struct {
	Paused     bool  `json:"paused"`
	Shadow     bool  `json:"shadow"`
	Revision   int64 `json:"revision"`
	LastEventMS int64 `json:"last_event_ms"`
}

// ControlHandler subscribes to a strategy's control subject and applies
// commands to the owning Strategy. It replies on the ack subject so the
// admin-api can update its audit log. The handler owns the runtime flags
// (paused, shadow) that gate the IntentSink used by the strategy runtime.
type ControlHandler struct {
	bus      *natsbus.Client
	strategy strategy.Strategy
	cfg      ControlConfig
	logger   *slog.Logger

	// lastRevision tracks the highest Revision we've successfully applied;
	// replayed / duplicate messages are rejected without touching state.
	lastRevision atomic.Int64

	paused atomic.Bool
	shadow atomic.Bool

	mu sync.Mutex // serialises ApplyParams invocations
}

// NewControlHandler constructs a handler for a specific strategy instance.
func NewControlHandler(bus *natsbus.Client, s strategy.Strategy, cfg ControlConfig) (*ControlHandler, error) {
	if bus == nil {
		return nil, ErrControlBusNil
	}
	if s == nil {
		return nil, errors.New("strategyrunner: control strategy is nil")
	}
	if strings.TrimSpace(cfg.StrategyID) == "" {
		return nil, ErrControlStrategyEmpty
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Durable == "" {
		cfg.Durable = "runner-ctl-" + cfg.StrategyID
	}
	return &ControlHandler{
		bus:      bus,
		strategy: s,
		cfg:      cfg,
		logger:   cfg.Logger,
	}, nil
}

// Start subscribes to the control subject. Returns the subscription so
// the caller can Unsubscribe on shutdown.
func (h *ControlHandler) Start(ctx context.Context) (*nats.Subscription, error) {
	subject := natsbus.SubjectStrategyControl(h.cfg.StrategyID)
	return h.bus.Subscribe(ctx, subject, natsbus.SubscribeConfig{
		Durable:    h.cfg.Durable,
		AckWait:    5 * time.Second,
		MaxDeliver: 3,
	}, h.handleMessage)
}

// Snapshot returns a lock-free view of the current lifecycle state.
func (h *ControlHandler) Snapshot() StateSnapshot {
	return StateSnapshot{
		Paused:   h.paused.Load(),
		Shadow:   h.shadow.Load(),
		Revision: h.lastRevision.Load(),
	}
}

// IsPaused is a cheap accessor for the IntentSink wrapper.
func (h *ControlHandler) IsPaused() bool { return h.paused.Load() }

// IsShadow is a cheap accessor for the IntentSink wrapper.
func (h *ControlHandler) IsShadow() bool { return h.shadow.Load() }

// handleMessage is the NATS subscribe callback. It parses the envelope,
// dispatches by Type, and publishes an ack with the outcome. Errors here
// are converted into rejecting acks rather than returned, so NATS's
// redelivery machinery doesn't loop on malformed payloads.
func (h *ControlHandler) handleMessage(ctx context.Context, msg natsbus.Message) error {
	var env contracts.StrategyControlEnvelope
	if err := json.Unmarshal(msg.Data, &env); err != nil {
		h.publishAck(ctx, env.StrategyID, env.Revision, false, fmt.Sprintf("unmarshal: %v", err))
		return nil // don't retry; payload is unrecoverable
	}
	if env.StrategyID != h.cfg.StrategyID {
		// Shouldn't happen given the subscription subject, but guard
		// against misrouting.
		h.publishAck(ctx, env.StrategyID, env.Revision, false, "strategy_id mismatch")
		return nil
	}
	if env.Revision > 0 && env.Revision <= h.lastRevision.Load() {
		h.publishAck(ctx, env.StrategyID, env.Revision, false, "stale revision")
		return nil
	}

	accepted, errMsg := h.dispatch(env)
	if accepted && env.Revision > 0 {
		h.lastRevision.Store(env.Revision)
	}
	h.publishAck(ctx, env.StrategyID, env.Revision, accepted, errMsg)
	h.logger.Info("strategyrunner: control handled",
		"type", env.Type, "rev", env.Revision, "accepted", accepted, "err", errMsg)
	return nil
}

func (h *ControlHandler) dispatch(env contracts.StrategyControlEnvelope) (bool, string) {
	switch env.Type {
	case contracts.StrategyControlUpdateParams:
		reloader, ok := h.strategy.(strategy.ParamReloader)
		if !ok {
			return false, "strategy does not support hot-reload"
		}
		if len(env.Params) == 0 {
			return false, "empty params"
		}
		h.mu.Lock()
		err := reloader.ApplyParams(env.Params)
		h.mu.Unlock()
		if err != nil {
			return false, err.Error()
		}
		return true, ""
	case contracts.StrategyControlPause:
		h.paused.Store(true)
		return true, ""
	case contracts.StrategyControlResume:
		h.paused.Store(false)
		return true, ""
	case contracts.StrategyControlShadowOn:
		h.shadow.Store(true)
		return true, ""
	case contracts.StrategyControlShadowOff:
		h.shadow.Store(false)
		return true, ""
	default:
		return false, fmt.Sprintf("unknown control type %q", env.Type)
	}
}

func (h *ControlHandler) publishAck(ctx context.Context, strategyID string, revision int64, accepted bool, errMsg string) {
	ack := contracts.StrategyControlAck{
		StrategyID: strategyID,
		Revision:   revision,
		Accepted:   accepted,
		Error:      errMsg,
		AppliedMS:  time.Now().UnixMilli(),
	}
	if err := natsbus.PublishStrategyControlAck(ctx, h.bus, ack, nil); err != nil {
		h.logger.Warn("strategyrunner: publish ack failed", "error", err, "rev", revision)
	}
}

// GatedIntentSink wraps a downstream IntentSink so that:
//   - When the handler is paused, intents are dropped (logged only).
//   - When in shadow mode, intents are routed to SubjectStrategyShadowIntent
//     instead of the live path.
//
// It is safe to compose with the existing NATSIntentSink.
func GatedIntentSink(h *ControlHandler, live strategy.IntentSink, bus *natsbus.Client) strategy.IntentSink {
	return func(ctx context.Context, intent strategy.OrderIntent) error {
		if h.IsPaused() {
			h.logger.Info("strategyrunner: intent dropped (paused)",
				"strategy_id", intent.StrategyID, "intent_id", intent.IntentID)
			return nil
		}
		if h.IsShadow() {
			if bus == nil {
				// No shadow sink wired; just drop silently so the
				// strategy doesn't back-pressure on a misconfig.
				return nil
			}
			return natsbus.PublishShadowIntent(ctx, bus, contracts.OrderIntent(intent), nil)
		}
		return live(ctx, intent)
	}
}
