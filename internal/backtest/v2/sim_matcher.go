// Package v2 provides an event-driven backtest engine that reuses
// internal/core so live trading and backtest share the same orchestration.
//
// SimMatcher implements adapter.TradeGateway as a virtual exchange:
// every PlaceOrder is synchronously matched against the most recent market
// snapshot for the symbol, with configurable slippage and taker fees. The
// backtest engine calls TakeFill immediately after the ack to retrieve the
// synthesised fill and feed it back to core.Engine.ApplyFill.
package v2

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"quant-system/internal/adapter"
	"quant-system/pkg/contracts"
)

// ErrNoMarket is returned when an order is placed for a symbol before any
// market snapshot has been recorded.
var ErrNoMarket = errors.New("sim_matcher: no market state for symbol")

// ErrUnsupportedSide is returned when the order side is neither buy nor sell.
var ErrUnsupportedSide = errors.New("sim_matcher: unsupported side")

// SimMatcherConfig controls simulated fill behaviour. All rates are expressed
// in basis points (1 bps = 0.01%).
type SimMatcherConfig struct {
	// SlippageBps is the adverse price movement applied to every fill. A buy
	// at the best ask of 20,000 with SlippageBps=2 fills at 20,004.
	SlippageBps float64
	// TakerFeeBps is the fee applied to every fill, stored as an absolute
	// currency amount in TradeFillEvent.Fee.
	TakerFeeBps float64
}

type marketState struct {
	bidPX, askPX, lastPX float64
	tsMS                 int64
}

// SimMatcher is the virtual exchange used by backtests. It is safe for use
// from a single backtest goroutine; concurrent multi-goroutine access is
// guarded by an internal mutex.
type SimMatcher struct {
	cfg   SimMatcherConfig
	mu    sync.Mutex
	state map[string]marketState // symbol → snapshot
	pend  map[string]FillTicket  // clientOrderID → pending fill awaiting TakeFill
	seq   atomic.Uint64
}

// FillTicket captures a synthesised fill produced at PlaceOrder time. The
// backtest engine retrieves it with TakeFill and forwards it to core.Engine.
type FillTicket struct {
	ClientOrderID string
	Symbol        string
	Side          string
	FillQty       float64
	FillPrice     float64
	Fee           float64
	TSMS          int64
}

// NewSimMatcher returns a SimMatcher with negative bps clamped to zero.
func NewSimMatcher(cfg SimMatcherConfig) *SimMatcher {
	if cfg.SlippageBps < 0 {
		cfg.SlippageBps = 0
	}
	if cfg.TakerFeeBps < 0 {
		cfg.TakerFeeBps = 0
	}
	return &SimMatcher{
		cfg:   cfg,
		state: make(map[string]marketState),
		pend:  make(map[string]FillTicket),
	}
}

// UpdateMarket records the latest market snapshot for a symbol so subsequent
// orders can be matched using it.
func (m *SimMatcher) UpdateMarket(evt contracts.MarketNormalizedEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state[normSym(evt.Symbol)] = marketState{
		bidPX:  evt.BidPX,
		askPX:  evt.AskPX,
		lastPX: evt.LastPX,
		tsMS:   evt.EmitTSMS,
	}
}

// UpdateMarketFromKline is a convenience for kline-driven strategies: the
// close price becomes lastPX and a synthetic 0.5 bps half-spread provides
// bid/ask. Non-closed klines are ignored.
func (m *SimMatcher) UpdateMarketFromKline(k contracts.Kline) {
	if !k.Closed || k.Close <= 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	half := k.Close * 0.00005
	m.state[normSym(k.Symbol)] = marketState{
		bidPX:  k.Close - half,
		askPX:  k.Close + half,
		lastPX: k.Close,
		tsMS:   k.CloseTime,
	}
}

// PlaceOrder matches the order against the latest market state and queues a
// FillTicket for the caller to retrieve via TakeFill. It implements
// adapter.TradeGateway.
func (m *SimMatcher) PlaceOrder(_ context.Context, req adapter.VenueOrderRequest) (adapter.VenueOrderAck, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	st, ok := m.state[normSym(req.Symbol)]
	if !ok || st.lastPX <= 0 {
		return adapter.VenueOrderAck{}, fmt.Errorf("%w: %s", ErrNoMarket, req.Symbol)
	}

	side := strings.ToLower(strings.TrimSpace(req.Side))
	slip := m.cfg.SlippageBps / 10_000.0

	var fillPrice float64
	switch side {
	case "buy":
		ref := st.askPX
		if ref <= 0 {
			ref = st.lastPX
		}
		fillPrice = ref * (1.0 + slip)
	case "sell":
		ref := st.bidPX
		if ref <= 0 {
			ref = st.lastPX
		}
		fillPrice = ref * (1.0 - slip)
	default:
		return adapter.VenueOrderAck{}, fmt.Errorf("%w: %q", ErrUnsupportedSide, req.Side)
	}

	fee := fillPrice * req.Quantity * (m.cfg.TakerFeeBps / 10_000.0)
	venueID := fmt.Sprintf("sim-%d", m.seq.Add(1))

	m.pend[req.ClientOrderID] = FillTicket{
		ClientOrderID: req.ClientOrderID,
		Symbol:        req.Symbol,
		Side:          side,
		FillQty:       req.Quantity,
		FillPrice:     fillPrice,
		Fee:           fee,
		TSMS:          st.tsMS,
	}

	return adapter.VenueOrderAck{
		ClientOrderID: req.ClientOrderID,
		VenueOrderID:  venueID,
		Status:        "ack",
	}, nil
}

// CancelOrder drops any pending fill for the given client order ID. It
// implements adapter.TradeGateway.
func (m *SimMatcher) CancelOrder(_ context.Context, req adapter.VenueCancelRequest) (adapter.VenueCancelAck, error) {
	m.mu.Lock()
	delete(m.pend, req.ClientOrderID)
	m.mu.Unlock()
	return adapter.VenueCancelAck{
		ClientOrderID: req.ClientOrderID,
		VenueOrderID:  req.VenueOrderID,
		Status:        "canceled",
	}, nil
}

// TakeFill retrieves and removes the FillTicket for the given client order ID.
// Returns (zero, false) when no ticket is queued.
func (m *SimMatcher) TakeFill(clientOrderID string) (FillTicket, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.pend[clientOrderID]
	if ok {
		delete(m.pend, clientOrderID)
	}
	return t, ok
}

// ToTradeFillEvent converts a FillTicket into the contract event consumed by
// core.Engine.ApplyFill.
func (t FillTicket) ToTradeFillEvent(accountID string) contracts.TradeFillEvent {
	return contracts.TradeFillEvent{
		TradeID:    "sim-fill-" + t.ClientOrderID,
		AccountID:  accountID,
		Symbol:     t.Symbol,
		Side:       t.Side,
		FillQty:    t.FillQty,
		FillPrice:  t.FillPrice,
		Fee:        t.Fee,
		SourceTSMS: t.TSMS,
	}
}

func normSym(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}
