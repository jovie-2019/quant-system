package pipeline

import (
	"context"

	"quant-system/internal/bus/natsbus"
	"quant-system/pkg/contracts"
)

// natsSink implements core.EventSink by forwarding lifecycle events to NATS.
// It is the live-trading sink; the backtest engine uses core.MemorySink.
type natsSink struct {
	bus *natsbus.Client
}

// newNATSSink returns a sink that publishes to the given NATS client. Passing
// nil returns a sink whose methods are no-ops, which simplifies tests that
// exercise the pipeline without a live bus.
func newNATSSink(bus *natsbus.Client) *natsSink { return &natsSink{bus: bus} }

func (s *natsSink) PublishRiskDecision(ctx context.Context, accountID string, d contracts.RiskDecision) error {
	if s.bus == nil {
		return nil
	}
	return natsbus.PublishRiskDecision(ctx, s.bus, accountID, d, nil)
}

func (s *natsSink) PublishOrderLifecycle(ctx context.Context, accountID string, evt contracts.OrderLifecycleEvent) error {
	if s.bus == nil {
		return nil
	}
	return natsbus.PublishOrderLifecycle(ctx, s.bus, accountID, evt, nil)
}

func (s *natsSink) PublishTradeFill(ctx context.Context, accountID string, evt contracts.TradeFillEvent) error {
	if s.bus == nil {
		return nil
	}
	return natsbus.PublishTradeFill(ctx, s.bus, accountID, evt, nil)
}
