package orderfsm

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"quant-system/pkg/contracts"
)

var (
	ErrInvalidEvent      = errors.New("orderfsm: invalid event")
	ErrIllegalTransition = errors.New("orderfsm: illegal state transition")
)

type State = contracts.OrderState

const (
	StateNew      State = contracts.OrderStateNew
	StateAck      State = contracts.OrderStateAck
	StatePartial  State = contracts.OrderStatePartial
	StateFilled   State = contracts.OrderStateFilled
	StateCanceled State = contracts.OrderStateCanceled
	StateRejected State = contracts.OrderStateRejected
)

type Event = contracts.OrderEvent
type Order = contracts.Order

type OrderStateMachine interface {
	Apply(event Event) (Order, error)
	Get(clientOrderID string) (Order, bool)
}

type InMemoryStateMachine struct {
	mu     sync.RWMutex
	orders map[string]Order
	logger *slog.Logger
}

func NewInMemoryStateMachine(logger ...*slog.Logger) *InMemoryStateMachine {
	var l *slog.Logger
	if len(logger) > 0 && logger[0] != nil {
		l = logger[0]
	} else {
		l = slog.Default()
	}
	return &InMemoryStateMachine{
		orders: make(map[string]Order),
		logger: l,
	}
}

func (m *InMemoryStateMachine) Apply(event Event) (Order, error) {
	if strings.TrimSpace(event.ClientOrderID) == "" {
		return Order{}, fmt.Errorf("%w: client_order_id", ErrInvalidEvent)
	}
	if !isValidState(event.State) {
		return Order{}, fmt.Errorf("%w: state=%s", ErrInvalidEvent, event.State)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.orders[event.ClientOrderID]
	if !ok {
		order := newOrderFromEvent(event)
		m.orders[event.ClientOrderID] = order
		m.logger.Info("order state transition",
			"client_order_id", event.ClientOrderID,
			"from", "(new)",
			"to", event.State,
		)
		return order, nil
	}

	if isIdempotent(existing, event) {
		return existing, nil
	}

	if !canTransition(existing.State, event.State) {
		return Order{}, fmt.Errorf("%w: %s -> %s", ErrIllegalTransition, existing.State, event.State)
	}

	fromState := existing.State
	updated := existing
	updated.State = event.State
	updated.Symbol = firstNonEmpty(event.Symbol, existing.Symbol)
	updated.VenueOrderID = firstNonEmpty(event.VenueOrderID, existing.VenueOrderID)
	updated.FilledQty = event.FilledQty
	updated.AvgPrice = event.AvgPrice
	updated.StateVersion++
	updated.UpdatedMS = time.Now().UnixMilli()

	m.orders[event.ClientOrderID] = updated

	m.logger.Info("order state transition",
		"client_order_id", event.ClientOrderID,
		"from", fromState,
		"to", event.State,
	)
	return updated, nil
}

func (m *InMemoryStateMachine) Get(clientOrderID string) (Order, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	order, ok := m.orders[clientOrderID]
	return order, ok
}

func newOrderFromEvent(event Event) Order {
	return Order{
		ClientOrderID: event.ClientOrderID,
		VenueOrderID:  event.VenueOrderID,
		Symbol:        event.Symbol,
		State:         event.State,
		FilledQty:     event.FilledQty,
		AvgPrice:      event.AvgPrice,
		StateVersion:  1,
		UpdatedMS:     time.Now().UnixMilli(),
	}
}

func isIdempotent(existing Order, event Event) bool {
	return existing.State == event.State &&
		existing.FilledQty == event.FilledQty &&
		existing.AvgPrice == event.AvgPrice &&
		(firstNonEmpty(event.VenueOrderID, existing.VenueOrderID) == existing.VenueOrderID)
}

func isValidState(state State) bool {
	switch state {
	case StateNew, StateAck, StatePartial, StateFilled, StateCanceled, StateRejected:
		return true
	default:
		return false
	}
}

func canTransition(from, to State) bool {
	if from == to {
		return true
	}
	switch from {
	case StateNew:
		return to == StateAck || to == StatePartial || to == StateFilled || to == StateCanceled || to == StateRejected
	case StateAck:
		return to == StatePartial || to == StateFilled || to == StateCanceled || to == StateRejected
	case StatePartial:
		return to == StatePartial || to == StateFilled || to == StateCanceled
	case StateFilled, StateCanceled, StateRejected:
		return false
	default:
		return false
	}
}

func firstNonEmpty(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
