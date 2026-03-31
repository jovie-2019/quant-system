package adapter

import (
	"context"
	"errors"

	"quant-system/pkg/contracts"
)

var ErrNotImplemented = errors.New("adapter: not implemented")

type Venue = contracts.Venue

const (
	VenueBinance Venue = contracts.VenueBinance
	VenueOKX     Venue = contracts.VenueOKX
)

type RawMarketEvent = contracts.RawMarketEvent
type RawExecEvent = contracts.RawExecEvent
type VenueOrderRequest = contracts.VenueOrderRequest
type VenueOrderAck = contracts.VenueOrderAck
type VenueCancelRequest = contracts.VenueCancelRequest
type VenueCancelAck = contracts.VenueCancelAck
type VenueOrderQueryRequest = contracts.VenueOrderQueryRequest
type VenueOrderStatus = contracts.VenueOrderStatus

type MarketStream interface {
	Subscribe(ctx context.Context, symbols []string) (<-chan RawMarketEvent, error)
}

type TradeGateway interface {
	PlaceOrder(ctx context.Context, req VenueOrderRequest) (VenueOrderAck, error)
	CancelOrder(ctx context.Context, req VenueCancelRequest) (VenueCancelAck, error)
}

type OrderQueryGateway interface {
	QueryOrder(ctx context.Context, req VenueOrderQueryRequest) (VenueOrderStatus, error)
}

// StubMarketStream keeps compile-time boundaries stable before venue implementations.
type StubMarketStream struct{}

func (s StubMarketStream) Subscribe(_ context.Context, _ []string) (<-chan RawMarketEvent, error) {
	return nil, ErrNotImplemented
}

// StubTradeGateway keeps execution integration points explicit in early iterations.
type StubTradeGateway struct{}

func (s StubTradeGateway) PlaceOrder(_ context.Context, _ VenueOrderRequest) (VenueOrderAck, error) {
	return VenueOrderAck{}, ErrNotImplemented
}

func (s StubTradeGateway) CancelOrder(_ context.Context, _ VenueCancelRequest) (VenueCancelAck, error) {
	return VenueCancelAck{}, ErrNotImplemented
}

func (s StubTradeGateway) QueryOrder(_ context.Context, _ VenueOrderQueryRequest) (VenueOrderStatus, error) {
	return VenueOrderStatus{}, ErrNotImplemented
}
