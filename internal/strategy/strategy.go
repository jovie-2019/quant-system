package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"quant-system/internal/book"
	"quant-system/pkg/contracts"
)

var (
	ErrStrategyExists = errors.New("strategy: duplicate strategy id")
	ErrIntentSinkNil  = errors.New("strategy: intent sink is nil")
)

type OrderIntent = contracts.OrderIntent

type Strategy interface {
	ID() string
	OnMarket(evt contracts.MarketNormalizedEvent) []OrderIntent
}

// KlineHandler is an optional interface. Strategies that need K-line data
// should implement this. The runtime calls OnKline when a candle closes.
type KlineHandler interface {
	OnKline(kline contracts.Kline) []OrderIntent
}

// DepthHandler is an optional interface. Strategies that need order book
// depth should implement this. The runtime calls OnDepth on each update.
type DepthHandler interface {
	OnDepth(depth contracts.DepthSnapshot) []OrderIntent
}

// ParamReloader is an optional interface implemented by strategies that
// support hot-swapping their parameter set without rebuilding the
// instance. Strategies that implement this preserve their internal state
// (indicators, open positions, rolling windows) across the swap when
// possible; fields that cannot be hot-swapped should be rejected with
// a descriptive error so the caller can fall back to full replacement.
type ParamReloader interface {
	// ApplyParams validates raw and, on success, atomically swaps the
	// strategy's parameter set. Implementations should be safe to call
	// concurrently with OnMarket/OnKline/OnDepth.
	ApplyParams(raw json.RawMessage) error
}

type IntentSink func(ctx context.Context, intent OrderIntent) error

type BookReader interface {
	Snapshot(key book.VenueSymbol) (book.BookSnapshot, bool)
}

type Runtime interface {
	Register(s Strategy) error
	HandleMarket(ctx context.Context, evt contracts.MarketNormalizedEvent) error
	HandleKline(ctx context.Context, kline contracts.Kline) error
	HandleDepth(ctx context.Context, depth contracts.DepthSnapshot) error
	SetBookReader(reader BookReader)
	GetBookSnapshot(key book.VenueSymbol) (book.BookSnapshot, bool)
}

type InMemoryRuntime struct {
	mu         sync.RWMutex
	strategies map[string]Strategy
	intentSink IntentSink
	bookReader BookReader
}

func NewInMemoryRuntime(intentSink IntentSink) (*InMemoryRuntime, error) {
	if intentSink == nil {
		return nil, ErrIntentSinkNil
	}
	return &InMemoryRuntime{
		strategies: make(map[string]Strategy),
		intentSink: intentSink,
	}, nil
}

func (r *InMemoryRuntime) Register(s Strategy) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := s.ID()
	if _, exists := r.strategies[id]; exists {
		return fmt.Errorf("%w: %s", ErrStrategyExists, id)
	}
	r.strategies[id] = s
	return nil
}

// Unregister removes a strategy by ID. It is a no-op if the ID is not found.
func (r *InMemoryRuntime) Unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.strategies, id)
}

// Replace atomically removes the old strategy and registers the new one.
func (r *InMemoryRuntime) Replace(old Strategy, newS Strategy) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.strategies, old.ID())
	newID := newS.ID()
	if _, exists := r.strategies[newID]; exists {
		return fmt.Errorf("%w: %s", ErrStrategyExists, newID)
	}
	r.strategies[newID] = newS
	return nil
}

func (r *InMemoryRuntime) HandleMarket(ctx context.Context, evt contracts.MarketNormalizedEvent) error {
	r.mu.RLock()
	strategies := make([]Strategy, 0, len(r.strategies))
	for _, s := range r.strategies {
		strategies = append(strategies, s)
	}
	r.mu.RUnlock()

	for _, s := range strategies {
		intents := s.OnMarket(evt)
		for _, intent := range intents {
			intent.StrategyID = s.ID()
			if err := r.intentSink(ctx, intent); err != nil {
				return err
			}
		}
	}
	return nil
}

// HandleKline dispatches a kline event to strategies that implement KlineHandler.
func (r *InMemoryRuntime) HandleKline(ctx context.Context, kline contracts.Kline) error {
	r.mu.RLock()
	strategies := make([]Strategy, 0, len(r.strategies))
	for _, s := range r.strategies {
		strategies = append(strategies, s)
	}
	r.mu.RUnlock()

	for _, s := range strategies {
		if kh, ok := s.(KlineHandler); ok {
			intents := kh.OnKline(kline)
			for _, intent := range intents {
				intent.StrategyID = s.ID()
				if err := r.intentSink(ctx, intent); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// HandleDepth dispatches a depth event to strategies that implement DepthHandler.
func (r *InMemoryRuntime) HandleDepth(ctx context.Context, depth contracts.DepthSnapshot) error {
	r.mu.RLock()
	strategies := make([]Strategy, 0, len(r.strategies))
	for _, s := range r.strategies {
		strategies = append(strategies, s)
	}
	r.mu.RUnlock()

	for _, s := range strategies {
		if dh, ok := s.(DepthHandler); ok {
			intents := dh.OnDepth(depth)
			for _, intent := range intents {
				intent.StrategyID = s.ID()
				if err := r.intentSink(ctx, intent); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (r *InMemoryRuntime) SetBookReader(reader BookReader) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bookReader = reader
}

func (r *InMemoryRuntime) GetBookSnapshot(key book.VenueSymbol) (book.BookSnapshot, bool) {
	r.mu.RLock()
	reader := r.bookReader
	r.mu.RUnlock()
	if reader == nil {
		return book.BookSnapshot{}, false
	}
	return reader.Snapshot(key)
}
