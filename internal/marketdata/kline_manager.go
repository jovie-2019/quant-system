package marketdata

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"quant-system/pkg/contracts"
)

const defaultKlineBufferLen = 200

// KlineFetcher is a function that retrieves historical klines from a venue REST API.
// It is injected into Warmup for testability.
type KlineFetcher func(ctx context.Context, symbol, interval string, limit int) ([]contracts.Kline, error)

// KlineManager is a thread-safe in-memory store for K-line (candlestick) data.
type KlineManager struct {
	mu      sync.RWMutex
	buffers map[string]*klineBuffer // key: "symbol:interval"
	logger  *slog.Logger
}

// klineBuffer is a ring buffer that holds at most maxLen klines.
type klineBuffer struct {
	klines []contracts.Kline
	maxLen int
}

// NewKlineManager creates a new KlineManager.
func NewKlineManager(logger *slog.Logger) *KlineManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &KlineManager{
		buffers: make(map[string]*klineBuffer),
		logger:  logger,
	}
}

func bufferKey(symbol, interval string) string {
	return symbol + ":" + interval
}

// Warmup fetches historical klines via REST for each symbol+interval combination.
// Called once at startup. The fetcher function is injected for testability.
func (m *KlineManager) Warmup(ctx context.Context, symbols []string, intervals []string, limit int, fetcher KlineFetcher) error {
	for _, symbol := range symbols {
		for _, interval := range intervals {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			klines, err := fetcher(ctx, symbol, interval, limit)
			if err != nil {
				return fmt.Errorf("warmup %s %s: %w", symbol, interval, err)
			}

			key := bufferKey(symbol, interval)
			buf := &klineBuffer{maxLen: defaultKlineBufferLen}

			// Only keep the last maxLen klines if fetcher returned more.
			if len(klines) > buf.maxLen {
				klines = klines[len(klines)-buf.maxLen:]
			}
			buf.klines = make([]contracts.Kline, len(klines))
			copy(buf.klines, klines)

			m.mu.Lock()
			m.buffers[key] = buf
			m.mu.Unlock()

			m.logger.Info("kline warmup complete",
				"symbol", symbol,
				"interval", interval,
				"count", len(buf.klines),
			)
		}
	}
	return nil
}

// Update processes a real-time kline update from WebSocket.
// If kline.Closed is true, it appends a new candle; otherwise it updates the last one.
func (m *KlineManager) Update(kline contracts.Kline) {
	key := bufferKey(kline.Symbol, kline.Interval)

	m.mu.Lock()
	defer m.mu.Unlock()

	buf, ok := m.buffers[key]
	if !ok {
		buf = &klineBuffer{maxLen: defaultKlineBufferLen}
		m.buffers[key] = buf
	}

	if kline.Closed {
		// Append new closed candle. Drop oldest if full.
		if len(buf.klines) >= buf.maxLen {
			copy(buf.klines, buf.klines[1:])
			buf.klines[len(buf.klines)-1] = kline
		} else {
			buf.klines = append(buf.klines, kline)
		}
	} else {
		// Update the last (open) candle in-place.
		if len(buf.klines) == 0 {
			buf.klines = append(buf.klines, kline)
		} else {
			buf.klines[len(buf.klines)-1] = kline
		}
	}
}

// Get returns the last N klines for a symbol+interval.
// If fewer than limit klines exist, all available klines are returned.
func (m *KlineManager) Get(symbol, interval string, limit int) []contracts.Kline {
	key := bufferKey(symbol, interval)

	m.mu.RLock()
	defer m.mu.RUnlock()

	buf, ok := m.buffers[key]
	if !ok || len(buf.klines) == 0 {
		return nil
	}

	n := len(buf.klines)
	if limit > 0 && limit < n {
		n = limit
	}

	result := make([]contracts.Kline, n)
	copy(result, buf.klines[len(buf.klines)-n:])
	return result
}

// Latest returns the most recent kline for a symbol+interval.
func (m *KlineManager) Latest(symbol, interval string) (contracts.Kline, bool) {
	key := bufferKey(symbol, interval)

	m.mu.RLock()
	defer m.mu.RUnlock()

	buf, ok := m.buffers[key]
	if !ok || len(buf.klines) == 0 {
		return contracts.Kline{}, false
	}
	return buf.klines[len(buf.klines)-1], true
}

// Closes returns just the close prices as a float64 slice (for indicators).
func (m *KlineManager) Closes(symbol, interval string, limit int) []float64 {
	klines := m.Get(symbol, interval, limit)
	if len(klines) == 0 {
		return nil
	}
	closes := make([]float64, len(klines))
	for i, k := range klines {
		closes[i] = k.Close
	}
	return closes
}
