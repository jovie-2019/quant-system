package natsbus

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
)

var (
	ErrInvalidConfig = errors.New("natsbus: invalid config")
)

type Config struct {
	URL           string
	Name          string
	ConnectWait   time.Duration
	ReconnectWait time.Duration
	MaxReconnects int
}

type StreamConfig struct {
	Name      string
	Subjects  []string
	MaxAge    time.Duration
	MaxBytes  int64
	Storage   nats.StorageType
	Replicas  int
	Discard   nats.DiscardPolicy
	Duplicate time.Duration
}

type SubscribeConfig struct {
	Durable       string
	Queue         string
	AckWait       time.Duration
	MaxDeliver    int
	DeliverPolicy string // "new", "all", "last" — defaults to "all"
	OnAck         func(streamSeq, consumerSeq uint64)
}

type Message struct {
	Subject string
	Header  map[string]string
	Data    []byte
}

type Handler func(context.Context, Message) error

type Client struct {
	nc *nats.Conn
	js nats.JetStreamContext
}

func Connect(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, ErrInvalidConfig
	}
	if cfg.ConnectWait <= 0 {
		cfg.ConnectWait = 5 * time.Second
	}
	if cfg.ReconnectWait <= 0 {
		cfg.ReconnectWait = 500 * time.Millisecond
	}
	if cfg.MaxReconnects == 0 {
		cfg.MaxReconnects = 20
	}

	opts := []nats.Option{
		nats.Name(strings.TrimSpace(cfg.Name)),
		nats.Timeout(cfg.ConnectWait),
		nats.ReconnectWait(cfg.ReconnectWait),
		nats.MaxReconnects(cfg.MaxReconnects),
	}
	nc, err := nats.Connect(strings.TrimSpace(cfg.URL), opts...)
	if err != nil {
		return nil, err
	}
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, err
	}
	return &Client{nc: nc, js: js}, nil
}

func (c *Client) Close() {
	if c == nil || c.nc == nil {
		return
	}
	c.nc.Close()
}

func (c *Client) EnsureStream(ctx context.Context, cfg StreamConfig) error {
	if c == nil || c.js == nil {
		return ErrInvalidConfig
	}
	if strings.TrimSpace(cfg.Name) == "" || len(cfg.Subjects) == 0 {
		return ErrInvalidConfig
	}
	if cfg.Storage == 0 {
		cfg.Storage = nats.FileStorage
	}
	if cfg.Replicas <= 0 {
		cfg.Replicas = 1
	}
	if cfg.MaxAge <= 0 {
		cfg.MaxAge = 7 * 24 * time.Hour
	}
	if cfg.Discard == 0 {
		cfg.Discard = nats.DiscardOld
	}
	if cfg.Duplicate <= 0 {
		cfg.Duplicate = 2 * time.Minute
	}

	sc := &nats.StreamConfig{
		Name:       cfg.Name,
		Subjects:   cfg.Subjects,
		MaxAge:     cfg.MaxAge,
		MaxBytes:   cfg.MaxBytes,
		Storage:    cfg.Storage,
		Replicas:   cfg.Replicas,
		Discard:    cfg.Discard,
		Duplicates: cfg.Duplicate,
	}

	info, err := c.js.StreamInfo(cfg.Name)
	if err != nil {
		if !errors.Is(err, nats.ErrStreamNotFound) {
			return err
		}
		_, err = c.js.AddStream(sc, nats.Context(ctx))
		return err
	}

	sc.Name = info.Config.Name
	_, err = c.js.UpdateStream(sc, nats.Context(ctx))
	return err
}

func (c *Client) Publish(ctx context.Context, subject string, payload []byte, header map[string]string) error {
	if c == nil || c.js == nil {
		return ErrInvalidConfig
	}
	if strings.TrimSpace(subject) == "" {
		return ErrInvalidConfig
	}

	msg := &nats.Msg{
		Subject: subject,
		Data:    payload,
		Header:  nats.Header{},
	}
	for k, v := range header {
		msg.Header.Set(k, v)
	}
	_, err := c.js.PublishMsg(msg, nats.Context(ctx))
	return err
}

func (c *Client) Subscribe(ctx context.Context, subject string, cfg SubscribeConfig, handler Handler) (*nats.Subscription, error) {
	if c == nil || c.js == nil {
		return nil, ErrInvalidConfig
	}
	if strings.TrimSpace(subject) == "" || strings.TrimSpace(cfg.Durable) == "" || handler == nil {
		return nil, ErrInvalidConfig
	}
	if cfg.AckWait <= 0 {
		cfg.AckWait = 5 * time.Second
	}
	if cfg.MaxDeliver <= 0 {
		cfg.MaxDeliver = 5
	}

	cb := func(msg *nats.Msg) {
		m := Message{
			Subject: msg.Subject,
			Header:  map[string]string{},
			Data:    msg.Data,
		}
		for key, values := range msg.Header {
			if len(values) > 0 {
				m.Header[key] = values[0]
			}
		}

		if err := handler(ctx, m); err != nil {
			slog.Error("natsbus handler error", "subject", msg.Subject, "error", err)
			_ = msg.Nak()
			return
		}
		_ = msg.Ack()

		if cfg.OnAck != nil {
			meta, err := msg.Metadata()
			if err == nil {
				cfg.OnAck(meta.Sequence.Stream, meta.Sequence.Consumer)
			}
		}
	}

	deliverOpt := deliverOption(cfg.DeliverPolicy)

	sub, err := c.js.QueueSubscribe(
		subject,
		cfg.Queue,
		cb,
		nats.Durable(cfg.Durable),
		nats.ManualAck(),
		nats.AckWait(cfg.AckWait),
		nats.MaxDeliver(cfg.MaxDeliver),
		deliverOpt,
	)
	if err != nil {
		return nil, fmt.Errorf("natsbus subscribe error: %w", err)
	}
	return sub, nil
}

func deliverOption(policy string) nats.SubOpt {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "new":
		return nats.DeliverNew()
	case "last":
		return nats.DeliverLast()
	case "all":
		return nats.DeliverAll()
	default:
		return nats.DeliverAll()
	}
}
