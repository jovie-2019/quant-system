// Command kline-backfill pulls historical spot klines from Binance via REST
// and writes them into the ClickHouse `quant.klines` table. It is the
// primary tool for seeding a backtest dataset before the regime-detector
// and strategy research pipelines go online.
//
// Usage:
//
//	kline-backfill \
//	  -venue binance \
//	  -symbol BTC-USDT \
//	  -interval 1m \
//	  -start 2026-01-01 \
//	  -end   2026-01-07 \
//	  -clickhouse-addr 127.0.0.1:9000 \
//	  -clickhouse-user quant \
//	  -clickhouse-password quant
//
// Environment variables override flag defaults for every parameter, which
// is convenient for docker-compose operators and cron-style wrapper jobs.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"quant-system/internal/marketstore"
	"quant-system/internal/obs/logging"
)

func main() {
	logging.Init()

	var (
		venue        = flag.String("venue", envOr("BACKFILL_VENUE", "binance"), "exchange venue (binance)")
		symbol       = flag.String("symbol", envOr("BACKFILL_SYMBOL", "BTC-USDT"), "symbol in dashed form (BTC-USDT)")
		interval     = flag.String("interval", envOr("BACKFILL_INTERVAL", "1m"), "kline interval (1m/5m/15m/1h/4h/1d)")
		startStr     = flag.String("start", envOr("BACKFILL_START", ""), "window start, e.g. 2026-01-01 or 2026-01-01T00:00:00Z")
		endStr       = flag.String("end", envOr("BACKFILL_END", ""), "window end, exclusive upper bound")
		chAddr       = flag.String("clickhouse-addr", envOr("CLICKHOUSE_ADDR", "127.0.0.1:9000"), "ClickHouse native host:port")
		chDB         = flag.String("clickhouse-db", envOr("CLICKHOUSE_DB", "quant"), "ClickHouse database")
		chUser       = flag.String("clickhouse-user", envOr("CLICKHOUSE_USER", "quant"), "ClickHouse username")
		chPass       = flag.String("clickhouse-password", envOr("CLICKHOUSE_PASSWORD", "quant"), "ClickHouse password")
		binanceBase  = flag.String("binance-base-url", envOr("BINANCE_REST_BASE", "https://api.binance.com"), "Binance REST base URL")
		pacing       = flag.Duration("pacing", envDuration("BACKFILL_PACING", 200*time.Millisecond), "delay between successive REST calls")
		flushEvery   = flag.Int("flush-every", envInt("BACKFILL_FLUSH_EVERY", 10_000), "insert to ClickHouse in batches of this size")
		dryRun       = flag.Bool("dry-run", envBool("BACKFILL_DRY_RUN", false), "do not write to ClickHouse; just report counts")
	)
	flag.Parse()

	startMS, err := parseTimeMS(*startStr)
	if err != nil {
		fatal("invalid -start: %v", err)
	}
	endMS, err := parseTimeMS(*endStr)
	if err != nil {
		fatal("invalid -end: %v", err)
	}
	if startMS == 0 || endMS == 0 {
		fatal("both -start and -end are required (e.g. 2026-01-01)")
	}
	if startMS >= endMS {
		fatal("start (%d) must be strictly before end (%d)", startMS, endMS)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		slog.Info("kline-backfill: signal received, cancelling")
		cancel()
	}()

	// --- Fetcher ---
	if !strings.EqualFold(*venue, "binance") {
		fatal("only -venue=binance is supported in this build; okx support is a follow-up")
	}
	fetcher := marketstore.NewBinanceFetcher(marketstore.BinanceFetcherConfig{
		BaseURL:    *binanceBase,
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
	})

	// --- Store ---
	var store marketstore.KlineStore
	if *dryRun {
		store = marketstore.NewMemoryStore()
		slog.Info("kline-backfill: dry-run mode, using in-memory store")
	} else {
		chStore, err := marketstore.NewClickHouseStore(ctx, marketstore.ClickHouseConfig{
			Addrs:    []string{*chAddr},
			Database: *chDB,
			Username: *chUser,
			Password: *chPass,
		})
		if err != nil {
			fatal("clickhouse: %v", err)
		}
		defer chStore.Close()
		store = chStore
	}

	slog.Info("kline-backfill: starting",
		"venue", *venue,
		"symbol", *symbol,
		"interval", *interval,
		"start", time.UnixMilli(startMS).UTC().Format(time.RFC3339),
		"end", time.UnixMilli(endMS).UTC().Format(time.RFC3339),
		"pacing_ms", pacing.Milliseconds(),
		"flush_every", *flushEvery,
		"dry_run", *dryRun,
	)

	total, err := runBackfill(ctx, fetcher, store, *symbol, *interval, startMS, endMS, *pacing, *flushEvery)
	if err != nil {
		slog.Error("kline-backfill: failed", "error", err, "klines_written", total)
		os.Exit(1)
	}
	slog.Info("kline-backfill: done", "klines_written", total)
}

// runBackfill fetches in manageable chunks (flushEvery) and writes each
// chunk to the store before fetching the next. This bounds memory use on
// multi-year backfills and gives callers partial progress on crash.
func runBackfill(
	ctx context.Context,
	fetcher *marketstore.BinanceFetcher,
	store marketstore.KlineStore,
	symbol, interval string,
	startMS, endMS int64,
	pacing time.Duration,
	flushEvery int,
) (int, error) {
	klines, err := fetcher.FetchRange(ctx, symbol, interval, startMS, endMS, pacing)
	if err != nil {
		return 0, fmt.Errorf("fetch: %w", err)
	}
	total := 0
	for i := 0; i < len(klines); i += flushEvery {
		j := i + flushEvery
		if j > len(klines) {
			j = len(klines)
		}
		if err := store.Upsert(ctx, klines[i:j]); err != nil {
			return total, fmt.Errorf("upsert %d..%d: %w", i, j, err)
		}
		total += j - i
		slog.Info("kline-backfill: batch flushed", "written", total, "of", len(klines))
	}
	return total, nil
}

// parseTimeMS accepts YYYY-MM-DD or RFC3339. Empty returns zero.
func parseTimeMS(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	formats := []string{time.RFC3339, "2006-01-02", "2006-01-02 15:04:05"}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t.UnixMilli(), nil
		}
	}
	return 0, fmt.Errorf("unrecognised time format %q (try 2026-01-01 or RFC3339)", s)
}

func envOr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := time.ParseDuration("0"); err == nil {
			_ = n
		}
		// Simplest parse via fmt.Sscanf; invalid values fall back silently.
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func fatal(format string, a ...any) {
	slog.Error(fmt.Sprintf(format, a...))
	os.Exit(1)
}
