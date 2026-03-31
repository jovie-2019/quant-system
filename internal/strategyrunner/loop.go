package strategyrunner

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/nats-io/nats.go"

	"quant-system/internal/bus/natsbus"
	"quant-system/internal/strategy"
	"quant-system/pkg/contracts"
)

var (
	ErrBusNil        = errors.New("strategyrunner: bus client is nil")
	ErrRuntimeNil    = errors.New("strategyrunner: strategy runtime is nil")
	ErrDurableEmpty  = errors.New("strategyrunner: durable is empty")
	ErrSubjectEmpty  = errors.New("strategyrunner: subject is empty")
	ErrIntentBusNil  = errors.New("strategyrunner: intent sink bus client is nil")
	ErrStrategyEmpty = errors.New("strategyrunner: strategy id is empty")
)

type Runtime interface {
	HandleMarket(ctx context.Context, evt contracts.MarketNormalizedEvent) error
}

type Config struct {
	Subject       string
	Durable       string
	Queue         string
	AckWait       time.Duration
	MaxDeliver    int
	DeliverPolicy string
}

type Loop struct {
	bus     *natsbus.Client
	runtime Runtime
	cfg     Config
}

func NewLoop(bus *natsbus.Client, runtime Runtime, cfg Config) (*Loop, error) {
	if bus == nil {
		return nil, ErrBusNil
	}
	if runtime == nil {
		return nil, ErrRuntimeNil
	}
	if strings.TrimSpace(cfg.Subject) == "" {
		cfg.Subject = "market.normalized.spot.>"
	}
	if strings.TrimSpace(cfg.Durable) == "" {
		return nil, ErrDurableEmpty
	}
	if cfg.AckWait <= 0 {
		cfg.AckWait = 5 * time.Second
	}
	if cfg.MaxDeliver <= 0 {
		cfg.MaxDeliver = 5
	}
	return &Loop{
		bus:     bus,
		runtime: runtime,
		cfg:     cfg,
	}, nil
}

func (l *Loop) Start(ctx context.Context) (*nats.Subscription, error) {
	return l.bus.Subscribe(
		ctx,
		l.cfg.Subject,
		natsbus.SubscribeConfig{
			Durable:       l.cfg.Durable,
			Queue:         l.cfg.Queue,
			AckWait:       l.cfg.AckWait,
			MaxDeliver:    l.cfg.MaxDeliver,
			DeliverPolicy: l.cfg.DeliverPolicy,
		},
		func(ctx context.Context, msg natsbus.Message) error {
			return l.HandleMessage(ctx, msg)
		},
	)
}

func (l *Loop) HandleMessage(ctx context.Context, msg natsbus.Message) error {
	var evt contracts.MarketNormalizedEvent
	if err := json.Unmarshal(msg.Data, &evt); err != nil {
		return err
	}
	return l.runtime.HandleMarket(ctx, evt)
}

func NewNATSIntentSink(client *natsbus.Client) (strategy.IntentSink, error) {
	if client == nil {
		return nil, ErrIntentBusNil
	}
	return func(ctx context.Context, intent strategy.OrderIntent) error {
		if strings.TrimSpace(intent.StrategyID) == "" {
			return ErrStrategyEmpty
		}
		return natsbus.PublishStrategyIntent(ctx, client, contracts.OrderIntent(intent), nil)
	}, nil
}
