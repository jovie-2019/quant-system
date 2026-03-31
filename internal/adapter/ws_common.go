package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

var (
	ErrWSConfigInvalid = errors.New("adapter/ws: invalid config")
)

type wsRuntimeConfig struct {
	ReconnectMin time.Duration
	ReconnectMax time.Duration
	PingInterval time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

func withDefaults(cfg wsRuntimeConfig) wsRuntimeConfig {
	if cfg.ReconnectMin <= 0 {
		cfg.ReconnectMin = 300 * time.Millisecond
	}
	if cfg.ReconnectMax <= 0 {
		cfg.ReconnectMax = 3 * time.Second
	}
	if cfg.ReconnectMax < cfg.ReconnectMin {
		cfg.ReconnectMax = cfg.ReconnectMin
	}
	if cfg.PingInterval <= 0 {
		cfg.PingInterval = 15 * time.Second
	}
	if cfg.ReadTimeout <= 0 {
		cfg.ReadTimeout = 45 * time.Second
	}
	if cfg.WriteTimeout <= 0 {
		cfg.WriteTimeout = 3 * time.Second
	}
	return cfg
}

func nextBackoff(current, max time.Duration) time.Duration {
	n := current * 2
	if n > max {
		return max
	}
	return n
}

func sleepOrDone(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func logReconnect(venue string, attempt int, backoffMS int64) {
	slog.Warn("ws reconnecting",
		"venue", venue,
		"attempt", attempt,
		"backoff_ms", backoffMS,
	)
}

func startPingLoop(
	ctx context.Context,
	conn *websocket.Conn,
	interval time.Duration,
	writeTimeout time.Duration,
	done <-chan struct{},
	now func() time.Time,
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-ticker.C:
			_ = conn.WriteControl(websocket.PingMessage, []byte("ping"), now().Add(writeTimeout))
		}
	}
}

func toCanonicalFromCompactSymbol(symbol string) string {
	s := strings.ToUpper(strings.TrimSpace(symbol))
	if strings.Contains(s, "-") {
		return s
	}
	switch {
	case strings.HasSuffix(s, "USDT"):
		return s[:len(s)-4] + "-USDT"
	case strings.HasSuffix(s, "USDC"):
		return s[:len(s)-4] + "-USDC"
	case strings.HasSuffix(s, "BTC"):
		return s[:len(s)-3] + "-BTC"
	case strings.HasSuffix(s, "ETH"):
		return s[:len(s)-3] + "-ETH"
	default:
		return s
	}
}

func buildNormalizedPayload(bidPX, bidSZ, askPX, askSZ, lastPX string, seq int64, tsMS int64) []byte {
	payload, _ := json.Marshal(map[string]any{
		"bid_px": bidPX,
		"bid_sz": bidSZ,
		"ask_px": askPX,
		"ask_sz": askSZ,
		"last_px": func() string {
			if strings.TrimSpace(lastPX) == "" {
				return bidPX
			}
			return lastPX
		}(),
		"seq": strconv.FormatInt(seq, 10),
		"ts":  strconv.FormatInt(tsMS, 10),
	})
	return payload
}
