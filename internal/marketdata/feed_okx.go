package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"quant-system/pkg/contracts"
)

// OKXFeedConfig holds configuration for the OKX market data feed.
type OKXFeedConfig struct {
	WSEndpoint  string   // default wss://ws.okx.com:8443/ws/v5/public
	RESTBaseURL string   // default https://www.okx.com
	Symbols     []string // canonical format, e.g. "BTC-USDT"
	Intervals   []string // default ["1m","5m","15m"]
}

// OKXFeed streams kline and depth data from OKX via WebSocket and REST.
type OKXFeed struct {
	cfg    OKXFeedConfig
	dialer *websocket.Dialer
	client *http.Client
	logger *slog.Logger

	onKline func(contracts.Kline)
	onDepth func(contracts.DepthSnapshot)
}

// NewOKXFeed creates a new OKXFeed with sensible defaults.
func NewOKXFeed(cfg OKXFeedConfig, onKline func(contracts.Kline), onDepth func(contracts.DepthSnapshot), logger *slog.Logger) *OKXFeed {
	if cfg.WSEndpoint == "" {
		cfg.WSEndpoint = "wss://ws.okx.com:8443/ws/v5/public"
	}
	if cfg.RESTBaseURL == "" {
		cfg.RESTBaseURL = "https://www.okx.com"
	}
	if len(cfg.Intervals) == 0 {
		cfg.Intervals = []string{"1m", "5m", "15m"}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &OKXFeed{
		cfg:     cfg,
		dialer:  newDialer(),
		client:  newClient(8 * time.Second),
		logger:  logger,
		onKline: onKline,
		onDepth: onDepth,
	}
}

// FetchHistoricalKlines retrieves historical klines from OKX REST API for warmup.
func (f *OKXFeed) FetchHistoricalKlines(ctx context.Context, symbol, interval string, limit int) ([]contracts.Kline, error) {
	if limit <= 0 {
		limit = 200
	}
	instID := strings.TrimSpace(symbol)
	bar := okxInterval(interval)
	url := fmt.Sprintf("%s/api/v5/market/candles?instId=%s&bar=%s&limit=%d",
		strings.TrimRight(f.cfg.RESTBaseURL, "/"), instID, bar, limit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("okx klines: build request: %w", err)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("okx klines: request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("okx klines: read body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("okx klines: status=%d body=%s", resp.StatusCode, string(body))
	}

	// OKX returns: {"code":"0","data":[["ts","o","h","l","c","vol","volCcy","volCcyQuote","confirm"], ...]}
	var envelope struct {
		Code string           `json:"code"`
		Msg  string           `json:"msg"`
		Data [][]string       `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("okx klines: decode: %w", err)
	}
	if envelope.Code != "0" {
		return nil, fmt.Errorf("okx klines: code=%s msg=%s", envelope.Code, envelope.Msg)
	}

	klines := make([]contracts.Kline, 0, len(envelope.Data))
	for _, row := range envelope.Data {
		if len(row) < 9 {
			continue
		}
		k, err := parseOKXKlineRow(row, instID, interval)
		if err != nil {
			f.logger.Warn("okx klines: skip row", "error", err)
			continue
		}
		klines = append(klines, k)
	}

	// OKX returns newest first; reverse to chronological order.
	for i, j := 0, len(klines)-1; i < j; i, j = i+1, j-1 {
		klines[i], klines[j] = klines[j], klines[i]
	}
	return klines, nil
}

// parseOKXKlineRow parses a single kline row from OKX REST response.
func parseOKXKlineRow(row []string, symbol, interval string) (contracts.Kline, error) {
	ts, err := strconv.ParseInt(row[0], 10, 64)
	if err != nil {
		return contracts.Kline{}, fmt.Errorf("parse ts: %w", err)
	}
	confirm := row[8] == "1"
	return contracts.Kline{
		Venue:     contracts.VenueOKX,
		Symbol:    symbol,
		Interval:  interval,
		OpenTime:  ts,
		CloseTime: 0, // OKX does not provide close time in candle data
		Open:      parseFloat(row[1]),
		High:      parseFloat(row[2]),
		Low:       parseFloat(row[3]),
		Close:     parseFloat(row[4]),
		Volume:    parseFloat(row[5]),
		Closed:    confirm,
	}, nil
}

// StartKlineStream connects to OKX kline WebSocket streams and dispatches updates.
func (f *OKXFeed) StartKlineStream(ctx context.Context) error {
	if len(f.cfg.Symbols) == 0 {
		return fmt.Errorf("okx kline stream: no symbols configured")
	}

	args := make([]map[string]string, 0, len(f.cfg.Symbols)*len(f.cfg.Intervals))
	for _, sym := range f.cfg.Symbols {
		instID := strings.TrimSpace(sym)
		for _, iv := range f.cfg.Intervals {
			args = append(args, map[string]string{
				"channel": "candle" + okxInterval(iv),
				"instId":  instID,
			})
		}
	}

	f.runWS(ctx, args, f.handleKlineMessage, "kline")
	return nil
}

// StartDepthStream connects to OKX depth WebSocket stream (books5) and dispatches snapshots.
func (f *OKXFeed) StartDepthStream(ctx context.Context) error {
	if len(f.cfg.Symbols) == 0 {
		return fmt.Errorf("okx depth stream: no symbols configured")
	}

	args := make([]map[string]string, 0, len(f.cfg.Symbols))
	for _, sym := range f.cfg.Symbols {
		args = append(args, map[string]string{
			"channel": "books5",
			"instId":  strings.TrimSpace(sym),
		})
	}

	f.runWS(ctx, args, f.handleDepthMessage, "depth")
	return nil
}

// runWS is the generic reconnect loop for OKX WebSocket streams.
func (f *OKXFeed) runWS(ctx context.Context, args []map[string]string, handler func([]byte), streamType string) {
	const (
		reconnectMin = 300 * time.Millisecond
		reconnectMax = 3 * time.Second
		pingInterval = 15 * time.Second
		readTimeout  = 45 * time.Second
		writeTimeout = 3 * time.Second
	)

	backoff := reconnectMin
	for {
		if ctx.Err() != nil {
			return
		}

		conn, _, err := f.dialer.DialContext(ctx, f.cfg.WSEndpoint, nil)
		if err != nil {
			f.logger.Warn("okx ws dial failed", "stream", streamType, "error", err)
			if !sleepCtx(ctx, backoff) {
				return
			}
			backoff = nextBack(backoff, reconnectMax)
			continue
		}
		backoff = reconnectMin

		// Subscribe.
		subMsg := map[string]any{
			"op":   "subscribe",
			"args": args,
		}
		_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
		if err := conn.WriteJSON(subMsg); err != nil {
			f.logger.Warn("okx ws subscribe failed", "stream", streamType, "error", err)
			_ = conn.Close()
			if !sleepCtx(ctx, backoff) {
				return
			}
			backoff = nextBack(backoff, reconnectMax)
			continue
		}

		// Read loop with ping.
		err = f.okxReadLoop(ctx, conn, handler, readTimeout, writeTimeout, pingInterval)
		_ = conn.Close()
		if err == nil || ctx.Err() != nil {
			return
		}
		f.logger.Warn("okx ws disconnected", "stream", streamType, "error", err)
		if !sleepCtx(ctx, backoff) {
			return
		}
		backoff = nextBack(backoff, reconnectMax)
	}
}

// okxReadLoop reads messages from the OKX WebSocket connection until error or context cancellation.
func (f *OKXFeed) okxReadLoop(ctx context.Context, conn *websocket.Conn, handler func([]byte), readTimeout, writeTimeout, pingInterval time.Duration) error {
	_ = conn.SetReadDeadline(time.Now().Add(readTimeout))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(readTimeout))
	})

	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				_ = conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(writeTimeout))
			}
		}
	}()
	defer close(done)

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		_ = conn.SetReadDeadline(time.Now().Add(readTimeout))
		handler(msg)
	}
}

// handleKlineMessage parses an OKX kline WebSocket message and dispatches it.
func (f *OKXFeed) handleKlineMessage(msg []byte) {
	var envelope struct {
		Event string `json:"event"`
		Arg   struct {
			Channel string `json:"channel"`
			InstID  string `json:"instId"`
		} `json:"arg"`
		Data [][]string `json:"data"`
	}
	if err := json.Unmarshal(msg, &envelope); err != nil {
		return
	}
	// Skip event messages (subscribe confirmations, errors).
	if envelope.Event != "" {
		return
	}
	if !strings.HasPrefix(envelope.Arg.Channel, "candle") {
		return
	}

	// Extract the original interval from the channel name (e.g. "candle1m" -> "1m").
	rawInterval := strings.TrimPrefix(envelope.Arg.Channel, "candle")
	interval := fromOKXInterval(rawInterval)
	instID := envelope.Arg.InstID

	for _, row := range envelope.Data {
		if len(row) < 9 {
			continue
		}
		ts, _ := strconv.ParseInt(row[0], 10, 64)
		kline := contracts.Kline{
			Venue:    contracts.VenueOKX,
			Symbol:   instID,
			Interval: interval,
			OpenTime: ts,
			Open:     parseFloat(row[1]),
			High:     parseFloat(row[2]),
			Low:      parseFloat(row[3]),
			Close:    parseFloat(row[4]),
			Volume:   parseFloat(row[5]),
			Closed:   row[8] == "1",
		}
		if f.onKline != nil {
			f.onKline(kline)
		}
	}
}

// handleDepthMessage parses an OKX books5 WebSocket message and dispatches it.
func (f *OKXFeed) handleDepthMessage(msg []byte) {
	var envelope struct {
		Event string `json:"event"`
		Arg   struct {
			Channel string `json:"channel"`
			InstID  string `json:"instId"`
		} `json:"arg"`
		Data []struct {
			Asks [][]string `json:"asks"`
			Bids [][]string `json:"bids"`
			TS   string     `json:"ts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(msg, &envelope); err != nil {
		return
	}
	if envelope.Event != "" {
		return
	}
	if envelope.Arg.Channel != "books5" {
		return
	}
	if len(envelope.Data) == 0 {
		return
	}

	for _, d := range envelope.Data {
		ts, _ := strconv.ParseInt(d.TS, 10, 64)
		snap := contracts.DepthSnapshot{
			Venue:  contracts.VenueOKX,
			Symbol: envelope.Arg.InstID,
			Bids:   parseOKXDepthLevels(d.Bids),
			Asks:   parseOKXDepthLevels(d.Asks),
			TSms:   ts,
		}
		if f.onDepth != nil {
			f.onDepth(snap)
		}
	}
}

// parseOKXDepthLevels converts OKX depth rows [price, qty, 0, numOrders] to DepthLevel slices.
func parseOKXDepthLevels(levels [][]string) []contracts.DepthLevel {
	result := make([]contracts.DepthLevel, 0, len(levels))
	for _, l := range levels {
		if len(l) < 2 {
			continue
		}
		result = append(result, contracts.DepthLevel{
			Price:    parseFloat(l[0]),
			Quantity: parseFloat(l[1]),
		})
	}
	return result
}

// okxInterval maps canonical intervals to OKX format.
func okxInterval(interval string) string {
	switch interval {
	case "1h":
		return "1H"
	case "4h":
		return "4H"
	case "1d":
		return "1D"
	default:
		return interval // "1m","5m","15m" are the same
	}
}

// fromOKXInterval maps OKX interval format back to canonical.
func fromOKXInterval(interval string) string {
	switch interval {
	case "1H":
		return "1h"
	case "4H":
		return "4h"
	case "1D":
		return "1d"
	default:
		return interval
	}
}
