package core

import (
	"context"
	"sync"

	"quant-system/pkg/contracts"
)

// EventSink publishes lifecycle events produced by the Engine. The live
// implementation forwards to NATS; the backtest implementation buffers events
// in memory so the runner can inspect them after a replay finishes.
type EventSink interface {
	PublishRiskDecision(ctx context.Context, accountID string, decision contracts.RiskDecision) error
	PublishOrderLifecycle(ctx context.Context, accountID string, evt contracts.OrderLifecycleEvent) error
	PublishTradeFill(ctx context.Context, accountID string, evt contracts.TradeFillEvent) error
}

// NopSink discards all events. Useful for tests that only care about the
// ledger/FSM side effects.
type NopSink struct{}

// PublishRiskDecision discards the event.
func (NopSink) PublishRiskDecision(context.Context, string, contracts.RiskDecision) error {
	return nil
}

// PublishOrderLifecycle discards the event.
func (NopSink) PublishOrderLifecycle(context.Context, string, contracts.OrderLifecycleEvent) error {
	return nil
}

// PublishTradeFill discards the event.
func (NopSink) PublishTradeFill(context.Context, string, contracts.TradeFillEvent) error {
	return nil
}

// MemorySink buffers every published event in memory and exposes accessors for
// inspection. It is the sink used by the backtest engine.
type MemorySink struct {
	mu        sync.Mutex
	decisions []contracts.RiskDecision
	lifecycle []contracts.OrderLifecycleEvent
	fills     []contracts.TradeFillEvent
}

// NewMemorySink returns an empty MemorySink.
func NewMemorySink() *MemorySink { return &MemorySink{} }

// PublishRiskDecision appends the decision to the internal buffer.
func (s *MemorySink) PublishRiskDecision(_ context.Context, _ string, d contracts.RiskDecision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.decisions = append(s.decisions, d)
	return nil
}

// PublishOrderLifecycle appends the event to the internal buffer.
func (s *MemorySink) PublishOrderLifecycle(_ context.Context, _ string, evt contracts.OrderLifecycleEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lifecycle = append(s.lifecycle, evt)
	return nil
}

// PublishTradeFill appends the event to the internal buffer.
func (s *MemorySink) PublishTradeFill(_ context.Context, _ string, evt contracts.TradeFillEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fills = append(s.fills, evt)
	return nil
}

// Decisions returns a copy of buffered risk decisions.
func (s *MemorySink) Decisions() []contracts.RiskDecision {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]contracts.RiskDecision(nil), s.decisions...)
}

// Lifecycle returns a copy of buffered order lifecycle events.
func (s *MemorySink) Lifecycle() []contracts.OrderLifecycleEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]contracts.OrderLifecycleEvent(nil), s.lifecycle...)
}

// Fills returns a copy of buffered fill events.
func (s *MemorySink) Fills() []contracts.TradeFillEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]contracts.TradeFillEvent(nil), s.fills...)
}

// Reset clears all buffered events.
func (s *MemorySink) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.decisions = nil
	s.lifecycle = nil
	s.fills = nil
}
