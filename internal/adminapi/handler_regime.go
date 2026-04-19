package adminapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"quant-system/internal/marketstore"
	"quant-system/internal/regime"
)

// regime-related HTTP handlers.
//
// Routes exposed (all under /api/v1/, all JWT-gated):
//   POST   /api/v1/regime/compute        run the threshold classifier over a window of klines
//   GET    /api/v1/regime/history        time series for drill-down
//   GET    /api/v1/regime/matrix         latest label per (venue, symbol, interval, method)
//
// The compute path reads klines from the configured KlineStore, invokes
// regime.ClassifyKlines, and persists output into the RegimeStore. Both
// sources/sinks are optional: if the admin-api runs without ClickHouse,
// requests return 503 with a clear message so the UI can fall back.

// ComputeRegimeRequest is the body of POST /api/v1/regime/compute.
type ComputeRegimeRequest struct {
	Venue    string `json:"venue"`
	Symbol   string `json:"symbol"`
	Interval string `json:"interval"`
	StartMS  int64  `json:"start_ms"`
	EndMS    int64  `json:"end_ms"`

	// Optional classifier knobs. Zero values pick package defaults.
	ADXPeriod      int     `json:"adx_period,omitempty"`
	ATRPeriod      int     `json:"atr_period,omitempty"`
	BBPeriod       int     `json:"bb_period,omitempty"`
	BBStdDev       float64 `json:"bb_stddev,omitempty"`
	HurstLookback  int     `json:"hurst_lookback,omitempty"`
	HurstMinN      int     `json:"hurst_min_n,omitempty"`
	Thresholds     *ThresholdsPayload `json:"thresholds,omitempty"`
}

// ThresholdsPayload mirrors regime.Thresholds but uses pointers so the UI
// can send only the fields it wants to override.
type ThresholdsPayload struct {
	ADXTrend        *float64 `json:"adx_trend,omitempty"`
	ADXRange        *float64 `json:"adx_range,omitempty"`
	HurstPersistent *float64 `json:"hurst_persistent,omitempty"`
	HurstMeanRevert *float64 `json:"hurst_mean_revert,omitempty"`
	ATRPercentHigh  *float64 `json:"atr_percent_high,omitempty"`
	VolumeRatioLow  *float64 `json:"volume_ratio_low,omitempty"`
}

// ComputeRegimeResponse summarises a classifier run.
type ComputeRegimeResponse struct {
	Venue         string           `json:"venue"`
	Symbol        string           `json:"symbol"`
	Interval      string           `json:"interval"`
	BarsFetched   int              `json:"bars_fetched"`
	RecordsStored int              `json:"records_stored"`
	Latest        *regime.Record   `json:"latest,omitempty"`
	Tail          []regime.Record  `json:"tail"`           // up to last 20 for quick UI preview
	Method        regime.Method    `json:"method"`
}

// RegimeHistoryResponse envelopes /regime/history output.
type RegimeHistoryResponse struct {
	Items []regime.Record `json:"items"`
	Count int             `json:"count"`
}

// RegimeMatrixRow is one cell of the matrix view.
type RegimeMatrixRow struct {
	Venue      string        `json:"venue"`
	Symbol     string        `json:"symbol"`
	Interval   string        `json:"interval"`
	Method     regime.Method `json:"method"`
	Regime     regime.Regime `json:"regime"`
	Confidence float64       `json:"confidence"`
	BarTime    int64         `json:"bar_time"`
	ADX        float64       `json:"adx"`
	Hurst      float64       `json:"hurst"`
	BBW        float64       `json:"bbw"`
	ATR        float64       `json:"atr"`
}

// RegimeMatrixResponse is the payload of /regime/matrix.
type RegimeMatrixResponse struct {
	Rows []RegimeMatrixRow `json:"rows"`
}

// HandleComputeRegime runs the threshold classifier and stores the output.
func (s *Server) HandleComputeRegime(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	if s.klines == nil || s.regimes == nil {
		s.writeError(w, http.StatusServiceUnavailable, "store_unavailable",
			"regime compute requires both KlineStore and RegimeStore (start admin-api with CLICKHOUSE_ADDR set)")
		return
	}

	var req ComputeRegimeRequest
	if err := s.readJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := validateComputeRequest(req); err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	klines, err := s.klines.Query(r.Context(), marketstore.KlineQuery{
		Venue:    req.Venue,
		Symbol:   req.Symbol,
		Interval: req.Interval,
		StartMS:  req.StartMS,
		EndMS:    req.EndMS,
	})
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "kline_query_failed", err.Error())
		return
	}
	if len(klines) == 0 {
		s.writeError(w, http.StatusNotFound, "no_klines",
			fmt.Sprintf("no klines for %s %s in [%d, %d]", req.Symbol, req.Interval, req.StartMS, req.EndMS))
		return
	}

	cfg := regime.ClassifyConfig{
		ADXPeriod:     req.ADXPeriod,
		ATRPeriod:     req.ATRPeriod,
		BBPeriod:      req.BBPeriod,
		BBStdDev:      req.BBStdDev,
		HurstLookback: req.HurstLookback,
		HurstMinN:     req.HurstMinN,
		Thresholds:    mergeThresholds(req.Thresholds),
	}
	records := regime.ClassifyKlines(klines, cfg)

	// Stamp venue/symbol/interval on each record for stores that cannot
	// infer them from the Kline struct.
	for i := range records {
		records[i].Venue = req.Venue
		records[i].Symbol = req.Symbol
		records[i].Interval = req.Interval
	}

	if err := s.regimes.UpsertRegimes(r.Context(), records); err != nil {
		s.writeError(w, http.StatusInternalServerError, "regime_upsert_failed", err.Error())
		return
	}

	tail := records
	if len(tail) > 20 {
		tail = tail[len(tail)-20:]
	}
	var latest *regime.Record
	if len(records) > 0 {
		l := records[len(records)-1]
		latest = &l
	}

	s.writeJSON(w, http.StatusOK, ComputeRegimeResponse{
		Venue:         req.Venue,
		Symbol:        req.Symbol,
		Interval:      req.Interval,
		BarsFetched:   len(klines),
		RecordsStored: len(records),
		Latest:        latest,
		Tail:          tail,
		Method:        regime.MethodThreshold,
	})
}

// HandleRegimeHistory returns a time series of regime records for drill-down.
func (s *Server) HandleRegimeHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET required")
		return
	}
	if s.regimes == nil {
		s.writeError(w, http.StatusServiceUnavailable, "store_unavailable",
			"regime store not configured")
		return
	}
	q := r.URL.Query()
	startMS, _ := strconv.ParseInt(q.Get("start_ms"), 10, 64)
	endMS, _ := strconv.ParseInt(q.Get("end_ms"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 10_000 {
		limit = 1_000
	}
	rq := marketstore.RegimeQuery{
		Venue:    q.Get("venue"),
		Symbol:   q.Get("symbol"),
		Interval: q.Get("interval"),
		Method:   regime.Method(q.Get("method")),
		StartMS:  startMS,
		EndMS:    endMS,
		Limit:    limit,
	}
	items, err := s.regimes.QueryRegimes(r.Context(), rq)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, RegimeHistoryResponse{Items: items, Count: len(items)})
}

// HandleRegimeMatrix returns the latest label per (symbol, interval). The
// query string accepts comma-separated lists: symbols=BTC-USDT,ETH-USDT
// and intervals=1m,5m,1h — the response has one row per combination.
func (s *Server) HandleRegimeMatrix(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET required")
		return
	}
	if s.regimes == nil {
		s.writeError(w, http.StatusServiceUnavailable, "store_unavailable",
			"regime store not configured")
		return
	}
	q := r.URL.Query()
	symbols := splitAndTrim(q.Get("symbols"))
	intervals := splitAndTrim(q.Get("intervals"))
	if len(symbols) == 0 || len(intervals) == 0 {
		s.writeError(w, http.StatusBadRequest, "bad_request", "symbols and intervals query params are required (comma-separated)")
		return
	}
	venue := q.Get("venue")
	method := regime.Method(q.Get("method"))
	if method == "" {
		method = regime.MethodThreshold
	}

	keys := make([]marketstore.RegimeMatrixKey, 0, len(symbols)*len(intervals))
	for _, sym := range symbols {
		for _, iv := range intervals {
			keys = append(keys, marketstore.RegimeMatrixKey{
				Venue: venue, Symbol: sym, Interval: iv, Method: method,
			})
		}
	}

	records, err := s.regimes.LatestRegimes(r.Context(), keys)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "matrix_query_failed", err.Error())
		return
	}
	rows := make([]RegimeMatrixRow, 0, len(records))
	for _, rec := range records {
		rows = append(rows, RegimeMatrixRow{
			Venue:      rec.Venue,
			Symbol:     rec.Symbol,
			Interval:   rec.Interval,
			Method:     rec.Method,
			Regime:     rec.Regime,
			Confidence: rec.Confidence,
			BarTime:    rec.BarTime,
			ADX:        rec.Features.ADX,
			Hurst:      rec.Features.Hurst,
			BBW:        rec.Features.BBW,
			ATR:        rec.Features.ATR,
		})
	}
	s.writeJSON(w, http.StatusOK, RegimeMatrixResponse{Rows: rows})
}

func mergeThresholds(p *ThresholdsPayload) regime.Thresholds {
	t := regime.DefaultThresholds()
	if p == nil {
		return t
	}
	if p.ADXTrend != nil {
		t.ADXTrend = *p.ADXTrend
	}
	if p.ADXRange != nil {
		t.ADXRange = *p.ADXRange
	}
	if p.HurstPersistent != nil {
		t.HurstPersistent = *p.HurstPersistent
	}
	if p.HurstMeanRevert != nil {
		t.HurstMeanRevert = *p.HurstMeanRevert
	}
	if p.ATRPercentHigh != nil {
		t.ATRPercentHigh = *p.ATRPercentHigh
	}
	if p.VolumeRatioLow != nil {
		t.VolumeRatioLow = *p.VolumeRatioLow
	}
	return t
}

func validateComputeRequest(req ComputeRegimeRequest) error {
	if strings.TrimSpace(req.Symbol) == "" {
		return errors.New("symbol is required")
	}
	if strings.TrimSpace(req.Interval) == "" {
		return errors.New("interval is required")
	}
	if req.StartMS <= 0 || req.EndMS <= 0 {
		return errors.New("start_ms and end_ms are required")
	}
	if req.StartMS >= req.EndMS {
		return errors.New("start_ms must be < end_ms")
	}
	return nil
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Silence "unused import" if context becomes unreferenced after future edits.
var _ = context.Background
