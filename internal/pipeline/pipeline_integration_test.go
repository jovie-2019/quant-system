package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"quant-system/internal/bus/natsbus"
	"quant-system/internal/execution"
	"quant-system/internal/orderfsm"
	"quant-system/internal/position"
	"quant-system/internal/risk"
	"quant-system/pkg/contracts"
)

func TestPipelinePublishesLifecycleEvents(t *testing.T) {
	if os.Getenv("RUN_NATS_TESTS") != "1" {
		t.Skip("skip nats integration test (set RUN_NATS_TESTS=1)")
	}

	url := os.Getenv("NATS_URL")
	if url == "" {
		url = "nats://127.0.0.1:4222"
	}

	client, err := natsbus.Connect(natsbus.Config{
		URL:           url,
		Name:          "quant-pipeline-it",
		ConnectWait:   5 * time.Second,
		ReconnectWait: 300 * time.Millisecond,
		MaxReconnects: 3,
	})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	suffix := time.Now().UnixNano()
	accountID := fmt.Sprintf("acc-pipeline-it-%d", suffix)
	intentSubject := fmt.Sprintf("test.strategy.intent.%d", suffix)

	if err := client.EnsureStream(ctx, natsbus.StreamConfig{
		Name:     fmt.Sprintf("TEST_PIPELINE_INTENT_%d", suffix),
		Subjects: []string{intentSubject},
		MaxAge:   time.Hour,
		Storage:  nats.MemoryStorage,
	}); err != nil {
		t.Fatalf("EnsureStream(intent) error = %v", err)
	}
	if err := client.EnsureStream(ctx, natsbus.StreamConfig{
		Name:     fmt.Sprintf("TEST_PIPELINE_RISK_%d", suffix),
		Subjects: []string{fmt.Sprintf("risk.decision.%s", accountID)},
		MaxAge:   time.Hour,
		Storage:  nats.MemoryStorage,
	}); err != nil {
		t.Fatalf("EnsureStream(risk) error = %v", err)
	}
	if err := client.EnsureStream(ctx, natsbus.StreamConfig{
		Name:     fmt.Sprintf("TEST_PIPELINE_ORDERS_%d", suffix),
		Subjects: []string{fmt.Sprintf("order.lifecycle.%s.>", accountID)},
		MaxAge:   time.Hour,
		Storage:  nats.MemoryStorage,
	}); err != nil {
		t.Fatalf("EnsureStream(order) error = %v", err)
	}
	if err := client.EnsureStream(ctx, natsbus.StreamConfig{
		Name:     fmt.Sprintf("TEST_PIPELINE_FILLS_%d", suffix),
		Subjects: []string{fmt.Sprintf("trade.fill.%s.>", accountID)},
		MaxAge:   time.Hour,
		Storage:  nats.MemoryStorage,
	}); err != nil {
		t.Fatalf("EnsureStream(fill) error = %v", err)
	}

	riskCh := make(chan contracts.RiskDecision, 2)
	orderCh := make(chan contracts.OrderLifecycleEvent, 8)
	fillCh := make(chan contracts.TradeFillEvent, 2)

	riskSub, err := client.Subscribe(ctx, fmt.Sprintf("risk.decision.%s", accountID), natsbus.SubscribeConfig{
		Durable:    fmt.Sprintf("dur-risk-%d", suffix),
		Queue:      fmt.Sprintf("q-risk-%d", suffix),
		AckWait:    time.Second,
		MaxDeliver: 2,
	}, func(_ context.Context, msg natsbus.Message) error {
		var evt contracts.RiskDecision
		if err := json.Unmarshal(msg.Data, &evt); err != nil {
			return err
		}
		riskCh <- evt
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe(risk) error = %v", err)
	}
	defer riskSub.Unsubscribe()

	orderSub, err := client.Subscribe(ctx, fmt.Sprintf("order.lifecycle.%s.>", accountID), natsbus.SubscribeConfig{
		Durable:    fmt.Sprintf("dur-order-%d", suffix),
		Queue:      fmt.Sprintf("q-order-%d", suffix),
		AckWait:    time.Second,
		MaxDeliver: 2,
	}, func(_ context.Context, msg natsbus.Message) error {
		var evt contracts.OrderLifecycleEvent
		if err := json.Unmarshal(msg.Data, &evt); err != nil {
			return err
		}
		orderCh <- evt
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe(order) error = %v", err)
	}
	defer orderSub.Unsubscribe()

	fillSub, err := client.Subscribe(ctx, fmt.Sprintf("trade.fill.%s.>", accountID), natsbus.SubscribeConfig{
		Durable:    fmt.Sprintf("dur-fill-%d", suffix),
		Queue:      fmt.Sprintf("q-fill-%d", suffix),
		AckWait:    time.Second,
		MaxDeliver: 2,
	}, func(_ context.Context, msg natsbus.Message) error {
		var evt contracts.TradeFillEvent
		if err := json.Unmarshal(msg.Data, &evt); err != nil {
			return err
		}
		fillCh <- evt
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe(fill) error = %v", err)
	}
	defer fillSub.Unsubscribe()

	riskEngine := risk.NewInMemoryEngine(risk.Config{
		MaxOrderQty:    10,
		MaxOrderAmount: 1_000_000,
		AllowedSymbols: map[string]struct{}{"BTC-USDT": {}},
	})
	gw := &fakeGateway{}
	exec, err := execution.NewInMemoryExecutor(gw)
	if err != nil {
		t.Fatalf("NewInMemoryExecutor() error = %v", err)
	}
	fsm := orderfsm.NewInMemoryStateMachine()
	ledger := position.NewInMemoryLedger()

	p, err := New(client, riskEngine, exec, fsm, ledger, nil, Config{
		AccountID:    accountID,
		Subject:      intentSubject,
		SimulateFill: true,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	intent := contracts.OrderIntent{
		IntentID:    fmt.Sprintf("intent-pipeline-it-%d", suffix),
		StrategyID:  "s1",
		Symbol:      "BTC-USDT",
		Side:        "buy",
		Price:       60000,
		Quantity:    0.1,
		TimeInForce: "IOC",
	}
	intentJSON, _ := json.Marshal(intent)

	if err := p.handleIntent(ctx, natsbus.Message{
		Subject: intentSubject,
		Data:    intentJSON,
	}); err != nil {
		t.Fatalf("handleIntent() error = %v", err)
	}

	select {
	case evt := <-riskCh:
		if evt.Decision != risk.DecisionAllow {
			t.Fatalf("expected allow risk decision, got %+v", evt)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting risk decision")
	}

	gotAck := false
	gotFilled := false
	deadline := time.After(5 * time.Second)
	for !(gotAck && gotFilled) {
		select {
		case evt := <-orderCh:
			if evt.State == string(orderfsm.StateAck) {
				gotAck = true
			}
			if evt.State == string(orderfsm.StateFilled) {
				gotFilled = true
			}
		case <-deadline:
			t.Fatalf("timeout waiting lifecycle events ack=%v filled=%v", gotAck, gotFilled)
		}
	}

	select {
	case evt := <-fillCh:
		if evt.Symbol != "BTC-USDT" || evt.FillQty != 0.1 {
			t.Fatalf("unexpected fill event: %+v", evt)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting fill event")
	}
}
