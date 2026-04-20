package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"quant-system/internal/adapter"
	"quant-system/internal/execution"
	"quant-system/internal/orderfsm"
	"quant-system/internal/position"
	"quant-system/internal/risk"
	"quant-system/pkg/contracts"
)

// stubGateway echoes placements as acks. It satisfies adapter.TradeGateway.
type stubGateway struct {
	placeCalls int
}

func (g *stubGateway) PlaceOrder(_ context.Context, req adapter.VenueOrderRequest) (adapter.VenueOrderAck, error) {
	g.placeCalls++
	return adapter.VenueOrderAck{
		ClientOrderID: req.ClientOrderID,
		VenueOrderID:  "venue-" + req.ClientOrderID,
		Status:        "ack",
	}, nil
}

func (g *stubGateway) CancelOrder(_ context.Context, req adapter.VenueCancelRequest) (adapter.VenueCancelAck, error) {
	return adapter.VenueCancelAck{
		ClientOrderID: req.ClientOrderID,
		VenueOrderID:  req.VenueOrderID,
		Status:        "canceled",
	}, nil
}

type engineFixture struct {
	engine  *Engine
	sink    *MemorySink
	ledger  *position.InMemoryLedger
	fsm     *orderfsm.InMemoryStateMachine
	gateway *stubGateway
	clock   *SimClock
}

func newFixture(t *testing.T, cfgMut func(*Config), riskCfg risk.Config) engineFixture {
	t.Helper()

	gw := &stubGateway{}
	exec, err := execution.NewInMemoryExecutor(gw)
	if err != nil {
		t.Fatalf("new executor: %v", err)
	}
	fsm := orderfsm.NewInMemoryStateMachine()
	ledger := position.NewInMemoryLedger()
	sink := NewMemorySink()
	clock := NewSimClock(time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC))

	cfg := Config{
		AccountID: "acct-test",
		Sink:      sink,
		Clock:     clock,
	}
	if cfgMut != nil {
		cfgMut(&cfg)
	}

	engine, err := NewEngine(risk.NewInMemoryEngine(riskCfg), exec, fsm, ledger, cfg)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	return engineFixture{engine: engine, sink: sink, ledger: ledger, fsm: fsm, gateway: gw, clock: clock}
}

func TestNewEngine_Validation(t *testing.T) {
	gw := &stubGateway{}
	exec, err := execution.NewInMemoryExecutor(gw)
	if err != nil {
		t.Fatal(err)
	}
	fsm := orderfsm.NewInMemoryStateMachine()
	ledger := position.NewInMemoryLedger()
	riskEngine := risk.NewInMemoryEngine(risk.Config{})

	cases := []struct {
		name   string
		risk   risk.RiskEngine
		exec   execution.Executor
		fsm    orderfsm.OrderStateMachine
		ledger position.PositionLedger
		want   error
	}{
		{"risk_nil", nil, exec, fsm, ledger, ErrRiskNil},
		{"exec_nil", riskEngine, nil, fsm, ledger, ErrExecNil},
		{"fsm_nil", riskEngine, exec, nil, ledger, ErrFSMNil},
		{"ledger_nil", riskEngine, exec, fsm, nil, ErrLedgerNil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewEngine(tc.risk, tc.exec, tc.fsm, tc.ledger, Config{})
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want=%v", err, tc.want)
			}
		})
	}
}

func TestEngine_HandleIntent_AllowAndAck(t *testing.T) {
	f := newFixture(t, nil, risk.Config{})

	result, err := f.engine.HandleIntent(context.Background(), sampleIntent("i1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Rejected {
		t.Fatal("expected allow, got rejected")
	}
	if result.Submit.ClientOrderID == "" {
		t.Fatal("submit.ClientOrderID empty")
	}
	if result.Filled {
		t.Fatal("simulateFill=false, should not be filled")
	}

	// Exactly one risk decision + one lifecycle (ack) + zero fills.
	if got := len(f.sink.Decisions()); got != 1 {
		t.Fatalf("decisions=%d want=1", got)
	}
	lc := f.sink.Lifecycle()
	if len(lc) != 1 || lc[0].State != string(orderfsm.StateAck) {
		t.Fatalf("lifecycle=%+v want single ack", lc)
	}
	if got := len(f.sink.Fills()); got != 0 {
		t.Fatalf("fills=%d want=0", got)
	}
	if f.gateway.placeCalls != 1 {
		t.Fatalf("placeCalls=%d want=1", f.gateway.placeCalls)
	}
	// Ack event stamped with sim clock.
	if lc[0].EmitTSMS != f.clock.UnixMilli() {
		t.Fatalf("ack EmitTSMS=%d clock=%d", lc[0].EmitTSMS, f.clock.UnixMilli())
	}
}

func TestEngine_HandleIntent_Reject(t *testing.T) {
	f := newFixture(t, nil, risk.Config{MaxOrderQty: 1})

	// Quantity exceeds limit → reject.
	intent := sampleIntent("i2")
	intent.Quantity = 10

	result, err := f.engine.HandleIntent(context.Background(), intent)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if !result.Rejected {
		t.Fatal("expected rejected")
	}
	if result.Submit.ClientOrderID != "" {
		t.Fatal("no submit expected on reject")
	}
	if got := len(f.sink.Lifecycle()); got != 0 {
		t.Fatalf("no lifecycle expected on reject, got=%d", got)
	}
	if f.gateway.placeCalls != 0 {
		t.Fatalf("gateway should not be called on reject, placeCalls=%d", f.gateway.placeCalls)
	}
}

func TestEngine_HandleIntent_SimulatedFill(t *testing.T) {
	f := newFixture(t, func(c *Config) { c.SimulateFill = true }, risk.Config{})

	intent := sampleIntent("i3")
	result, err := f.engine.HandleIntent(context.Background(), intent)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if !result.Filled {
		t.Fatal("expected filled")
	}
	if result.FillEvent.FillQty != intent.Quantity {
		t.Fatalf("fill qty=%v want=%v", result.FillEvent.FillQty, intent.Quantity)
	}

	// Lifecycle: ack + filled. Fills: 1. Ledger updated.
	lc := f.sink.Lifecycle()
	if len(lc) != 2 {
		t.Fatalf("lifecycle events=%d want=2 (ack+filled)", len(lc))
	}
	if lc[0].State != string(orderfsm.StateAck) || lc[1].State != string(orderfsm.StateFilled) {
		t.Fatalf("lifecycle order=%v want=[ack,filled]", []string{lc[0].State, lc[1].State})
	}
	if got := len(f.sink.Fills()); got != 1 {
		t.Fatalf("fills=%d want=1", got)
	}
	snap, ok := f.ledger.Get("acct-test", intent.Symbol)
	if !ok {
		t.Fatal("ledger missing acct-test position")
	}
	if snap.Quantity != intent.Quantity {
		t.Fatalf("ledger qty=%v want=%v", snap.Quantity, intent.Quantity)
	}
}

func TestEngine_ApplyFill_External(t *testing.T) {
	f := newFixture(t, nil, risk.Config{})

	intent := sampleIntent("i4")
	result, err := f.engine.HandleIntent(context.Background(), intent)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	// Advance clock and inject an external fill — e.g. from exchange exec report.
	f.clock.Advance(150_000_000) // 150ms in nanos
	fillTS := f.clock.UnixMilli()
	fill := contracts.TradeFillEvent{
		TradeID:    "ext-fill-1",
		AccountID:  "acct-test",
		Symbol:     intent.Symbol,
		Side:       intent.Side,
		FillQty:    intent.Quantity,
		FillPrice:  intent.Price + 0.5, // slight slippage
		SourceTSMS: fillTS,
	}
	if err := f.engine.ApplyFill(context.Background(), intent, result.Submit, fill); err != nil {
		t.Fatalf("apply fill: %v", err)
	}

	if got := len(f.sink.Fills()); got != 1 {
		t.Fatalf("fills=%d want=1", got)
	}
	snap, ok := f.ledger.Get("acct-test", intent.Symbol)
	if !ok {
		t.Fatal("ledger missing position")
	}
	// avg cost = fill_price = intent.Price + 0.5
	wantAvg := intent.Price + 0.5
	if snap.AvgCost != wantAvg {
		t.Fatalf("avg cost=%v want=%v", snap.AvgCost, wantAvg)
	}
}

func TestEngine_ExecutorResolverOverride(t *testing.T) {
	callsA := 0
	callsB := 0
	gwA := &stubGateway{}
	gwB := &stubGateway{}
	execA, _ := execution.NewInMemoryExecutor(gwA)
	execB, _ := execution.NewInMemoryExecutor(gwB)

	resolver := func(_ context.Context, intent contracts.OrderIntent) (execution.Executor, error) {
		if intent.StrategyID == "use-b" {
			callsB++
			return execB, nil
		}
		callsA++
		return execA, nil
	}

	cfg := Config{
		AccountID:    "acct",
		Sink:         NewMemorySink(),
		ExecResolver: resolver,
	}
	engine, err := NewEngine(
		risk.NewInMemoryEngine(risk.Config{}),
		nil, // no default; resolver always returns one
		orderfsm.NewInMemoryStateMachine(),
		position.NewInMemoryLedger(),
		cfg,
	)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	intentA := sampleIntent("a1")
	intentA.StrategyID = "use-a"
	intentB := sampleIntent("b1")
	intentB.StrategyID = "use-b"

	if _, err := engine.HandleIntent(context.Background(), intentA); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.HandleIntent(context.Background(), intentB); err != nil {
		t.Fatal(err)
	}

	if callsA != 1 || callsB != 1 {
		t.Fatalf("resolver calls A=%d B=%d want each=1", callsA, callsB)
	}
	if gwA.placeCalls != 1 || gwB.placeCalls != 1 {
		t.Fatalf("gateway placeCalls A=%d B=%d want each=1", gwA.placeCalls, gwB.placeCalls)
	}
}

func sampleIntent(id string) contracts.OrderIntent {
	return contracts.OrderIntent{
		IntentID:   id,
		StrategyID: "strat-1",
		Symbol:     "BTCUSDT",
		Side:       "buy",
		Price:      20000,
		Quantity:   0.1,
	}
}

