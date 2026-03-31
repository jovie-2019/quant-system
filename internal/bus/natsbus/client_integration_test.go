package natsbus

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"quant-system/pkg/contracts"
)

func TestJetStreamPublishSubscribeRetry(t *testing.T) {
	if os.Getenv("RUN_NATS_TESTS") != "1" {
		t.Skip("skip nats integration test (set RUN_NATS_TESTS=1)")
	}

	url := os.Getenv("NATS_URL")
	if url == "" {
		url = "nats://127.0.0.1:4222"
	}

	client, err := Connect(Config{
		URL:           url,
		Name:          "quant-natsbus-it",
		ConnectWait:   5 * time.Second,
		ReconnectWait: 300 * time.Millisecond,
		MaxReconnects: 3,
	})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	streamName := "TEST_TRADING_BUS"
	if err := client.EnsureStream(ctx, StreamConfig{
		Name:     streamName,
		Subjects: []string{"test.trading.>"},
		MaxAge:   time.Hour,
		Storage:  nats.MemoryStorage,
	}); err != nil {
		t.Fatalf("EnsureStream() error = %v", err)
	}
	defer func() {
		_ = client.js.DeleteStream(streamName)
	}()

	var attempts atomic.Int64
	delivered := make(chan struct{}, 1)

	sub, err := client.Subscribe(ctx, "test.trading.>", SubscribeConfig{
		Durable:    "durable-test-bus",
		Queue:      "q-test-bus",
		AckWait:    300 * time.Millisecond,
		MaxDeliver: 3,
	}, func(_ context.Context, _ Message) error {
		n := attempts.Add(1)
		if n == 1 {
			return errors.New("force retry")
		}
		select {
		case delivered <- struct{}{}:
		default:
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer sub.Unsubscribe()

	if err := client.Publish(ctx, "test.trading.events", []byte(`{"event":"x"}`), map[string]string{"x-trace-id": "t-1"}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	select {
	case <-delivered:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting retry delivery")
	}

	if attempts.Load() < 2 {
		t.Fatalf("expected at least 2 attempts, got %d", attempts.Load())
	}
}

func TestReplayTradeFill(t *testing.T) {
	if os.Getenv("RUN_NATS_TESTS") != "1" {
		t.Skip("skip nats integration test (set RUN_NATS_TESTS=1)")
	}

	url := os.Getenv("NATS_URL")
	if url == "" {
		url = "nats://127.0.0.1:4222"
	}

	client, err := Connect(Config{
		URL:           url,
		Name:          "quant-natsbus-replay-it",
		ConnectWait:   5 * time.Second,
		ReconnectWait: 300 * time.Millisecond,
		MaxReconnects: 3,
	})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	streamName := "TEST_REPLAY_BUS"
	if err := client.EnsureStream(ctx, StreamConfig{
		Name:     streamName,
		Subjects: []string{"trade.fill.>"},
		MaxAge:   time.Hour,
		Storage:  nats.MemoryStorage,
	}); err != nil {
		t.Fatalf("EnsureStream() error = %v", err)
	}
	defer func() {
		_ = client.js.DeleteStream(streamName)
	}()

	fill := contracts.TradeFillEvent{
		TradeID:   "trade-replay-1",
		AccountID: "acc-replay-1",
		Symbol:    "BTC-USDT",
		Side:      "buy",
		FillQty:   0.2,
		FillPrice: 62000,
	}
	if err := PublishTradeFill(ctx, client, "acc-replay-1", fill, map[string]string{"x-trace-id": "r1"}); err != nil {
		t.Fatalf("PublishTradeFill() error = %v", err)
	}

	replayed := make([]contracts.TradeFillEvent, 0, 4)
	err = ReplayTradeFill(ctx, client, "trade.fill.acc-replay-1.>", ReplayOptions{
		Durable:  "durable-replay-fill",
		Batch:    16,
		MaxWait:  300 * time.Millisecond,
		AckAfter: true,
	}, func(evt contracts.TradeFillEvent) error {
		replayed = append(replayed, evt)
		return nil
	})
	if err != nil {
		t.Fatalf("ReplayTradeFill() error = %v", err)
	}

	if len(replayed) == 0 {
		t.Fatalf("expected replayed events > 0")
	}
	if replayed[0].TradeID != fill.TradeID {
		t.Fatalf("unexpected replay event: %+v", replayed[0])
	}
}
