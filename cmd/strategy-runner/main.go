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
	"strconv"
	"syscall"
	"time"

	"quant-system/internal/bus/natsbus"
	"quant-system/internal/obs/logging"
	"quant-system/internal/obs/metrics"
	"quant-system/internal/strategy"
	momentum "quant-system/internal/strategy/momentum"
	"quant-system/internal/strategyrunner"
)

func main() {
	logging.Init()

	natsURL := getenv("NATS_URL", "nats://127.0.0.1:4222")
	runnerAddr := getenv("STRATEGY_RUNNER_ADDR", ":8081")
	durable := getenv("STRATEGY_RUNNER_DURABLE", "strategy-runner")
	subject := getenv("STRATEGY_RUNNER_SUBJECT", "market.normalized.spot.>")
	deliverPolicy := getenv("NATS_DELIVER_POLICY", "all")

	client, err := natsbus.Connect(natsbus.Config{
		URL:  natsURL,
		Name: "strategy-runner",
	})
	if err != nil {
		slog.Error("nats connect failed", "error", err)
		os.Exit(1)
	}
	defer client.Close()

	if err := ensureStreams(context.Background(), client); err != nil {
		slog.Error("ensure stream failed", "error", err)
		os.Exit(1)
	}

	intentSink, err := strategyrunner.NewNATSIntentSink(client)
	if err != nil {
		slog.Error("intent sink init failed", "error", err)
		os.Exit(1)
	}
	runtime, err := strategy.NewInMemoryRuntime(intentSink)
	if err != nil {
		slog.Error("runtime init failed", "error", err)
		os.Exit(1)
	}

	// Register momentum strategy if configured.
	if sym := os.Getenv("MOMENTUM_SYMBOL"); sym != "" {
		strat := momentum.New(momentum.Config{
			Symbol:            sym,
			WindowSize:        getenvInt("MOMENTUM_WINDOW_SIZE", 20),
			BreakoutThreshold: getenvFloat("MOMENTUM_BREAKOUT_THRESHOLD", 0.001),
			OrderQty:          getenvFloat("MOMENTUM_ORDER_QTY", 0.01),
			TimeInForce:       getenv("MOMENTUM_TIME_IN_FORCE", "IOC"),
			Cooldown:          time.Duration(getenvInt("MOMENTUM_COOLDOWN_MS", 5000)) * time.Millisecond,
		})
		if err := runtime.Register(strat); err != nil {
			slog.Error("register momentum strategy failed", "error", err)
			os.Exit(1)
		}
		slog.Info("registered momentum strategy", "symbol", sym)
	}

	loop, err := strategyrunner.NewLoop(client, runtime, strategyrunner.Config{
		Subject:       subject,
		Durable:       durable,
		Queue:         "strategy-runner",
		DeliverPolicy: deliverPolicy,
	})
	if err != nil {
		slog.Error("loop init failed", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub, err := loop.Start(ctx)
	if err != nil {
		slog.Error("subscribe failed", "error", err)
		os.Exit(1)
	}
	defer sub.Unsubscribe()

	server := &http.Server{
		Addr:              runnerAddr,
		Handler:           newHTTPHandler(),
		ReadHeaderTimeout: 2 * time.Second,
	}

	go func() {
		slog.Info("strategy-runner listening", "addr", runnerAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server failed", "error", err)
			os.Exit(1)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown failed", "error", err)
		os.Exit(1)
	}
}

func ensureStreams(ctx context.Context, client *natsbus.Client) error {
	if err := client.EnsureStream(ctx, natsbus.StreamConfig{
		Name:     "STREAM_MARKET",
		Subjects: []string{"market.normalized.spot.>"},
	}); err != nil {
		return err
	}
	if err := client.EnsureStream(ctx, natsbus.StreamConfig{
		Name:     "STREAM_TRADING",
		Subjects: []string{"strategy.intent.>"},
	}); err != nil {
		return err
	}
	return nil
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		if n, err := strconv.Atoi(value); err == nil {
			return n
		}
	}
	return fallback
}

func getenvFloat(key string, fallback float64) float64 {
	if value := os.Getenv(key); value != "" {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}
	return fallback
}

func newHTTPHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
		})
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, metrics.ExposePrometheus())
	})
	return mux
}
