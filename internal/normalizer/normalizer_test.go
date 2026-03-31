package normalizer

import (
	"errors"
	"testing"

	"quant-system/internal/adapter"
)

func TestNormalizeMarketSuccess(t *testing.T) {
	n := NewJSONNormalizer()
	raw := adapter.RawMarketEvent{
		Venue:      adapter.VenueBinance,
		Symbol:     "BTC-USDT",
		SourceTSMS: 1700000000000,
		Sequence:   42,
		Payload: []byte(`{
			"bid_px":"62000.1",
			"bid_sz":"1.2",
			"ask_px":"62000.2",
			"ask_sz":"0.8",
			"last_px":"62000.15"
		}`),
	}

	got, err := n.NormalizeMarket(raw)
	if err != nil {
		t.Fatalf("NormalizeMarket() error = %v", err)
	}
	if got.Venue != adapter.VenueBinance || got.Symbol != "BTC-USDT" {
		t.Fatalf("unexpected venue/symbol: %#v", got)
	}
	if got.Sequence != 42 {
		t.Fatalf("unexpected sequence: %d", got.Sequence)
	}
	if got.BidPX != 62000.1 || got.AskPX != 62000.2 {
		t.Fatalf("unexpected price mapping: %#v", got)
	}
	if got.IngestTSMS == 0 || got.EmitTSMS == 0 {
		t.Fatalf("timestamps not set: %#v", got)
	}
}

func TestNormalizeMarketInvalidJSON(t *testing.T) {
	n := NewJSONNormalizer()
	raw := adapter.RawMarketEvent{
		Venue:   adapter.VenueBinance,
		Symbol:  "BTC-USDT",
		Payload: []byte(`{"bid_px":`),
	}

	_, err := n.NormalizeMarket(raw)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrInvalidPayload) {
		t.Fatalf("expected ErrInvalidPayload, got %v", err)
	}
}

func TestNormalizeExecMissingField(t *testing.T) {
	n := NewJSONNormalizer()
	raw := adapter.RawExecEvent{
		Venue:  adapter.VenueOKX,
		Symbol: "BTC-USDT",
		Payload: []byte(`{
			"client_order_id":"cid-1",
			"state":"filled",
			"filled_qty":"1.0",
			"avg_price":"62000.5"
		}`),
	}

	_, err := n.NormalizeExec(raw)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrMissingField) {
		t.Fatalf("expected ErrMissingField, got %v", err)
	}
}
