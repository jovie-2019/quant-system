package strategyrunner

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"quant-system/internal/bus/natsbus"
	"quant-system/internal/strategy"
	"quant-system/pkg/contracts"
)

// fakeReloader implements strategy.Strategy + strategy.ParamReloader so
// the control handler's dispatch logic can be exercised without wiring a
// live momentum instance.
type fakeReloader struct {
	id          string
	applyCalls  int
	lastParams  json.RawMessage
	applyErr    error
}

func (f *fakeReloader) ID() string { return f.id }
func (f *fakeReloader) OnMarket(_ contracts.MarketNormalizedEvent) []contracts.OrderIntent {
	return nil
}
func (f *fakeReloader) ApplyParams(raw json.RawMessage) error {
	f.applyCalls++
	f.lastParams = append(f.lastParams[:0], raw...)
	return f.applyErr
}

// fakeNonReloader is a minimal Strategy that does NOT implement
// ParamReloader; handler should reject update_params.
type fakeNonReloader struct{ id string }

func (f *fakeNonReloader) ID() string { return f.id }
func (f *fakeNonReloader) OnMarket(_ contracts.MarketNormalizedEvent) []contracts.OrderIntent {
	return nil
}

func newTestHandler(t *testing.T, s strategy.Strategy) *ControlHandler {
	t.Helper()
	h, err := NewControlHandler(&natsbus.Client{}, s, ControlConfig{StrategyID: s.ID()})
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}
	return h
}

func envelope(t contracts.StrategyControlType, rev int64, params json.RawMessage) natsbus.Message {
	env := contracts.StrategyControlEnvelope{
		StrategyID: "s1",
		Type:       t,
		Revision:   rev,
		Params:     params,
	}
	data, _ := json.Marshal(env)
	return natsbus.Message{Data: data, Subject: "strategy.control.s1"}
}

func TestControlHandler_UpdateParamsSuccess(t *testing.T) {
	r := &fakeReloader{id: "s1"}
	h := newTestHandler(t, r)

	msg := envelope(contracts.StrategyControlUpdateParams, 1, json.RawMessage(`{"symbol":"X"}`))
	if err := h.handleMessage(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	if r.applyCalls != 1 {
		t.Fatalf("applyCalls=%d want 1", r.applyCalls)
	}
	if h.Snapshot().Revision != 1 {
		t.Fatalf("revision=%d want 1", h.Snapshot().Revision)
	}
}

func TestControlHandler_StaleRevisionRejected(t *testing.T) {
	r := &fakeReloader{id: "s1"}
	h := newTestHandler(t, r)

	_ = h.handleMessage(context.Background(), envelope(contracts.StrategyControlUpdateParams, 5, json.RawMessage(`{"symbol":"X"}`)))
	// Stale: revision 3 <= current 5 → rejected.
	_ = h.handleMessage(context.Background(), envelope(contracts.StrategyControlUpdateParams, 3, json.RawMessage(`{"symbol":"X"}`)))
	if r.applyCalls != 1 {
		t.Fatalf("applyCalls=%d want 1 (stale revision should be rejected)", r.applyCalls)
	}
	if h.Snapshot().Revision != 5 {
		t.Fatalf("revision=%d want 5 (stale should not advance)", h.Snapshot().Revision)
	}
}

func TestControlHandler_UpdateParamsOnNonReloaderRejected(t *testing.T) {
	h := newTestHandler(t, &fakeNonReloader{id: "s1"})
	_ = h.handleMessage(context.Background(), envelope(contracts.StrategyControlUpdateParams, 1, json.RawMessage(`{}`)))
	if h.Snapshot().Revision != 0 {
		t.Fatalf("revision advanced despite non-reloadable strategy: %d", h.Snapshot().Revision)
	}
}

func TestControlHandler_ApplyParamsError(t *testing.T) {
	r := &fakeReloader{id: "s1", applyErr: errors.New("bad")}
	h := newTestHandler(t, r)
	_ = h.handleMessage(context.Background(), envelope(contracts.StrategyControlUpdateParams, 1, json.RawMessage(`{}`)))
	if r.applyCalls != 1 {
		t.Fatalf("ApplyParams was not invoked")
	}
	if h.Snapshot().Revision != 0 {
		t.Fatalf("revision advanced despite apply error: %d", h.Snapshot().Revision)
	}
}

func TestControlHandler_PauseResume(t *testing.T) {
	h := newTestHandler(t, &fakeReloader{id: "s1"})
	if h.IsPaused() {
		t.Fatal("paused at boot")
	}
	_ = h.handleMessage(context.Background(), envelope(contracts.StrategyControlPause, 1, nil))
	if !h.IsPaused() {
		t.Fatal("not paused after pause command")
	}
	_ = h.handleMessage(context.Background(), envelope(contracts.StrategyControlResume, 2, nil))
	if h.IsPaused() {
		t.Fatal("still paused after resume")
	}
}

func TestControlHandler_ShadowOnOff(t *testing.T) {
	h := newTestHandler(t, &fakeReloader{id: "s1"})
	_ = h.handleMessage(context.Background(), envelope(contracts.StrategyControlShadowOn, 1, nil))
	if !h.IsShadow() {
		t.Fatal("shadow not on")
	}
	_ = h.handleMessage(context.Background(), envelope(contracts.StrategyControlShadowOff, 2, nil))
	if h.IsShadow() {
		t.Fatal("shadow still on after shadow_off")
	}
}

func TestControlHandler_UnknownType(t *testing.T) {
	h := newTestHandler(t, &fakeReloader{id: "s1"})
	msg := envelope("weird_type", 1, nil)
	_ = h.handleMessage(context.Background(), msg)
	if h.Snapshot().Revision != 0 {
		t.Fatal("revision advanced on unknown type")
	}
}

func TestControlHandler_MalformedPayloadDoesNotRetry(t *testing.T) {
	h := newTestHandler(t, &fakeReloader{id: "s1"})
	// Bad JSON returns nil (no retry) but publishes a rejecting ack.
	err := h.handleMessage(context.Background(), natsbus.Message{Data: []byte("not json")})
	if err != nil {
		t.Fatalf("err=%v want nil (malformed payloads should not retry)", err)
	}
}

func TestGatedIntentSink_PausedDropsIntents(t *testing.T) {
	h := newTestHandler(t, &fakeReloader{id: "s1"})
	h.paused.Store(true)

	delivered := 0
	live := func(_ context.Context, _ strategy.OrderIntent) error {
		delivered++
		return nil
	}
	gate := GatedIntentSink(h, live, nil)
	if err := gate(context.Background(), strategy.OrderIntent{StrategyID: "s1", IntentID: "i1"}); err != nil {
		t.Fatal(err)
	}
	if delivered != 0 {
		t.Fatalf("delivered=%d want 0 (paused)", delivered)
	}
}

func TestGatedIntentSink_LiveForwards(t *testing.T) {
	h := newTestHandler(t, &fakeReloader{id: "s1"})

	delivered := 0
	live := func(_ context.Context, _ strategy.OrderIntent) error {
		delivered++
		return nil
	}
	gate := GatedIntentSink(h, live, nil)
	if err := gate(context.Background(), strategy.OrderIntent{StrategyID: "s1", IntentID: "i1"}); err != nil {
		t.Fatal(err)
	}
	if delivered != 1 {
		t.Fatalf("delivered=%d want 1", delivered)
	}
}

func TestGatedIntentSink_ShadowSkipsLive(t *testing.T) {
	h := newTestHandler(t, &fakeReloader{id: "s1"})
	h.shadow.Store(true)

	delivered := 0
	live := func(_ context.Context, _ strategy.OrderIntent) error {
		delivered++
		return nil
	}
	// bus=nil → shadow drops silently (no panic).
	gate := GatedIntentSink(h, live, nil)
	if err := gate(context.Background(), strategy.OrderIntent{StrategyID: "s1", IntentID: "i1"}); err != nil {
		t.Fatal(err)
	}
	if delivered != 0 {
		t.Fatalf("delivered=%d want 0 (shadow should skip live path)", delivered)
	}
}
