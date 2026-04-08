package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/net/proxy"

	"quant-system/pkg/contracts"
)

// BinanceFeedConfig holds configuration for the Binance market data feed.
type BinanceFeedConfig struct {
	WSEndpoint  string   // default wss://stream.binance.com:9443/ws
	RESTBaseURL string   // default https://api.binance.com
	Symbols     []string // canonical format, e.g. "BTC-USDT"
	Intervals   []string // default ["1m","5m","15m"]
}

// BinanceFeed streams kline and depth data from Binance via WebSocket and REST.
type BinanceFeed struct {
	cfg    BinanceFeedConfig
	dialer *websocket.Dialer
	client *http.Client
	logger *slog.Logger

	onKline func(contracts.Kline)
	onDepth func(contracts.DepthSnapshot)
}

// NewBinanceFeed creates a new BinanceFeed with sensible defaults.
func NewBinanceFeed(cfg BinanceFeedConfig, onKline func(contracts.Kline), onDepth func(contracts.DepthSnapshot), logger *slog.Logger) *BinanceFeed {
	if cfg.WSEndpoint == "" {
		cfg.WSEndpoint = "wss://stream.binance.com:9443/ws"
	}
	if cfg.RESTBaseURL == "" {
		cfg.RESTBaseURL = "https://api.binance.com"
	}
	if len(cfg.Intervals) == 0 {
		cfg.Intervals = []string{"1m", "5m", "15m"}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &BinanceFeed{
		cfg:     cfg,
		dialer:  newDialer(),
		client:  newClient(8 * time.Second),
		logger:  logger,
		onKline: onKline,
		onDepth: onDepth,
	}
}

// FetchHistoricalKlines retrieves historical klines from Binance REST API for warmup.
func (f *BinanceFeed) FetchHistoricalKlines(ctx context.Context, symbol, interval string, limit int) ([]contracts.Kline, error) {
	if limit <= 0 {
		limit = 200
	}
	binSym := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(symbol), "-", ""))
	url := fmt.Sprintf("%s/api/v3/klines?symbol=%s&interval=%s&limit=%d",
		strings.TrimRight(f.cfg.RESTBaseURL, "/"), binSym, interval, limit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("binance klines: build request: %w", err)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("binance klines: request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("binance klines: read body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("binance klines: status=%d body=%s", resp.StatusCode, string(body))
	}

	// Binance returns: [[openTime, open, high, low, close, volume, closeTime, ...], ...]
	var raw [][]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("binance klines: decode: %w", err)
	}

	canonical := toBinanceCanonical(binSym)
	klines := make([]contracts.Kline, 0, len(raw))
	for _, row := range raw {
		if len(row) < 7 {
			continue
		}
		k, err := parseBinanceKlineRow(row, canonical, interval)
		if err != nil {
			f.logger.Warn("binance klines: skip row", "error", err)
			continue
		}
		k.Closed = true
		klines = append(klines, k)
	}
	return klines, nil
}

// parseBinanceKlineRow parses a single kline array from Binance REST response.
func parseBinanceKlineRow(row []json.RawMessage, symbol, interval string) (contracts.Kline, error) {
	var openTime, closeTime int64
	var openS, highS, lowS, closeS, volS string

	if err := json.Unmarshal(row[0], &openTime); err != nil {
		return contracts.Kline{}, err
	}
	if err := json.Unmarshal(row[1], &openS); err != nil {
		return contracts.Kline{}, err
	}
	if err := json.Unmarshal(row[2], &highS); err != nil {
		return contracts.Kline{}, err
	}
	if err := json.Unmarshal(row[3], &lowS); err != nil {
		return contracts.Kline{}, err
	}
	if err := json.Unmarshal(row[4], &closeS); err != nil {
		return contracts.Kline{}, err
	}
	if err := json.Unmarshal(row[5], &volS); err != nil {
		return contracts.Kline{}, err
	}
	if err := json.Unmarshal(row[6], &closeTime); err != nil {
		return contracts.Kline{}, err
	}

	return contracts.Kline{
		Venue:     contracts.VenueBinance,
		Symbol:    symbol,
		Interval:  interval,
		OpenTime:  openTime,
		CloseTime: closeTime,
		Open:      parseFloat(openS),
		High:      parseFloat(highS),
		Low:       parseFloat(lowS),
		Close:     parseFloat(closeS),
		Volume:    parseFloat(volS),
	}, nil
}

// StartKlineStream connects to Binance kline WebSocket streams and dispatches updates.
func (f *BinanceFeed) StartKlineStream(ctx context.Context) error {
	if len(f.cfg.Symbols) == 0 {
		return fmt.Errorf("binance kline stream: no symbols configured")
	}

	params := make([]string, 0, len(f.cfg.Symbols)*len(f.cfg.Intervals))
	for _, sym := range f.cfg.Symbols {
		bs := binanceSymbol(sym)
		for _, iv := range f.cfg.Intervals {
			params = append(params, bs+"@kline_"+iv)
		}
	}

	f.runWS(ctx, params, f.handleKlineMessage, "kline")
	return nil
}

// StartDepthStream connects to Binance depth WebSocket streams and dispatches snapshots.
func (f *BinanceFeed) StartDepthStream(ctx context.Context) error {
	if len(f.cfg.Symbols) == 0 {
		return fmt.Errorf("binance depth stream: no symbols configured")
	}

	params := make([]string, 0, len(f.cfg.Symbols))
	for _, sym := range f.cfg.Symbols {
		params = append(params, binanceSymbol(sym)+"@depth20@100ms")
	}

	f.runWS(ctx, params, f.handleDepthMessage, "depth")
	return nil
}

// runWS is the generic reconnect loop for Binance WebSocket streams.
func (f *BinanceFeed) runWS(ctx context.Context, params []string, handler func([]byte), streamType string) {
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
			f.logger.Warn("binance ws dial failed", "stream", streamType, "error", err)
			if !sleepCtx(ctx, backoff) {
				return
			}
			backoff = nextBack(backoff, reconnectMax)
			continue
		}
		backoff = reconnectMin

		// Subscribe.
		subMsg := map[string]any{
			"method": "SUBSCRIBE",
			"params": params,
			"id":     1,
		}
		_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
		if err := conn.WriteJSON(subMsg); err != nil {
			f.logger.Warn("binance ws subscribe failed", "stream", streamType, "error", err)
			_ = conn.Close()
			if !sleepCtx(ctx, backoff) {
				return
			}
			backoff = nextBack(backoff, reconnectMax)
			continue
		}

		// Read loop with ping.
		err = f.readLoop(ctx, conn, handler, readTimeout, writeTimeout, pingInterval)
		_ = conn.Close()
		if err == nil || ctx.Err() != nil {
			return
		}
		f.logger.Warn("binance ws disconnected", "stream", streamType, "error", err)
		if !sleepCtx(ctx, backoff) {
			return
		}
		backoff = nextBack(backoff, reconnectMax)
	}
}

// readLoop reads messages from the WebSocket connection until an error or context cancellation.
func (f *BinanceFeed) readLoop(ctx context.Context, conn *websocket.Conn, handler func([]byte), readTimeout, writeTimeout, pingInterval time.Duration) error {
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

// handleKlineMessage parses a Binance kline WebSocket message and dispatches it.
func (f *BinanceFeed) handleKlineMessage(msg []byte) {
	var payload struct {
		Event string `json:"e"`
		Symbol string `json:"s"`
		K      struct {
			OpenTime  int64  `json:"t"`
			CloseTime int64  `json:"T"`
			Interval  string `json:"i"`
			Open      string `json:"o"`
			High      string `json:"h"`
			Low       string `json:"l"`
			Close     string `json:"c"`
			Volume    string `json:"v"`
			Closed    bool   `json:"x"`
		} `json:"k"`
	}
	if err := json.Unmarshal(msg, &payload); err != nil {
		return
	}
	if payload.Event != "kline" {
		return
	}

	kline := contracts.Kline{
		Venue:     contracts.VenueBinance,
		Symbol:    toBinanceCanonical(payload.Symbol),
		Interval:  payload.K.Interval,
		OpenTime:  payload.K.OpenTime,
		CloseTime: payload.K.CloseTime,
		Open:      parseFloat(payload.K.Open),
		High:      parseFloat(payload.K.High),
		Low:       parseFloat(payload.K.Low),
		Close:     parseFloat(payload.K.Close),
		Volume:    parseFloat(payload.K.Volume),
		Closed:    payload.K.Closed,
	}
	if f.onKline != nil {
		f.onKline(kline)
	}
}

// handleDepthMessage parses a Binance depth WebSocket message and dispatches it.
func (f *BinanceFeed) handleDepthMessage(msg []byte) {
	// The depth20@100ms stream wraps inside a "stream" envelope for combined streams,
	// but when connecting to /ws directly it sends the raw payload.
	var payload struct {
		Stream       string     `json:"stream"`
		LastUpdateID int64      `json:"lastUpdateId"`
		Bids         [][]string `json:"bids"`
		Asks         [][]string `json:"asks"`
		Data         *struct {
			LastUpdateID int64      `json:"lastUpdateId"`
			Bids         [][]string `json:"bids"`
			Asks         [][]string `json:"asks"`
		} `json:"data"`
	}
	if err := json.Unmarshal(msg, &payload); err != nil {
		return
	}

	bids := payload.Bids
	asks := payload.Asks
	if payload.Data != nil {
		bids = payload.Data.Bids
		asks = payload.Data.Asks
	}
	if len(bids) == 0 && len(asks) == 0 {
		return
	}

	// Extract symbol from stream field if available (e.g. "btcusdt@depth20@100ms").
	symbol := ""
	if payload.Stream != "" {
		parts := strings.SplitN(payload.Stream, "@", 2)
		if len(parts) > 0 {
			symbol = toBinanceCanonical(parts[0])
		}
	}

	snap := contracts.DepthSnapshot{
		Venue:  contracts.VenueBinance,
		Symbol: symbol,
		Bids:   parseDepthLevels(bids),
		Asks:   parseDepthLevels(asks),
		TSms:   time.Now().UnixMilli(),
	}
	if f.onDepth != nil {
		f.onDepth(snap)
	}
}

// parseDepthLevels converts Binance price/quantity string pairs to DepthLevel slices.
func parseDepthLevels(levels [][]string) []contracts.DepthLevel {
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

// binanceSymbol converts canonical symbol to Binance lowercase format (e.g. "BTC-USDT" -> "btcusdt").
func binanceSymbol(canonical string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(canonical), "-", ""))
}

// toBinanceCanonical converts compact Binance symbol to canonical format (e.g. "BTCUSDT" -> "BTC-USDT").
func toBinanceCanonical(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	if strings.Contains(s, "-") {
		return s
	}
	for _, suffix := range []string{"USDT", "USDC", "BTC", "ETH"} {
		if strings.HasSuffix(s, suffix) {
			return s[:len(s)-len(suffix)] + "-" + suffix
		}
	}
	return s
}

// --- shared helpers (duplicated from adapter package for package isolation) ---

// parseFloat parses a decimal string to float64, returning 0 on failure.
func parseFloat(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return v
}

// sleepCtx blocks for duration d or until ctx is done. Returns true if the sleep completed.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// nextBack doubles the backoff up to max.
func nextBack(current, max time.Duration) time.Duration {
	n := current * 2
	if n > max {
		return max
	}
	return n
}

// newDialer creates a websocket.Dialer with SOCKS5 proxy support.
func newDialer() *websocket.Dialer {
	for _, env := range []string{"ALL_PROXY", "all_proxy"} {
		val := os.Getenv(env)
		if strings.HasPrefix(val, "socks5://") {
			addr := strings.TrimPrefix(val, "socks5://")
			socks5Dialer, err := proxy.SOCKS5("tcp", addr, nil, proxy.Direct)
			if err != nil {
				slog.Warn("marketdata: socks5 ws proxy init failed", "addr", addr, "error", err)
				break
			}
			slog.Info("marketdata: ws dialer using SOCKS5 proxy", "addr", addr)
			return &websocket.Dialer{
				NetDial: func(network, a string) (net.Conn, error) {
					return socks5Dialer.Dial(network, a)
				},
				HandshakeTimeout: 10 * time.Second,
			}
		}
	}
	return &websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 10 * time.Second,
	}
}

// newClient creates an http.Client with SOCKS5 proxy support.
func newClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	for _, env := range []string{"ALL_PROXY", "all_proxy"} {
		val := os.Getenv(env)
		if strings.HasPrefix(val, "socks5://") {
			addr := strings.TrimPrefix(val, "socks5://")
			dialer, err := proxy.SOCKS5("tcp", addr, nil, proxy.Direct)
			if err != nil {
				slog.Warn("marketdata: socks5 rest proxy init failed", "addr", addr, "error", err)
				break
			}
			transport.DialContext = func(ctx context.Context, network, a string) (net.Conn, error) {
				return dialer.Dial(network, a)
			}
			slog.Info("marketdata: rest using SOCKS5 proxy", "addr", addr)
			break
		}
	}
	return &http.Client{Timeout: timeout, Transport: transport}
}
