package normalizer

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"quant-system/internal/adapter"
	"quant-system/pkg/contracts"
)

var (
	ErrInvalidPayload = errors.New("normalizer: invalid payload")
	ErrMissingField   = errors.New("normalizer: missing required field")
)

type MarketNormalizedEvent = contracts.MarketNormalizedEvent
type OrderLifecycleEvent = contracts.OrderLifecycleEvent

type Normalizer interface {
	NormalizeMarket(raw adapter.RawMarketEvent) (MarketNormalizedEvent, error)
	NormalizeExec(raw adapter.RawExecEvent) (OrderLifecycleEvent, error)
}

type JSONNormalizer struct{}

func NewJSONNormalizer() *JSONNormalizer {
	return &JSONNormalizer{}
}

func (n *JSONNormalizer) NormalizeMarket(raw adapter.RawMarketEvent) (MarketNormalizedEvent, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw.Payload, &payload); err != nil {
		return MarketNormalizedEvent{}, fmt.Errorf("%w: %v", ErrInvalidPayload, err)
	}

	bidPX, err := requiredFloat(payload, "bid_px")
	if err != nil {
		return MarketNormalizedEvent{}, err
	}
	bidSZ, err := requiredFloat(payload, "bid_sz")
	if err != nil {
		return MarketNormalizedEvent{}, err
	}
	askPX, err := requiredFloat(payload, "ask_px")
	if err != nil {
		return MarketNormalizedEvent{}, err
	}
	askSZ, err := requiredFloat(payload, "ask_sz")
	if err != nil {
		return MarketNormalizedEvent{}, err
	}
	lastPX, err := requiredFloat(payload, "last_px")
	if err != nil {
		return MarketNormalizedEvent{}, err
	}

	sourceTS := raw.SourceTSMS
	if sourceTS == 0 {
		sourceTS = optionalInt64(payload, "ts")
	}

	seq := raw.Sequence
	if seq == 0 {
		seq = optionalInt64(payload, "seq")
	}

	nowMS := nowUnixMS()
	return MarketNormalizedEvent{
		Venue:      raw.Venue,
		Symbol:     raw.Symbol,
		Sequence:   seq,
		BidPX:      bidPX,
		BidSZ:      bidSZ,
		AskPX:      askPX,
		AskSZ:      askSZ,
		LastPX:     lastPX,
		SourceTSMS: sourceTS,
		IngestTSMS: nowMS,
		EmitTSMS:   nowMS,
	}, nil
}

func (n *JSONNormalizer) NormalizeExec(raw adapter.RawExecEvent) (OrderLifecycleEvent, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw.Payload, &payload); err != nil {
		return OrderLifecycleEvent{}, fmt.Errorf("%w: %v", ErrInvalidPayload, err)
	}

	clientOrderID, err := requiredString(payload, "client_order_id")
	if err != nil {
		return OrderLifecycleEvent{}, err
	}
	venueOrderID, err := requiredString(payload, "venue_order_id")
	if err != nil {
		return OrderLifecycleEvent{}, err
	}
	state, err := requiredString(payload, "state")
	if err != nil {
		return OrderLifecycleEvent{}, err
	}
	filledQty, err := requiredFloat(payload, "filled_qty")
	if err != nil {
		return OrderLifecycleEvent{}, err
	}
	avgPrice, err := requiredFloat(payload, "avg_price")
	if err != nil {
		return OrderLifecycleEvent{}, err
	}

	sourceTS := raw.SourceTSMS
	if sourceTS == 0 {
		sourceTS = optionalInt64(payload, "ts")
	}

	nowMS := nowUnixMS()
	return OrderLifecycleEvent{
		Venue:         raw.Venue,
		Symbol:        raw.Symbol,
		ClientOrderID: clientOrderID,
		VenueOrderID:  venueOrderID,
		State:         state,
		FilledQty:     filledQty,
		AvgPrice:      avgPrice,
		SourceTSMS:    sourceTS,
		IngestTSMS:    nowMS,
		EmitTSMS:      nowMS,
	}, nil
}

func requiredString(payload map[string]any, field string) (string, error) {
	value, ok := payload[field]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrMissingField, field)
	}
	s, ok := value.(string)
	if !ok || s == "" {
		return "", fmt.Errorf("%w: %s", ErrMissingField, field)
	}
	return s, nil
}

func requiredFloat(payload map[string]any, field string) (float64, error) {
	value, ok := payload[field]
	if !ok {
		return 0, fmt.Errorf("%w: %s", ErrMissingField, field)
	}
	f, err := toFloat64(value)
	if err != nil {
		return 0, fmt.Errorf("%w: %s", ErrMissingField, field)
	}
	return f, nil
}

func optionalInt64(payload map[string]any, field string) int64 {
	value, ok := payload[field]
	if !ok {
		return 0
	}
	switch v := value.(type) {
	case float64:
		return int64(v)
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			return parsed
		}
	case json.Number:
		parsed, err := v.Int64()
		if err == nil {
			return parsed
		}
	}
	return 0
}

func toFloat64(value any) (float64, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case string:
		return strconv.ParseFloat(v, 64)
	case json.Number:
		return v.Float64()
	default:
		return 0, ErrInvalidPayload
	}
}

func nowUnixMS() int64 {
	return time.Now().UnixMilli()
}
