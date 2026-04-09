package adminapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

// serviceStatus represents the health status of a single service.
type serviceStatus struct {
	Status string `json:"status"`
	Info   string `json:"info"`
}

// natsStreamInfo represents a single NATS JetStream stream.
type natsStreamInfo struct {
	Name      string `json:"name"`
	Messages  uint64 `json:"messages"`
	Bytes     uint64 `json:"bytes"`
	Consumers int    `json:"consumers"`
}

// tableStats represents row count for a MySQL table.
type tableStats struct {
	Name string `json:"name"`
	Rows int64  `json:"rows"`
	Err  string `json:"error,omitempty"`
}

// systemStatusResponse is the JSON response for the system status endpoint.
type systemStatusResponse struct {
	Services    map[string]serviceStatus `json:"services"`
	NATSStreams []natsStreamInfo         `json:"nats_streams"`
	MySQLTables []tableStats             `json:"mysql_tables"`
}

// natsMonitorURL returns the NATS monitoring base URL from env or default.
func natsMonitorURL() string {
	if u := os.Getenv("NATS_MONITOR_URL"); u != "" {
		return u
	}
	return "http://nats:8222"
}

// HandleSystemStatus handles GET /api/v1/system/status.
// Returns system health, service statuses, and infrastructure info.
func (s *Server) HandleSystemStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET required")
		return
	}

	resp := systemStatusResponse{
		Services:    make(map[string]serviceStatus),
		NATSStreams: []natsStreamInfo{},
		MySQLTables: []tableStats{},
	}

	var mu sync.Mutex
	var wg sync.WaitGroup

	probeTimeout := 2 * time.Second

	// Probe MySQL.
	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(r.Context(), probeTimeout)
		defer cancel()

		db := s.store.DB()
		status := serviceStatus{Status: "ok", Info: "connected"}
		if err := db.PingContext(ctx); err != nil {
			status = serviceStatus{Status: "error", Info: fmt.Sprintf("ping failed: %v", err)}
		}

		// Query table row counts.
		tables := []string{"orders", "positions", "exchanges", "api_keys", "strategy_configs"}
		tableResults := make([]tableStats, len(tables))
		for i, tbl := range tables {
			var count int64
			row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+tbl)
			if err := row.Scan(&count); err != nil {
				tableResults[i] = tableStats{Name: tbl, Rows: -1, Err: err.Error()}
			} else {
				tableResults[i] = tableStats{Name: tbl, Rows: count}
			}
		}

		mu.Lock()
		resp.Services["mysql"] = status
		resp.MySQLTables = tableResults
		mu.Unlock()
	}()

	// Probe NATS health + JetStream info.
	wg.Add(1)
	go func() {
		defer wg.Done()

		baseURL := natsMonitorURL()
		httpClient := &http.Client{Timeout: probeTimeout}

		// Health check.
		status := serviceStatus{Status: "ok", Info: "healthy"}
		healthResp, err := httpClient.Get(baseURL + "/healthz")
		if err != nil {
			status = serviceStatus{Status: "error", Info: fmt.Sprintf("unreachable: %v", err)}
			mu.Lock()
			resp.Services["nats"] = status
			mu.Unlock()
			return
		}
		healthResp.Body.Close()
		if healthResp.StatusCode != http.StatusOK {
			status = serviceStatus{Status: "error", Info: fmt.Sprintf("healthz returned %d", healthResp.StatusCode)}
			mu.Lock()
			resp.Services["nats"] = status
			mu.Unlock()
			return
		}

		// JetStream info.
		jszResp, err := httpClient.Get(baseURL + "/jsz?streams=true")
		if err != nil {
			status.Info = "healthy (jsz unavailable)"
			mu.Lock()
			resp.Services["nats"] = status
			mu.Unlock()
			return
		}
		defer jszResp.Body.Close()

		body, err := io.ReadAll(io.LimitReader(jszResp.Body, 1<<20))
		if err != nil {
			status.Info = "healthy (jsz read error)"
			mu.Lock()
			resp.Services["nats"] = status
			mu.Unlock()
			return
		}

		var jsz jszResponse
		if err := json.Unmarshal(body, &jsz); err != nil {
			status.Info = "healthy (jsz parse error)"
			mu.Lock()
			resp.Services["nats"] = status
			mu.Unlock()
			return
		}

		// Summarize.
		totalMessages := uint64(0)
		streams := make([]natsStreamInfo, 0, len(jsz.AccountDetails))
		for _, acct := range jsz.AccountDetails {
			for _, si := range acct.StreamDetails {
				info := natsStreamInfo{
					Name:      si.Name,
					Messages:  si.State.Messages,
					Bytes:     si.State.Bytes,
					Consumers: si.State.Consumers,
				}
				streams = append(streams, info)
				totalMessages += si.State.Messages
			}
		}

		status.Info = fmt.Sprintf("streams=%d, messages=%d", len(streams), totalMessages)

		mu.Lock()
		resp.Services["nats"] = status
		resp.NATSStreams = streams
		mu.Unlock()
	}()

	// Probe Loki.
	wg.Add(1)
	go func() {
		defer wg.Done()

		httpClient := &http.Client{Timeout: probeTimeout}
		status := serviceStatus{Status: "ok", Info: "ready"}

		lokiResp, err := httpClient.Get("http://loki:3100/ready")
		if err != nil {
			status = serviceStatus{Status: "error", Info: fmt.Sprintf("unreachable: %v", err)}
			mu.Lock()
			resp.Services["loki"] = status
			mu.Unlock()
			return
		}
		lokiResp.Body.Close()

		if lokiResp.StatusCode != http.StatusOK {
			status = serviceStatus{Status: "error", Info: fmt.Sprintf("ready returned %d", lokiResp.StatusCode)}
		}

		mu.Lock()
		resp.Services["loki"] = status
		mu.Unlock()
	}()

	wg.Wait()

	s.writeJSON(w, http.StatusOK, resp)
}

// jszResponse is a partial representation of the NATS /jsz endpoint response.
type jszResponse struct {
	AccountDetails []jszAccount `json:"account_details"`
}

// jszAccount represents an account in the /jsz response.
type jszAccount struct {
	StreamDetails []jszStream `json:"stream_detail"`
}

// jszStream represents a stream in the /jsz response.
type jszStream struct {
	Name  string   `json:"name"`
	State jszState `json:"state"`
}

// jszState represents a stream state in the /jsz response.
type jszState struct {
	Messages  uint64 `json:"messages"`
	Bytes     uint64 `json:"bytes"`
	Consumers int    `json:"consumer_count"`
}
