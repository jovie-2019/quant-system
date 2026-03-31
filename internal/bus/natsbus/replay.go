package natsbus

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"

	"quant-system/pkg/contracts"
)

type ReplayOptions struct {
	Durable  string
	Batch    int
	MaxWait  time.Duration
	AckAfter bool
}

func ReplayTradeFill(ctx context.Context, client *Client, subject string, opts ReplayOptions, fn func(contracts.TradeFillEvent) error) error {
	if client == nil || client.js == nil {
		return ErrInvalidConfig
	}
	if opts.Durable == "" {
		return fmt.Errorf("natsbus: durable is required")
	}
	if opts.Batch <= 0 {
		opts.Batch = 64
	}
	if opts.MaxWait <= 0 {
		opts.MaxWait = 2 * time.Second
	}

	sub, err := client.js.PullSubscribe(subject, opts.Durable, nats.DeliverAll())
	if err != nil {
		return err
	}

	for {
		if ctx.Err() != nil {
			return nil
		}

		msgs, err := sub.Fetch(opts.Batch, nats.MaxWait(opts.MaxWait))
		if err != nil {
			if err == nats.ErrTimeout {
				return nil
			}
			return err
		}
		if len(msgs) == 0 {
			return nil
		}

		for _, msg := range msgs {
			var evt contracts.TradeFillEvent
			if err := json.Unmarshal(msg.Data, &evt); err != nil {
				_ = msg.Term()
				continue
			}
			if err := fn(evt); err != nil {
				_ = msg.Nak()
				return err
			}
			if opts.AckAfter {
				_ = msg.Ack()
			}
		}
	}
}
