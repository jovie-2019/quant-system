package adminapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

// lokiURL returns the Loki base URL from env or default.
func lokiURL() string {
	if u := os.Getenv("LOKI_URL"); u != "" {
		return u
	}
	return "http://loki:3100"
}

// logLine represents a single parsed log entry from Loki.
type logLine struct {
	TS     string         `json:"ts"`
	Level  string         `json:"level"`
	Msg    string         `json:"msg"`
	Fields map[string]any `json:"fields,omitempty"`
}

// strategyLogsResponse is the JSON response for the strategy logs endpoint.
type strategyLogsResponse struct {
	StrategyID string    `json:"strategy_id"`
	Lines      []logLine `json:"lines"`
	Count      int       `json:"count"`
}

// lokiQueryRangeResponse is a partial representation of the Loki query_range response.
type lokiQueryRangeResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Stream map[string]string `json:"stream"`
			Values [][]string        `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

// HandleStrategyLogs handles GET /api/v1/strategies/{id}/logs.
// Query params: limit (default 200), since (default "1h").
// Queries Loki for logs matching the strategy_id.
func (s *Server) HandleStrategyLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET required")
		return
	}

	// Parse strategy ID from URL path.
	idStr := s.pathParam(r, "strategy_id")
	if idStr == "" {
		s.writeError(w, http.StatusBadRequest, "bad_request", "missing strategy id")
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", "invalid strategy id")
		return
	}

	// Load strategy config from store to get strategy_id string.
	cfg, found, err := s.store.GetStrategyConfig(r.Context(), id)
	if err != nil {
		s.logger.Error("HandleStrategyLogs: store error", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal", "failed to load strategy config")
		return
	}
	if !found {
		s.writeError(w, http.StatusNotFound, "not_found", "strategy not found")
		return
	}

	// Read query params.
	limitStr := r.URL.Query().Get("limit")
	if limitStr == "" {
		limitStr = "200"
	}
	if _, err := strconv.Atoi(limitStr); err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", "invalid limit parameter")
		return
	}

	sinceStr := r.URL.Query().Get("since")
	if sinceStr == "" {
		sinceStr = "1h"
	}
	sinceDur, err := time.ParseDuration(sinceStr)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", "invalid since parameter: must be a Go duration string (e.g. 5m, 1h, 6h)")
		return
	}

	// Build Loki query_range request.
	now := time.Now()
	startNano := now.Add(-sinceDur).UnixNano()
	endNano := now.UnixNano()

	query := fmt.Sprintf(`{service=~".+"} |= "%s"`, cfg.StrategyID)

	lokiReqURL := fmt.Sprintf("%s/loki/api/v1/query_range?query=%s&limit=%s&start=%d&end=%d&direction=backward",
		lokiURL(),
		url.QueryEscape(query),
		limitStr,
		startNano,
		endNano,
	)

	// Query Loki with a 5-second timeout.
	httpClient := &http.Client{Timeout: 5 * time.Second}
	lokiResp, err := httpClient.Get(lokiReqURL)
	if err != nil {
		s.logger.Error("HandleStrategyLogs: loki request failed", "error", err)
		s.writeError(w, http.StatusBadGateway, "loki_error", "failed to query Loki")
		return
	}
	defer lokiResp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(lokiResp.Body, 5<<20)) // 5 MB limit
	if err != nil {
		s.logger.Error("HandleStrategyLogs: loki read failed", "error", err)
		s.writeError(w, http.StatusBadGateway, "loki_error", "failed to read Loki response")
		return
	}

	if lokiResp.StatusCode != http.StatusOK {
		s.logger.Error("HandleStrategyLogs: loki returned non-200", "status", lokiResp.StatusCode, "body", string(body))
		s.writeError(w, http.StatusBadGateway, "loki_error", fmt.Sprintf("Loki returned status %d", lokiResp.StatusCode))
		return
	}

	// Parse Loki response.
	var lokiResult lokiQueryRangeResponse
	if err := json.Unmarshal(body, &lokiResult); err != nil {
		s.logger.Error("HandleStrategyLogs: loki parse failed", "error", err)
		s.writeError(w, http.StatusBadGateway, "loki_error", "failed to parse Loki response")
		return
	}

	// Extract log lines from all streams.
	var lines []logLine
	for _, stream := range lokiResult.Data.Result {
		for _, value := range stream.Values {
			if len(value) < 2 {
				continue
			}
			line := parseLogLine(value[0], value[1])
			lines = append(lines, line)
		}
	}

	if lines == nil {
		lines = []logLine{}
	}

	s.writeJSON(w, http.StatusOK, strategyLogsResponse{
		StrategyID: cfg.StrategyID,
		Lines:      lines,
		Count:      len(lines),
	})
}

// parseLogLine parses a Loki log line (nanosecond timestamp + JSON string) into a logLine.
func parseLogLine(nsTimestamp, raw string) logLine {
	// Try to parse the raw log line as JSON.
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		// Not valid JSON; return the raw string as the message.
		return logLine{
			TS:  nsToTimeString(nsTimestamp),
			Msg: raw,
		}
	}

	line := logLine{
		Fields: make(map[string]any),
	}

	// Extract known fields.
	if v, ok := parsed["time"]; ok {
		line.TS = fmt.Sprintf("%v", v)
		delete(parsed, "time")
	} else {
		line.TS = nsToTimeString(nsTimestamp)
	}

	if v, ok := parsed["level"]; ok {
		line.Level = fmt.Sprintf("%v", v)
		delete(parsed, "level")
	}

	if v, ok := parsed["msg"]; ok {
		line.Msg = fmt.Sprintf("%v", v)
		delete(parsed, "msg")
	}

	// Remaining fields go into Fields.
	for k, v := range parsed {
		line.Fields[k] = v
	}
	if len(line.Fields) == 0 {
		line.Fields = nil
	}

	return line
}

// nsToTimeString converts a nanosecond timestamp string to an RFC3339Nano time string.
func nsToTimeString(ns string) string {
	n, err := strconv.ParseInt(ns, 10, 64)
	if err != nil {
		return ns
	}
	return time.Unix(0, n).UTC().Format(time.RFC3339Nano)
}
