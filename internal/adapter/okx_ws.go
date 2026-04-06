package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type OKXSpotWSConfig struct {
	Endpoint     string
	ReconnectMin time.Duration
	ReconnectMax time.Duration
	PingInterval time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

type OKXSpotWSMarketStream struct {
	endpoint  string
	dialer    *websocket.Dialer
	wsCfg     wsRuntimeConfig
	now       func() time.Time
	reconnect atomic.Uint64
}

func NewOKXSpotWSMarketStream(cfg OKXSpotWSConfig) (*OKXSpotWSMarketStream, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, ErrWSConfigInvalid
	}
	return &OKXSpotWSMarketStream{
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

func (s *OKXSpotWSMarketStream) Subscribe(ctx context.Context, symbols []string) (<-chan RawMarketEvent, error) {
	if len(symbols) == 0 {
		return nil, fmt.Errorf("%w: empty symbols", ErrWSConfigInvalid)
	}

	out := make(chan RawMarketEvent, 1024)
	go s.run(ctx, symbols, out)
	return out, nil
}

func (s *OKXSpotWSMarketStream) ReconnectCount() uint64 {
	return s.reconnect.Load()
}

func (s *OKXSpotWSMarketStream) run(ctx context.Context, symbols []string, out chan<- RawMarketEvent) {
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

func (s *OKXSpotWSMarketStream) subscribe(conn *websocket.Conn, symbols []string) error {
	args := make([]map[string]string, 0, len(symbols))
	for _, symbol := range symbols {
		args = append(args, map[string]string{
			"channel": "tickers",
			"instId":  canonicalToOKXInstID(symbol),
		})
	}
	msg := map[string]any{
		"op":   "subscribe",
		"args": args,
	}
	_ = conn.SetWriteDeadline(s.now().Add(s.wsCfg.WriteTimeout))
	return conn.WriteJSON(msg)
}

func (s *OKXSpotWSMarketStream) readLoop(ctx context.Context, conn *websocket.Conn, out chan<- RawMarketEvent) error {
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

		events, err := parseOKXTicker(msg)
		if err != nil || len(events) == 0 {
			continue
		}

		for _, event := range events {
			select {
			case <-ctx.Done():
				return nil
			case out <- event:
			}
		}
	}
}

func parseOKXTicker(msg []byte) ([]RawMarketEvent, error) {
	var envelope struct {
		Event string `json:"event"`
		Arg   struct {
			Channel string `json:"channel"`
			InstID  string `json:"instId"`
		} `json:"arg"`
		Data []struct {
			Last  string `json:"last"`
			BidPx string `json:"bidPx"`
			BidSz string `json:"bidSz"`
			AskPx string `json:"askPx"`
			AskSz string `json:"askSz"`
			TS    string `json:"ts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(msg, &envelope); err != nil {
		return nil, err
	}
	if envelope.Event != "" {
		return nil, nil
	}
	if envelope.Arg.Channel != "tickers" {
		return nil, nil
	}
	if len(envelope.Data) == 0 {
		return nil, nil
	}

	events := make([]RawMarketEvent, 0, len(envelope.Data))
	symbol := canonicalToOKXInstID(envelope.Arg.InstID)
	for _, data := range envelope.Data {
		tsMS, _ := strconv.ParseInt(strings.TrimSpace(data.TS), 10, 64)
		if tsMS == 0 {
			tsMS = time.Now().UnixMilli()
		}
		rawPayload := buildNormalizedPayload(
			data.BidPx,
			data.BidSz,
			data.AskPx,
			data.AskSz,
			data.Last,
			0,
			tsMS,
		)
		events = append(events, RawMarketEvent{
			Venue:      VenueOKX,
			Symbol:     symbol,
			EventType:  "tickers",
			Payload:    rawPayload,
			SourceTSMS: tsMS,
			Sequence:   0,
		})
	}
	return events, nil
}
