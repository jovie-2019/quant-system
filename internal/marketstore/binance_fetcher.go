package marketstore

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/proxy"

	"quant-system/pkg/contracts"
)

// defaultHTTPClient builds an http.Client that honours ALL_PROXY for
// SOCKS5 proxying. Operators running in networks that require a proxy
// (Mainland China workstations commonly set ALL_PROXY=socks5://...)
// expect every outbound HTTP call to route through it, mirroring the
// behaviour of internal/adapter's rest client.
func defaultHTTPClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	for _, env := range []string{"ALL_PROXY", "all_proxy"} {
		val := os.Getenv(env)
		if !strings.HasPrefix(val, "socks5://") {
			continue
		}
		addr := strings.TrimPrefix(val, "socks5://")
		dialer, err := proxy.SOCKS5("tcp", addr, nil, proxy.Direct)
		if err != nil {
			slog.Warn("marketstore: socks5 proxy init failed", "addr", addr, "error", err)
			break
		}
		transport.DialContext = func(_ context.Context, network, a string) (net.Conn, error) {
			return dialer.Dial(network, a)
		}
		slog.Info("marketstore: Binance REST using SOCKS5 proxy", "addr", addr)
		break
	}
	return &http.Client{Timeout: timeout, Transport: transport}
}

// BinanceFetcher pulls historical klines from the Binance Spot REST API with
// an explicit [startMS, endMS] window. The existing marketdata.BinanceFeed
// exposes only a `limit`-based fetch aimed at warm-up; for backfilling
// months of history we need windowed paging, which lives here.
type BinanceFetcher struct {
	baseURL string
	client  *http.Client
}

// BinanceFetcherConfig parameters for NewBinanceFetcher.
type BinanceFetcherConfig struct {
	// BaseURL defaults to https://api.binance.com.
	BaseURL string
	// HTTPClient lets callers inject a proxy-aware client. When nil a client
	// with a 15s timeout is created.
	HTTPClient *http.Client
}

// NewBinanceFetcher returns a ready-to-use fetcher.
func NewBinanceFetcher(cfg BinanceFetcherConfig) *BinanceFetcher {
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = "https://api.binance.com"
	}
	client := cfg.HTTPClient
	if client == nil {
		client = defaultHTTPClient(15 * time.Second)
	}
	return &BinanceFetcher{baseURL: base, client: client}
}

// maxBinanceKlinesPerRequest is the Binance Spot limit for a single
// /api/v3/klines call. Binance silently caps larger requests, so we enforce
// this here to keep paging logic exact.
const maxBinanceKlinesPerRequest = 1000

// FetchRange fetches all klines in the closed interval [startMS, endMS]. It
// pages the Binance REST endpoint with a 1000-row limit per call and
// advances by the interval duration so consecutive calls do not overlap.
//
// Symbol is normalised to Binance format (BTCUSDT, no dash). Interval uses
// Binance codes (1m, 5m, 15m, 1h, 4h, 1d, ...).
//
// A sleep of `pacing` is applied between calls to stay well inside Binance's
// weight limits (a default 200ms is usually safe for 1m bars).
func (f *BinanceFetcher) FetchRange(ctx context.Context, symbol, interval string, startMS, endMS int64, pacing time.Duration) ([]contracts.Kline, error) {
	if startMS <= 0 || endMS <= 0 || startMS > endMS {
		return nil, fmt.Errorf("binance fetcher: invalid window [%d,%d]", startMS, endMS)
	}
	stepMS, err := intervalToMS(interval)
	if err != nil {
		return nil, err
	}
	binSym := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(symbol), "-", ""))

	var all []contracts.Kline
	cursor := startMS
	for cursor <= endMS {
		select {
		case <-ctx.Done():
			return all, ctx.Err()
		default:
		}
		batch, err := f.fetchOnce(ctx, binSym, interval, cursor, endMS, maxBinanceKlinesPerRequest)
		if err != nil {
			return all, err
		}
		if len(batch) == 0 {
			break
		}
		all = append(all, batch...)
		last := batch[len(batch)-1].OpenTime
		nextCursor := last + stepMS
		if nextCursor <= cursor {
			// Pathological case: would loop forever. Bail out.
			break
		}
		cursor = nextCursor
		if pacing > 0 && cursor <= endMS {
			timer := time.NewTimer(pacing)
			select {
			case <-ctx.Done():
				timer.Stop()
				return all, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return all, nil
}

// fetchOnce performs one Binance REST call and returns the parsed klines.
func (f *BinanceFetcher) fetchOnce(ctx context.Context, binSym, interval string, startMS, endMS int64, limit int) ([]contracts.Kline, error) {
	q := url.Values{}
	q.Set("symbol", binSym)
	q.Set("interval", interval)
	q.Set("startTime", strconv.FormatInt(startMS, 10))
	q.Set("endTime", strconv.FormatInt(endMS, 10))
	q.Set("limit", strconv.Itoa(limit))
	reqURL := f.baseURL + "/api/v3/klines?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("binance fetcher: build request: %w", err)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("binance fetcher: do: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("binance fetcher: read: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("binance fetcher: status=%d body=%s", resp.StatusCode, string(body))
	}

	var raw [][]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("binance fetcher: decode: %w", err)
	}

	out := make([]contracts.Kline, 0, len(raw))
	for _, row := range raw {
		if len(row) < 7 {
			continue
		}
		k, err := parseBinanceKlineRow(row, binSym, interval)
		if err != nil {
			continue
		}
		k.Closed = true
		out = append(out, k)
	}
	return out, nil
}

// parseBinanceKlineRow parses a single Binance kline array.
// Response shape: [openTime, open, high, low, close, volume, closeTime, ...].
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
		Open:      parseFloatOr0(openS),
		High:      parseFloatOr0(highS),
		Low:       parseFloatOr0(lowS),
		Close:     parseFloatOr0(closeS),
		Volume:    parseFloatOr0(volS),
	}, nil
}

func parseFloatOr0(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}

// intervalToMS converts a Binance interval code to its millisecond duration.
// Supports the common spot intervals; unrecognised codes return an error.
func intervalToMS(interval string) (int64, error) {
	interval = strings.ToLower(strings.TrimSpace(interval))
	if len(interval) < 2 {
		return 0, fmt.Errorf("binance fetcher: unknown interval %q", interval)
	}
	unit := interval[len(interval)-1]
	numStr := interval[:len(interval)-1]
	n, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("binance fetcher: unknown interval %q", interval)
	}
	switch unit {
	case 'm':
		return n * 60 * 1000, nil
	case 'h':
		return n * 60 * 60 * 1000, nil
	case 'd':
		return n * 24 * 60 * 60 * 1000, nil
	case 'w':
		return n * 7 * 24 * 60 * 60 * 1000, nil
	default:
		return 0, fmt.Errorf("binance fetcher: unknown interval unit %q in %q", string(unit), interval)
	}
}
