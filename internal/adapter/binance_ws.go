package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type BinanceSpotWSConfig struct {
	Endpoint     string
	ReconnectMin time.Duration
	ReconnectMax time.Duration
	PingInterval time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

type BinanceSpotWSMarketStream struct {
	endpoint  string
	dialer    *websocket.Dialer
	wsCfg     wsRuntimeConfig
	now       func() time.Time
	reconnect atomic.Uint64
}

func NewBinanceSpotWSMarketStream(cfg BinanceSpotWSConfig) (*BinanceSpotWSMarketStream, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, ErrWSConfigInvalid
	}
	return &BinanceSpotWSMarketStream{
		endpoint: strings.TrimSpace(cfg.Endpoint),
		dialer: newWSDialer(),
		wsCfg: withDefaults(wsRuntimeConfig{
			ReconnectMin: cfg.ReconnectMin,
			ReconnectMax: cfg.ReconnectMax,
			PingInterval: cfg.PingInterval,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
		}),
		now: time.Now,
	}, nil
}

func (s *BinanceSpotWSMarketStream) Subscribe(ctx context.Context, symbols []string) (<-chan RawMarketEvent, error) {
	if len(symbols) == 0 {
		return nil, fmt.Errorf("%w: empty symbols", ErrWSConfigInvalid)
	}

	out := make(chan RawMarketEvent, 1024)
	go s.run(ctx, symbols, out)
	return out, nil
}

func (s *BinanceSpotWSMarketStream) ReconnectCount() uint64 {
	return s.reconnect.Load()
}

func (s *BinanceSpotWSMarketStream) run(ctx context.Context, symbols []string, out chan<- RawMarketEvent) {
	defer close(out)

	backoff := s.wsCfg.ReconnectMin
	for {
		if ctx.Err() != nil {
			return
		}

		conn, _, err := s.dialer.DialContext(ctx, s.endpoint, nil)
		if err != nil {
			if !sleepOrDone(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff, s.wsCfg.ReconnectMax)
			continue
		}
		s.reconnect.Add(1)
		backoff = s.wsCfg.ReconnectMin

		if err := s.subscribe(conn, symbols); err != nil {
			_ = conn.Close()
			if !sleepOrDone(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff, s.wsCfg.ReconnectMax)
			continue
		}

		err = s.readLoop(ctx, conn, out)
		_ = conn.Close()
		if err == nil || ctx.Err() != nil {
			return
		}
		if !sleepOrDone(ctx, backoff) {
			return
		}
		backoff = nextBackoff(backoff, s.wsCfg.ReconnectMax)
	}
}

func (s *BinanceSpotWSMarketStream) subscribe(conn *websocket.Conn, symbols []string) error {
	params := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		params = append(params, strings.ToLower(strings.ReplaceAll(strings.TrimSpace(symbol), "-", ""))+"@bookTicker")
	}

	msg := map[string]any{
		"method": "SUBSCRIBE",
		"params": params,
		"id":     1,
	}
	_ = conn.SetWriteDeadline(s.now().Add(s.wsCfg.WriteTimeout))
	return conn.WriteJSON(msg)
}

func (s *BinanceSpotWSMarketStream) readLoop(ctx context.Context, conn *websocket.Conn, out chan<- RawMarketEvent) error {
	_ = conn.SetReadDeadline(s.now().Add(s.wsCfg.ReadTimeout))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(s.now().Add(s.wsCfg.ReadTimeout))
	})

	done := make(chan struct{})
	go startPingLoop(ctx, conn, s.wsCfg.PingInterval, s.wsCfg.WriteTimeout, done, s.now)
	defer close(done)

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		_ = conn.SetReadDeadline(s.now().Add(s.wsCfg.ReadTimeout))

		event, ok, err := parseBinanceBookTicker(msg)
		if err != nil || !ok {
			continue
		}

		select {
		case <-ctx.Done():
			return nil
		case out <- event:
		}
	}
}

func parseBinanceBookTicker(msg []byte) (RawMarketEvent, bool, error) {
	var payload struct {
		EventType string `json:"e"`
		EventTime int64  `json:"E"`
		Symbol    string `json:"s"`
		UpdateID  int64  `json:"u"`
		BidPX     string `json:"b"`
		BidSZ     string `json:"B"`
		AskPX     string `json:"a"`
		AskSZ     string `json:"A"`
	}
	if err := json.Unmarshal(msg, &payload); err != nil {
		return RawMarketEvent{}, false, err
	}
	// Individual bookTicker streams don't include "e" field;
	// combined streams do. Accept both formats.
	if payload.EventType != "" && payload.EventType != "bookTicker" {
		return RawMarketEvent{}, false, nil
	}
	if payload.Symbol == "" {
		return RawMarketEvent{}, false, nil
	}

	sourceTS := payload.EventTime
	if sourceTS == 0 {
		sourceTS = time.Now().UnixMilli()
	}

	symbol := toCanonicalFromCompactSymbol(payload.Symbol)
	rawPayload := buildNormalizedPayload(
		payload.BidPX,
		payload.BidSZ,
		payload.AskPX,
		payload.AskSZ,
		payload.BidPX,
		payload.UpdateID,
		sourceTS,
	)

	return RawMarketEvent{
		Venue:      VenueBinance,
		Symbol:     symbol,
		EventType:  payload.EventType,
		Payload:    rawPayload,
		SourceTSMS: sourceTS,
		Sequence:   payload.UpdateID,
	}, true, nil
}
