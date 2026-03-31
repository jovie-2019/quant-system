package strategyrunner

import (
	"context"
	"errors"
	"testing"

	"quant-system/internal/bus/natsbus"
	"quant-system/pkg/contracts"
)

type fakeRuntime struct {
	calls int
	last  contracts.MarketNormalizedEvent
	err   error
}

func (f *fakeRuntime) HandleMarket(_ context.Context, evt contracts.MarketNormalizedEvent) error {
	f.calls++
	f.last = evt
	return f.err
}

func TestNewLoopValidation(t *testing.T) {
	r := &fakeRuntime{}
	_, err := NewLoop(nil, r, Config{Subject: "market.normalized.spot.>", Durable: "d1"})
	if !errors.Is(err, ErrBusNil) {
		t.Fatalf("expected ErrBusNil, got %v", err)
	}

	_, err = NewLoop(&natsbus.Client{}, nil, Config{Subject: "market.normalized.spot.>", Durable: "d1"})
	if !errors.Is(err, ErrRuntimeNil) {
		t.Fatalf("expected ErrRuntimeNil, got %v", err)
	}

	_, err = NewLoop(&natsbus.Client{}, r, Config{Subject: "market.normalized.spot.>", Durable: ""})
	if !errors.Is(err, ErrDurableEmpty) {
		t.Fatalf("expected ErrDurableEmpty, got %v", err)
	}
}

func TestHandleMessage(t *testing.T) {
	rt := &fakeRuntime{}
	loop, err := NewLoop(&natsbus.Client{}, rt, Config{
		Subject: "market.normalized.spot.>",
		Durable: "runner-it",
	})
	if err != nil {
		t.Fatalf("NewLoop() error = %v", err)
	}

	err = loop.HandleMessage(context.Background(), natsbus.Message{
		Subject: "market.normalized.spot.binance.BTC-USDT",
		Data: []byte(`{
			"Venue":"binance",
			"Symbol":"BTC-USDT",
			"Sequence":12,
			"BidPX":62000.1,
			"BidSZ":1.2,
			"AskPX":62000.2,
			"AskSZ":0.8,
			"LastPX":62000.15,
			"SourceTSMS":1700000000000,
			"IngestTSMS":1700000000001,
			"EmitTSMS":1700000000002
		}`),
	})
	if err != nil {
		t.Fatalf("HandleMessage() error = %v", err)
	}
	if rt.calls != 1 {
		t.Fatalf("expected runtime call=1, got %d", rt.calls)
	}
	if rt.last.Symbol != "BTC-USDT" {
		t.Fatalf("unexpected runtime symbol: %+v", rt.last)
	}
}

func TestHandleMessageInvalidJSON(t *testing.T) {
	rt := &fakeRuntime{}
	loop, err := NewLoop(&natsbus.Client{}, rt, Config{
		Subject: "market.normalized.spot.>",
		Durable: "runner-it",
	})
	if err != nil {
		t.Fatalf("NewLoop() error = %v", err)
	}

	err = loop.HandleMessage(context.Background(), natsbus.Message{
		Subject: "market.normalized.spot.binance.BTC-USDT",
		Data:    []byte(`{invalid json`),
	})
	if err == nil {
		t.Fatal("expected invalid json error")
	}
}

func TestNewNATSIntentSinkValidation(t *testing.T) {
	_, err := NewNATSIntentSink(nil)
	if !errors.Is(err, ErrIntentBusNil) {
		t.Fatalf("expected ErrIntentBusNil, got %v", err)
	}

	sink, err := NewNATSIntentSink(&natsbus.Client{})
	if err != nil {
		t.Fatalf("NewNATSIntentSink() error = %v", err)
	}
	err = sink(context.Background(), contracts.OrderIntent{StrategyID: ""})
	if !errors.Is(err, ErrStrategyEmpty) {
		t.Fatalf("expected ErrStrategyEmpty, got %v", err)
	}
}
