package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"quant-system/internal/bus/natsbus"
	"quant-system/internal/execution"
	"quant-system/internal/orderfsm"
	"quant-system/internal/position"
	"quant-system/internal/risk"
	"quant-system/pkg/contracts"
)

func TestNewPipelineValidation(t *testing.T) {
	r := risk.NewInMemoryEngine(risk.Config{})
	gw := &fakeGateway{}
	exec, _ := execution.NewInMemoryExecutor(gw)
	fsm := orderfsm.NewInMemoryStateMachine()
	ledger := position.NewInMemoryLedger()

	tests := []struct {
		name string
		fn   func() error
		want error
	}{
		{"nil bus", func() error {
			_, err := New(nil, r, exec, fsm, ledger, nil, Config{})
			return err
		}, ErrBusNil},
		{"nil risk", func() error {
			_, err := New(&natsbus.Client{}, nil, exec, fsm, ledger, nil, Config{})
			return err
		}, ErrRiskNil},
		{"nil exec", func() error {
			_, err := New(&natsbus.Client{}, r, nil, fsm, ledger, nil, Config{})
			return err
		}, ErrExecNil},
		{"nil fsm", func() error {
			_, err := New(&natsbus.Client{}, r, exec, nil, ledger, nil, Config{})
			return err
		}, ErrFSMNil},
		{"nil ledger", func() error {
			_, err := New(&natsbus.Client{}, r, exec, fsm, nil, nil, Config{})
			return err
		}, ErrLedgerNil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); err != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, err)
			}
		})
	}
}

func TestHandleIntentHappyPath(t *testing.T) {
	ctx := context.Background()

	riskEngine := risk.NewInMemoryEngine(risk.Config{
		MaxOrderQty:    10,
		MaxOrderAmount: 1_000_000,
		AllowedSymbols: map[string]struct{}{"BTC-USDT": {}},
	})
	gw := &fakeGateway{}
	exec, err := execution.NewInMemoryExecutor(gw)
	if err != nil {
		t.Fatal(err)
	}
	fsm := orderfsm.NewInMemoryStateMachine()
	ledger := position.NewInMemoryLedger()
	store := &fakePersister{}
	bus := &natsbus.Client{}

	p, err := New(bus, riskEngine, exec, fsm, ledger, store, Config{
		AccountID:    "acc-1",
		SimulateFill: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	intent := contracts.OrderIntent{
		IntentID:    "test-intent-1",
		StrategyID:  "s1",
		Symbol:      "BTC-USDT",
		Side:        "buy",
		Price:       60000,
		Quantity:    0.1,
		TimeInForce: "IOC",
	}
	intentJSON, _ := json.Marshal(intent)

	err = p.handleIntent(ctx, natsbus.Message{
		Subject: "strategy.intent.s1",
		Data:    intentJSON,
	})
	if err != nil {
		t.Fatalf("handleIntent() error = %v", err)
	}

	// Verify gateway was called.
	if gw.placeCalls != 1 {
		t.Fatalf("expected 1 place call, got %d", gw.placeCalls)
	}

	// Verify FSM is in filled state.
	clientOrderID := "cid-test-intent-1"
	order, ok := fsm.Get(clientOrderID)
	if !ok {
		t.Fatal("order not found in FSM")
	}
	if order.State != orderfsm.StateFilled {
		t.Fatalf("expected filled, got %s", order.State)
	}

	// Verify position.
	pos, ok := ledger.Get("acc-1", "BTC-USDT")
	if !ok || pos.Quantity != 0.1 {
		t.Fatalf("expected position qty=0.1, got %+v", pos)
	}

	// Verify persistence was called.
	if store.riskDecisions != 1 {
		t.Fatalf("expected 1 risk decision persisted, got %d", store.riskDecisions)
	}
	if store.orders < 2 { // ack + filled
		t.Fatalf("expected >=2 order upserts, got %d", store.orders)
	}
	if store.positions != 1 {
		t.Fatalf("expected 1 position upsert, got %d", store.positions)
	}
}

func TestHandleIntentRejected(t *testing.T) {
	ctx := context.Background()

	riskEngine := risk.NewInMemoryEngine(risk.Config{
		MaxOrderQty:    0.05, // intent qty=0.1 will exceed this
		AllowedSymbols: map[string]struct{}{"BTC-USDT": {}},
	})
	gw := &fakeGateway{}
	exec, _ := execution.NewInMemoryExecutor(gw)
	fsm := orderfsm.NewInMemoryStateMachine()
	ledger := position.NewInMemoryLedger()
	bus := &natsbus.Client{}

	p, err := New(bus, riskEngine, exec, fsm, ledger, nil, Config{
		SimulateFill: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	intent := contracts.OrderIntent{
		IntentID:   "test-reject-1",
		StrategyID: "s1",
		Symbol:     "BTC-USDT",
		Side:       "buy",
		Price:      60000,
		Quantity:   0.1,
	}
	intentJSON, _ := json.Marshal(intent)

	err = p.handleIntent(ctx, natsbus.Message{
		Subject: "strategy.intent.s1",
		Data:    intentJSON,
	})
	if err != nil {
		t.Fatalf("handleIntent() error = %v", err)
	}

	// Gateway should NOT have been called.
	if gw.placeCalls != 0 {
		t.Fatalf("expected 0 place calls, got %d", gw.placeCalls)
	}
}

func TestHandleIntentExecutionErrorReturnsError(t *testing.T) {
	ctx := context.Background()

	riskEngine := risk.NewInMemoryEngine(risk.Config{
		MaxOrderQty:    10,
		MaxOrderAmount: 1_000_000,
		AllowedSymbols: map[string]struct{}{"BTC-USDT": {}},
	})
	gw := &fakeGateway{placeErr: errors.New("venue unavailable")}
	exec, _ := execution.NewInMemoryExecutor(gw)
	fsm := orderfsm.NewInMemoryStateMachine()
	ledger := position.NewInMemoryLedger()
	bus := &natsbus.Client{}

	p, err := New(bus, riskEngine, exec, fsm, ledger, nil, Config{
		SimulateFill: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	intent := contracts.OrderIntent{
		IntentID:   "test-exec-error-1",
		StrategyID: "s1",
		Symbol:     "BTC-USDT",
		Side:       "buy",
		Price:      60000,
		Quantity:   0.1,
	}
	intentJSON, _ := json.Marshal(intent)

	err = p.handleIntent(ctx, natsbus.Message{
		Subject: "strategy.intent.s1",
		Data:    intentJSON,
	})
	if err == nil {
		t.Fatal("expected handleIntent to return execution error")
	}
	if gw.placeCalls != 1 {
		t.Fatalf("expected 1 place call, got %d", gw.placeCalls)
	}
}

func TestHandleIntentRetryAfterFillFailure(t *testing.T) {
	ctx := context.Background()

	riskEngine := risk.NewInMemoryEngine(risk.Config{
		MaxOrderQty:    10,
		MaxOrderAmount: 1_000_000,
		AllowedSymbols: map[string]struct{}{"BTC-USDT": {}},
	})
	gw := &fakeGateway{}
	exec, _ := execution.NewInMemoryExecutor(gw)
	fsm := orderfsm.NewInMemoryStateMachine()
	ledger := &retryLedger{
		inner:             position.NewInMemoryLedger(),
		remainingFailures: 1,
	}
	bus := &natsbus.Client{}

	p, err := New(bus, riskEngine, exec, fsm, ledger, nil, Config{
		AccountID:    "acc-1",
		SimulateFill: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	intent := contracts.OrderIntent{
		IntentID:    "test-fill-retry-1",
		StrategyID:  "s1",
		Symbol:      "BTC-USDT",
		Side:        "buy",
		Price:       60000,
		Quantity:    0.1,
		TimeInForce: "IOC",
	}
	intentJSON, _ := json.Marshal(intent)
	msg := natsbus.Message{
		Subject: "strategy.intent.s1",
		Data:    intentJSON,
	}

	if err := p.handleIntent(ctx, msg); err == nil {
		t.Fatal("expected first handleIntent to fail on fill application")
	}
	if err := p.handleIntent(ctx, msg); err != nil {
		t.Fatalf("expected retry to succeed, got error: %v", err)
	}

	if gw.placeCalls != 1 {
		t.Fatalf("expected idempotent place call=1, got %d", gw.placeCalls)
	}

	order, ok := fsm.Get("cid-test-fill-retry-1")
	if !ok || order.State != orderfsm.StateFilled {
		t.Fatalf("expected filled order, got %+v found=%v", order, ok)
	}

	pos, ok := ledger.Get("acc-1", "BTC-USDT")
	if !ok || pos.Quantity != 0.1 {
		t.Fatalf("expected position qty=0.1, got %+v found=%v", pos, ok)
	}
}

// --- fakes ---

type fakeGateway struct {
	placeCalls int
	placeErr   error
}

func (g *fakeGateway) PlaceOrder(_ context.Context, req contracts.VenueOrderRequest) (contracts.VenueOrderAck, error) {
	g.placeCalls++
	if g.placeErr != nil {
		return contracts.VenueOrderAck{}, g.placeErr
	}
	return contracts.VenueOrderAck{
		ClientOrderID: req.ClientOrderID,
		VenueOrderID:  "vo-" + req.ClientOrderID,
		Status:        "ack",
	}, nil
}

func (g *fakeGateway) CancelOrder(_ context.Context, req contracts.VenueCancelRequest) (contracts.VenueCancelAck, error) {
	return contracts.VenueCancelAck{
		ClientOrderID: req.ClientOrderID,
		VenueOrderID:  req.VenueOrderID,
		Status:        "canceled",
	}, nil
}

type fakePersister struct {
	riskDecisions int
	orders        int
	positions     int
}

func (f *fakePersister) SaveRiskDecision(_ context.Context, _ contracts.RiskDecision) error {
	f.riskDecisions++
	return nil
}

func (f *fakePersister) UpsertOrder(_ context.Context, _ contracts.Order) error {
	f.orders++
	return nil
}

func (f *fakePersister) UpsertPosition(_ context.Context, _ contracts.PositionSnapshot) error {
	f.positions++
	return nil
}

type retryLedger struct {
	inner             *position.InMemoryLedger
	remainingFailures int
}

func (l *retryLedger) ApplyFill(ctx context.Context, fill position.TradeFillEvent) (position.PositionSnapshot, error) {
	if l.remainingFailures > 0 {
		l.remainingFailures--
		return position.PositionSnapshot{}, errors.New("temporary ledger failure")
	}
	return l.inner.ApplyFill(ctx, fill)
}

func (l *retryLedger) Get(accountID, symbol string) (position.PositionSnapshot, bool) {
	return l.inner.Get(accountID, symbol)
}
