package strategy

import (
	"context"
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

type IntentSink func(ctx context.Context, intent OrderIntent) error

type BookReader interface {
	Snapshot(key book.VenueSymbol) (book.BookSnapshot, bool)
}

type Runtime interface {
	Register(s Strategy) error
	HandleMarket(ctx context.Context, evt contracts.MarketNormalizedEvent) error
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
