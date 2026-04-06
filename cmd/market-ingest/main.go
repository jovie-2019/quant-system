package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"quant-system/internal/adapter"
	"quant-system/internal/bus/natsbus"
	"quant-system/internal/normalizer"
	"quant-system/internal/obs/logging"
	"quant-system/internal/obs/metrics"
)

func main() {
	logging.Init()

	venue := getenv("INGEST_VENUE", "binance")
	symbolsRaw := getenv("INGEST_SYMBOLS", "BTC-USDT")
	natsURL := getenv("NATS_URL", "nats://127.0.0.1:4222")
	addr := getenv("INGEST_ADDR", ":8082")

	symbols := parseSymbols(symbolsRaw)
	if len(symbols) == 0 {
		slog.Error("no symbols configured")
		os.Exit(1)
	}

	// --- NATS ---
	client, err := natsbus.Connect(natsbus.Config{
		URL:  natsURL,
		Name: "market-ingest",
	})
	if err != nil {
		slog.Error("nats connect failed", "error", err)
		os.Exit(1)
	}
	defer client.Close()

	if err := client.EnsureStream(context.Background(), natsbus.StreamConfig{
		Name:     "STREAM_MARKET",
		Subjects: []string{"market.normalized.spot.>"},
		MaxAge:   24 * time.Hour,
		MaxBytes: 500 * 1024 * 1024,
	}); err != nil {
		slog.Error("ensure stream failed", "error", err)
		os.Exit(1)
	}

	// --- WebSocket market stream ---
	stream, err := createMarketStream(venue)
	if err != nil {
		slog.Error("create market stream failed", "error", err, "venue", venue)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rawCh, err := stream.Subscribe(ctx, symbols)
	if err != nil {
		slog.Error("subscribe failed", "error", err)
		os.Exit(1)
	}

	norm := normalizer.NewJSONNormalizer()

	// --- Ingest loop ---
	go func() {
		var published, dropped uint64
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case raw, ok := <-rawCh:
				if !ok {
					slog.Warn("market stream channel closed, shutting down")
					cancel()
					return
				}
				evt, err := norm.NormalizeMarket(raw)
				if err != nil {
					dropped++
					metrics.ObserveMarketIngest(venue, "normalize_error")
					slog.Debug("normalize failed", "error", err, "venue", raw.Venue, "symbol", raw.Symbol)
					continue
				}

				// Emit market data quality metrics.
				latencyMS := float64(time.Now().UnixMilli() - evt.SourceTSMS)
				metrics.ObserveMarketLatency(venue, evt.Symbol, latencyMS)
				metrics.SetMarketPrice(venue, evt.Symbol, "bid", evt.BidPX)
				metrics.SetMarketPrice(venue, evt.Symbol, "ask", evt.AskPX)
				metrics.SetMarketPrice(venue, evt.Symbol, "last", evt.LastPX)
				metrics.ObserveMarketTickRate(venue, evt.Symbol)

				if err := natsbus.PublishMarketNormalized(ctx, client, evt, nil); err != nil {
					dropped++
					metrics.ObserveMarketIngest(venue, "publish_error")
					slog.Error("publish failed", "error", err, "symbol", evt.Symbol)
					continue
				}
				published++
				metrics.ObserveMarketIngest(venue, "published")
			case <-ticker.C:
				slog.Info("ingest stats", "published", published, "dropped", dropped, "venue", venue)
			}
		}
	}()

	// --- HTTP health + metrics ---
	server := &http.Server{
		Addr:              addr,
		Handler:           newHTTPHandler(),
		ReadHeaderTimeout: 2 * time.Second,
	}
	go func() {
		slog.Info("market-ingest listening", "addr", addr, "venue", venue, "symbols", symbols)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server failed", "error", err)
			os.Exit(1)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	slog.Info("shutting down")
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
}

func createMarketStream(venue string) (adapter.MarketStream, error) {
	switch strings.ToLower(strings.TrimSpace(venue)) {
	case "binance":
		endpoint := getenv("BINANCE_WS_ENDPOINT", "wss://stream.binance.com:9443/ws")
		return adapter.NewBinanceSpotWSMarketStream(adapter.BinanceSpotWSConfig{
			Endpoint: endpoint,
		})
	case "okx":
		endpoint := getenv("OKX_WS_ENDPOINT", "wss://ws.okx.com:8443/ws/v5/public")
		return adapter.NewOKXSpotWSMarketStream(adapter.OKXSpotWSConfig{
			Endpoint: endpoint,
		})
	default:
		return nil, fmt.Errorf("unsupported venue: %s", venue)
	}
}

func parseSymbols(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, s := range parts {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, strings.ToUpper(s))
		}
	}
	return out
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func newHTTPHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, metrics.ExposePrometheus())
	})
	return mux
}
