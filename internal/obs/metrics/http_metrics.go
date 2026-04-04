package metrics

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

var startTime = time.Now()

var defaultRegistry = NewRegistry()

func ObserveHTTP(method, path string, status int, duration time.Duration) {
	defaultRegistry.ObserveHTTP(method, path, status, duration)
}

func ObserveRiskEvaluation(decision string, duration time.Duration) {
	defaultRegistry.ObserveRiskEvaluation(decision, duration)
}

func ObserveExecutionSubmit(outcome string, duration time.Duration) {
	defaultRegistry.ObserveExecutionSubmit(outcome, duration)
}

func ObserveExecutionGateway(operation, result string) {
	defaultRegistry.ObserveExecutionGateway(operation, result)
}

func ObserveTTLCacheGet(cache string, hit bool) {
	defaultRegistry.ObserveTTLCacheGet(cache, hit)
}

func ObserveTTLCacheEviction(cache, reason string) {
	defaultRegistry.ObserveTTLCacheEviction(cache, reason)
}

func ObserveTTLCachePurge(cache string, purged int) {
	defaultRegistry.ObserveTTLCachePurge(cache, purged)
}

func ObserveTTLCacheSize(cache string, size int) {
	defaultRegistry.ObserveTTLCacheSize(cache, size)
}

func ObserveMomentumSignal(symbol, side string) {
	defaultRegistry.ObserveMomentumSignal(symbol, side)
}

func ObserveMomentumEvaluation(symbol, outcome string, duration time.Duration) {
	defaultRegistry.ObserveMomentumEvaluation(symbol, outcome, duration)
}

func ObserveMarketIngest(venue, result string) {
	defaultRegistry.ObserveMarketIngest(venue, result)
}

// ObserveMarketLatency records the end-to-end latency of a market data tick in milliseconds.
func ObserveMarketLatency(venue, symbol string, latencyMS float64) {
	defaultRegistry.ObserveMarketLatency(venue, symbol, latencyMS)
}

// SetMarketPrice sets the latest observed market price gauge for venue/symbol/side.
func SetMarketPrice(venue, symbol, side string, price float64) {
	defaultRegistry.SetMarketPrice(venue, symbol, side, price)
}

// SetMarketWSConnected sets the WebSocket connection status gauge for a venue.
func SetMarketWSConnected(venue string, connected bool) {
	defaultRegistry.SetMarketWSConnected(venue, connected)
}

// ObserveMarketWSReconnect increments the WebSocket reconnection counter for a venue.
func ObserveMarketWSReconnect(venue string) {
	defaultRegistry.ObserveMarketWSReconnect(venue)
}

// ObserveMarketTickRate increments the market tick counter for a venue/symbol pair.
func ObserveMarketTickRate(venue, symbol string) {
	defaultRegistry.ObserveMarketTickRate(venue, symbol)
}

// ExposeGoRuntime collects Go runtime metrics and returns them in Prometheus text format.
func ExposeGoRuntime() string {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	var gcPauseSumNs uint64
	for _, p := range m.PauseNs {
		gcPauseSumNs += p
	}
	var lastPauseNs uint64
	if m.NumGC > 0 {
		lastPauseNs = m.PauseNs[(m.NumGC-1)%256]
	}

	// Go stdlib does not expose the OS thread count directly.
	// We report GOMAXPROCS as the schedulable thread count.
	numThreads := runtime.GOMAXPROCS(0)

	uptimeSeconds := time.Since(startTime).Seconds()

	var b strings.Builder

	b.WriteString("# HELP go_goroutines Number of goroutines that currently exist.\n")
	b.WriteString("# TYPE go_goroutines gauge\n")
	b.WriteString(fmt.Sprintf("go_goroutines %d\n", runtime.NumGoroutine()))

	b.WriteString("# HELP go_memstats_heap_alloc_bytes Number of heap bytes allocated and still in use.\n")
	b.WriteString("# TYPE go_memstats_heap_alloc_bytes gauge\n")
	b.WriteString(fmt.Sprintf("go_memstats_heap_alloc_bytes %d\n", m.HeapAlloc))

	b.WriteString("# HELP go_memstats_heap_inuse_bytes Number of heap bytes in use.\n")
	b.WriteString("# TYPE go_memstats_heap_inuse_bytes gauge\n")
	b.WriteString(fmt.Sprintf("go_memstats_heap_inuse_bytes %d\n", m.HeapInuse))

	b.WriteString("# HELP go_memstats_heap_sys_bytes Number of heap bytes obtained from system.\n")
	b.WriteString("# TYPE go_memstats_heap_sys_bytes gauge\n")
	b.WriteString(fmt.Sprintf("go_memstats_heap_sys_bytes %d\n", m.HeapSys))

	b.WriteString("# HELP go_memstats_stack_inuse_bytes Number of bytes in use by the stack allocator.\n")
	b.WriteString("# TYPE go_memstats_stack_inuse_bytes gauge\n")
	b.WriteString(fmt.Sprintf("go_memstats_stack_inuse_bytes %d\n", m.StackInuse))

	b.WriteString("# HELP go_memstats_sys_bytes Number of bytes obtained from system.\n")
	b.WriteString("# TYPE go_memstats_sys_bytes gauge\n")
	b.WriteString(fmt.Sprintf("go_memstats_sys_bytes %d\n", m.Sys))

	b.WriteString("# HELP go_gc_cycles_total Total number of completed GC cycles.\n")
	b.WriteString("# TYPE go_gc_cycles_total counter\n")
	b.WriteString(fmt.Sprintf("go_gc_cycles_total %d\n", m.NumGC))

	b.WriteString("# HELP go_gc_pause_seconds_sum Cumulative GC pause duration in seconds.\n")
	b.WriteString("# TYPE go_gc_pause_seconds_sum counter\n")
	b.WriteString(fmt.Sprintf("go_gc_pause_seconds_sum %f\n", float64(gcPauseSumNs)/1e9))

	b.WriteString("# HELP go_gc_pause_seconds_last Duration of the most recent GC pause in seconds.\n")
	b.WriteString("# TYPE go_gc_pause_seconds_last gauge\n")
	b.WriteString(fmt.Sprintf("go_gc_pause_seconds_last %f\n", float64(lastPauseNs)/1e9))

	b.WriteString("# HELP go_threads Number of OS threads created.\n")
	b.WriteString("# TYPE go_threads gauge\n")
	b.WriteString(fmt.Sprintf("go_threads %d\n", numThreads))

	b.WriteString("# HELP process_uptime_seconds Time since process start in seconds.\n")
	b.WriteString("# TYPE process_uptime_seconds gauge\n")
	b.WriteString(fmt.Sprintf("process_uptime_seconds %f\n", uptimeSeconds))

	return b.String()
}

func ExposePrometheus() string {
	return defaultRegistry.ExposePrometheus()
}

func ResetForTest() {
	defaultRegistry = NewRegistry()
}

type requestKey struct {
	method string
	path   string
	status string
}

type latencyKey struct {
	method string
	path   string
}

type ttlGetKey struct {
	cache  string
	result string
}

type ttlEvictKey struct {
	cache  string
	reason string
}

type momentumEvalKey struct {
	symbol  string
	outcome string
}

type momentumSignalKey struct {
	symbol string
	side   string
}

type momentumLatencyKey struct {
	symbol string
}

type marketIngestKey struct {
	venue  string
	result string
}

type marketKey struct {
	venue  string
	symbol string
}

type marketPriceKey struct {
	venue  string
	symbol string
	side   string
}

type executionGatewayKey struct {
	operation string
	result    string
}

type histogram struct {
	buckets []float64
	counts  []uint64
	count   uint64
	sum     float64
}

type Registry struct {
	mu sync.RWMutex

	requests  map[requestKey]uint64
	histogram map[latencyKey]*histogram

	riskDecision map[string]uint64
	riskLatency  *histogram

	execSubmit  map[string]uint64
	execLatency *histogram
	execGateway map[executionGatewayKey]uint64

	ttlGet   map[ttlGetKey]uint64
	ttlEvict map[ttlEvictKey]uint64
	ttlPurge map[string]uint64
	ttlSize  map[string]int

	momentumEval    map[momentumEvalKey]uint64
	momentumLatency map[momentumLatencyKey]*histogram
	momentumSignals map[momentumSignalKey]uint64
	marketIngest    map[marketIngestKey]uint64

	marketLatency      map[marketKey]*histogram
	marketPrice        map[marketPriceKey]float64
	marketWSConnected  map[string]float64
	marketWSReconnects map[string]uint64
	marketTickCount    map[marketKey]uint64
}

func NewRegistry() *Registry {
	return &Registry{
		requests:     make(map[requestKey]uint64),
		histogram:    make(map[latencyKey]*histogram),
		riskDecision: make(map[string]uint64),
		riskLatency: &histogram{
			buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 25, 50},
			counts:  make([]uint64, 9),
		},
		execSubmit: make(map[string]uint64),
		execLatency: &histogram{
			buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 25, 50},
			counts:  make([]uint64, 9),
		},
		execGateway:     make(map[executionGatewayKey]uint64),
		ttlGet:          make(map[ttlGetKey]uint64),
		ttlEvict:        make(map[ttlEvictKey]uint64),
		ttlPurge:        make(map[string]uint64),
		ttlSize:         make(map[string]int),
		momentumEval:    make(map[momentumEvalKey]uint64),
		momentumLatency: make(map[momentumLatencyKey]*histogram),
		momentumSignals: make(map[momentumSignalKey]uint64),
		marketIngest:    make(map[marketIngestKey]uint64),

		marketLatency:      make(map[marketKey]*histogram),
		marketPrice:        make(map[marketPriceKey]float64),
		marketWSConnected:  make(map[string]float64),
		marketWSReconnects: make(map[string]uint64),
		marketTickCount:    make(map[marketKey]uint64),
	}
}

func (r *Registry) ObserveHTTP(method, path string, status int, duration time.Duration) {
	method = strings.ToUpper(strings.TrimSpace(method))
	path = strings.TrimSpace(path)
	if method == "" {
		method = "UNKNOWN"
	}
	if path == "" {
		path = "/unknown"
	}

	key := requestKey{
		method: method,
		path:   path,
		status: fmt.Sprintf("%d", status),
	}

	latency := latencyKey{
		method: method,
		path:   path,
	}

	ms := float64(duration.Microseconds()) / 1000.0

	r.mu.Lock()
	defer r.mu.Unlock()

	r.requests[key]++

	h, ok := r.histogram[latency]
	if !ok {
		h = &histogram{
			buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000},
			counts:  make([]uint64, 10),
		}
		r.histogram[latency] = h
	}

	observeHistogram(h, ms)
}

func (r *Registry) ObserveRiskEvaluation(decision string, duration time.Duration) {
	decision = strings.ToLower(strings.TrimSpace(decision))
	if decision == "" {
		decision = "unknown"
	}
	ms := float64(duration.Microseconds()) / 1000.0

	r.mu.Lock()
	defer r.mu.Unlock()

	r.riskDecision[decision]++
	observeHistogram(r.riskLatency, ms)
}

func (r *Registry) ObserveExecutionSubmit(outcome string, duration time.Duration) {
	outcome = strings.ToLower(strings.TrimSpace(outcome))
	if outcome == "" {
		outcome = "unknown"
	}
	ms := float64(duration.Microseconds()) / 1000.0

	r.mu.Lock()
	defer r.mu.Unlock()

	r.execSubmit[outcome]++
	observeHistogram(r.execLatency, ms)
}

func (r *Registry) ObserveExecutionGateway(operation, result string) {
	operation = strings.ToLower(strings.TrimSpace(operation))
	if operation == "" {
		operation = "unknown"
	}
	result = strings.ToLower(strings.TrimSpace(result))
	if result == "" {
		result = "unknown"
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.execGateway[executionGatewayKey{operation: operation, result: result}]++
}

func (r *Registry) ObserveTTLCacheGet(cache string, hit bool) {
	cache = normalizeCacheName(cache)
	result := "miss"
	if hit {
		result = "hit"
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.ttlGet[ttlGetKey{cache: cache, result: result}]++
}

func (r *Registry) ObserveTTLCacheEviction(cache, reason string) {
	cache = normalizeCacheName(cache)
	reason = strings.ToLower(strings.TrimSpace(reason))
	if reason == "" {
		reason = "unknown"
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.ttlEvict[ttlEvictKey{cache: cache, reason: reason}]++
}

func (r *Registry) ObserveTTLCachePurge(cache string, purged int) {
	if purged <= 0 {
		return
	}
	cache = normalizeCacheName(cache)

	r.mu.Lock()
	defer r.mu.Unlock()

	r.ttlPurge[cache] += uint64(purged)
}

func (r *Registry) ObserveTTLCacheSize(cache string, size int) {
	if size < 0 {
		size = 0
	}
	cache = normalizeCacheName(cache)

	r.mu.Lock()
	defer r.mu.Unlock()

	r.ttlSize[cache] = size
}

func (r *Registry) ObserveMomentumSignal(symbol, side string) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		symbol = "UNKNOWN"
	}
	side = strings.ToLower(strings.TrimSpace(side))
	if side == "" {
		side = "unknown"
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.momentumSignals[momentumSignalKey{symbol: symbol, side: side}]++
}

func (r *Registry) ObserveMomentumEvaluation(symbol, outcome string, duration time.Duration) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		symbol = "UNKNOWN"
	}
	outcome = strings.ToLower(strings.TrimSpace(outcome))
	if outcome == "" {
		outcome = "unknown"
	}
	ms := float64(duration.Microseconds()) / 1000.0

	r.mu.Lock()
	defer r.mu.Unlock()

	r.momentumEval[momentumEvalKey{symbol: symbol, outcome: outcome}]++

	key := momentumLatencyKey{symbol: symbol}
	h, ok := r.momentumLatency[key]
	if !ok {
		h = &histogram{
			buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 25, 50},
			counts:  make([]uint64, 9),
		}
		r.momentumLatency[key] = h
	}
	observeHistogram(h, ms)
}

func (r *Registry) ObserveMarketIngest(venue, result string) {
	venue = strings.ToLower(strings.TrimSpace(venue))
	if venue == "" {
		venue = "unknown"
	}
	result = strings.ToLower(strings.TrimSpace(result))
	if result == "" {
		result = "unknown"
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.marketIngest[marketIngestKey{venue: venue, result: result}]++
}

func (r *Registry) ObserveMarketLatency(venue, symbol string, latencyMS float64) {
	venue = strings.ToLower(strings.TrimSpace(venue))
	if venue == "" {
		venue = "unknown"
	}
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		symbol = "UNKNOWN"
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := marketKey{venue: venue, symbol: symbol}
	h, ok := r.marketLatency[key]
	if !ok {
		h = &histogram{
			buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2000, 5000},
			counts:  make([]uint64, 12),
		}
		r.marketLatency[key] = h
	}
	observeHistogram(h, latencyMS)
}

func (r *Registry) SetMarketPrice(venue, symbol, side string, price float64) {
	venue = strings.ToLower(strings.TrimSpace(venue))
	if venue == "" {
		venue = "unknown"
	}
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		symbol = "UNKNOWN"
	}
	side = strings.ToLower(strings.TrimSpace(side))
	if side == "" {
		side = "unknown"
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.marketPrice[marketPriceKey{venue: venue, symbol: symbol, side: side}] = price
}

func (r *Registry) SetMarketWSConnected(venue string, connected bool) {
	venue = strings.ToLower(strings.TrimSpace(venue))
	if venue == "" {
		venue = "unknown"
	}

	val := 0.0
	if connected {
		val = 1.0
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.marketWSConnected[venue] = val
}

func (r *Registry) ObserveMarketWSReconnect(venue string) {
	venue = strings.ToLower(strings.TrimSpace(venue))
	if venue == "" {
		venue = "unknown"
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.marketWSReconnects[venue]++
}

func (r *Registry) ObserveMarketTickRate(venue, symbol string) {
	venue = strings.ToLower(strings.TrimSpace(venue))
	if venue == "" {
		venue = "unknown"
	}
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		symbol = "UNKNOWN"
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.marketTickCount[marketKey{venue: venue, symbol: symbol}]++
}

func (r *Registry) ExposePrometheus() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var b strings.Builder

	b.WriteString("# HELP engine_controlapi_http_requests_total Total HTTP requests handled by controlapi.\n")
	b.WriteString("# TYPE engine_controlapi_http_requests_total counter\n")

	reqKeys := make([]requestKey, 0, len(r.requests))
	for k := range r.requests {
		reqKeys = append(reqKeys, k)
	}
	sort.Slice(reqKeys, func(i, j int) bool {
		a, c := reqKeys[i], reqKeys[j]
		if a.method != c.method {
			return a.method < c.method
		}
		if a.path != c.path {
			return a.path < c.path
		}
		return a.status < c.status
	})

	for _, key := range reqKeys {
		count := r.requests[key]
		b.WriteString(fmt.Sprintf(
			`engine_controlapi_http_requests_total{method="%s",path="%s",status="%s"} %d`+"\n",
			escapeLabel(key.method),
			escapeLabel(key.path),
			escapeLabel(key.status),
			count,
		))
	}

	b.WriteString("# HELP engine_controlapi_http_request_duration_ms HTTP request duration in milliseconds.\n")
	b.WriteString("# TYPE engine_controlapi_http_request_duration_ms histogram\n")

	latKeys := make([]latencyKey, 0, len(r.histogram))
	for k := range r.histogram {
		latKeys = append(latKeys, k)
	}
	sort.Slice(latKeys, func(i, j int) bool {
		a, c := latKeys[i], latKeys[j]
		if a.method != c.method {
			return a.method < c.method
		}
		return a.path < c.path
	})

	for _, key := range latKeys {
		h := r.histogram[key]
		writeHistogram(
			&b,
			"engine_controlapi_http_request_duration_ms",
			map[string]string{
				"method": key.method,
				"path":   key.path,
			},
			h,
		)
	}

	b.WriteString("# HELP engine_risk_decision_total Total risk decisions by decision type.\n")
	b.WriteString("# TYPE engine_risk_decision_total counter\n")
	decisionKeys := make([]string, 0, len(r.riskDecision))
	for k := range r.riskDecision {
		decisionKeys = append(decisionKeys, k)
	}
	sort.Strings(decisionKeys)
	for _, key := range decisionKeys {
		b.WriteString(fmt.Sprintf(
			`engine_risk_decision_total{decision="%s"} %d`+"\n",
			escapeLabel(key),
			r.riskDecision[key],
		))
	}

	b.WriteString("# HELP engine_risk_eval_duration_ms Risk evaluation duration in milliseconds.\n")
	b.WriteString("# TYPE engine_risk_eval_duration_ms histogram\n")
	writeHistogram(&b, "engine_risk_eval_duration_ms", map[string]string{}, r.riskLatency)

	b.WriteString("# HELP engine_execution_submit_total Total execution submit outcomes.\n")
	b.WriteString("# TYPE engine_execution_submit_total counter\n")
	execKeys := make([]string, 0, len(r.execSubmit))
	for k := range r.execSubmit {
		execKeys = append(execKeys, k)
	}
	sort.Strings(execKeys)
	for _, key := range execKeys {
		b.WriteString(fmt.Sprintf(
			`engine_execution_submit_total{outcome="%s"} %d`+"\n",
			escapeLabel(key),
			r.execSubmit[key],
		))
	}

	b.WriteString("# HELP engine_execution_submit_duration_ms Execution submit duration in milliseconds.\n")
	b.WriteString("# TYPE engine_execution_submit_duration_ms histogram\n")
	writeHistogram(&b, "engine_execution_submit_duration_ms", map[string]string{}, r.execLatency)

	b.WriteString("# HELP engine_execution_gateway_events_total Execution gateway events by operation and result.\n")
	b.WriteString("# TYPE engine_execution_gateway_events_total counter\n")
	execGatewayKeys := make([]executionGatewayKey, 0, len(r.execGateway))
	for k := range r.execGateway {
		execGatewayKeys = append(execGatewayKeys, k)
	}
	sort.Slice(execGatewayKeys, func(i, j int) bool {
		a, c := execGatewayKeys[i], execGatewayKeys[j]
		if a.operation != c.operation {
			return a.operation < c.operation
		}
		return a.result < c.result
	})
	for _, key := range execGatewayKeys {
		b.WriteString(fmt.Sprintf(
			`engine_execution_gateway_events_total{operation="%s",result="%s"} %d`+"\n",
			escapeLabel(key.operation),
			escapeLabel(key.result),
			r.execGateway[key],
		))
	}

	b.WriteString("# HELP engine_ttlcache_get_total TTL cache get operations by result.\n")
	b.WriteString("# TYPE engine_ttlcache_get_total counter\n")
	ttlGetKeys := make([]ttlGetKey, 0, len(r.ttlGet))
	for k := range r.ttlGet {
		ttlGetKeys = append(ttlGetKeys, k)
	}
	sort.Slice(ttlGetKeys, func(i, j int) bool {
		a, c := ttlGetKeys[i], ttlGetKeys[j]
		if a.cache != c.cache {
			return a.cache < c.cache
		}
		return a.result < c.result
	})
	for _, key := range ttlGetKeys {
		b.WriteString(fmt.Sprintf(
			`engine_ttlcache_get_total{cache="%s",result="%s"} %d`+"\n",
			escapeLabel(key.cache),
			escapeLabel(key.result),
			r.ttlGet[key],
		))
	}

	b.WriteString("# HELP engine_ttlcache_eviction_total TTL cache evictions by reason.\n")
	b.WriteString("# TYPE engine_ttlcache_eviction_total counter\n")
	ttlEvictKeys := make([]ttlEvictKey, 0, len(r.ttlEvict))
	for k := range r.ttlEvict {
		ttlEvictKeys = append(ttlEvictKeys, k)
	}
	sort.Slice(ttlEvictKeys, func(i, j int) bool {
		a, c := ttlEvictKeys[i], ttlEvictKeys[j]
		if a.cache != c.cache {
			return a.cache < c.cache
		}
		return a.reason < c.reason
	})
	for _, key := range ttlEvictKeys {
		b.WriteString(fmt.Sprintf(
			`engine_ttlcache_eviction_total{cache="%s",reason="%s"} %d`+"\n",
			escapeLabel(key.cache),
			escapeLabel(key.reason),
			r.ttlEvict[key],
		))
	}

	b.WriteString("# HELP engine_ttlcache_purge_total TTL cache purged entries total.\n")
	b.WriteString("# TYPE engine_ttlcache_purge_total counter\n")
	ttlPurgeKeys := make([]string, 0, len(r.ttlPurge))
	for k := range r.ttlPurge {
		ttlPurgeKeys = append(ttlPurgeKeys, k)
	}
	sort.Strings(ttlPurgeKeys)
	for _, key := range ttlPurgeKeys {
		b.WriteString(fmt.Sprintf(
			`engine_ttlcache_purge_total{cache="%s"} %d`+"\n",
			escapeLabel(key),
			r.ttlPurge[key],
		))
	}

	b.WriteString("# HELP engine_ttlcache_size Current TTL cache size.\n")
	b.WriteString("# TYPE engine_ttlcache_size gauge\n")
	ttlSizeKeys := make([]string, 0, len(r.ttlSize))
	for k := range r.ttlSize {
		ttlSizeKeys = append(ttlSizeKeys, k)
	}
	sort.Strings(ttlSizeKeys)
	for _, key := range ttlSizeKeys {
		b.WriteString(fmt.Sprintf(
			`engine_ttlcache_size{cache="%s"} %d`+"\n",
			escapeLabel(key),
			r.ttlSize[key],
		))
	}

	b.WriteString("# HELP engine_strategy_momentum_eval_total Momentum strategy evaluations by outcome.\n")
	b.WriteString("# TYPE engine_strategy_momentum_eval_total counter\n")
	momentumEvalKeys := make([]momentumEvalKey, 0, len(r.momentumEval))
	for k := range r.momentumEval {
		momentumEvalKeys = append(momentumEvalKeys, k)
	}
	sort.Slice(momentumEvalKeys, func(i, j int) bool {
		a, c := momentumEvalKeys[i], momentumEvalKeys[j]
		if a.symbol != c.symbol {
			return a.symbol < c.symbol
		}
		return a.outcome < c.outcome
	})
	for _, key := range momentumEvalKeys {
		b.WriteString(fmt.Sprintf(
			`engine_strategy_momentum_eval_total{symbol="%s",outcome="%s"} %d`+"\n",
			escapeLabel(key.symbol),
			escapeLabel(key.outcome),
			r.momentumEval[key],
		))
	}

	b.WriteString("# HELP engine_strategy_momentum_eval_duration_ms Momentum strategy evaluation duration in milliseconds.\n")
	b.WriteString("# TYPE engine_strategy_momentum_eval_duration_ms histogram\n")
	momentumLatencyKeys := make([]momentumLatencyKey, 0, len(r.momentumLatency))
	for k := range r.momentumLatency {
		momentumLatencyKeys = append(momentumLatencyKeys, k)
	}
	sort.Slice(momentumLatencyKeys, func(i, j int) bool {
		return momentumLatencyKeys[i].symbol < momentumLatencyKeys[j].symbol
	})
	for _, key := range momentumLatencyKeys {
		writeHistogram(
			&b,
			"engine_strategy_momentum_eval_duration_ms",
			map[string]string{"symbol": key.symbol},
			r.momentumLatency[key],
		)
	}

	b.WriteString("# HELP engine_strategy_momentum_signal_total Momentum strategy emitted signals by side.\n")
	b.WriteString("# TYPE engine_strategy_momentum_signal_total counter\n")
	momentumSignalKeys := make([]momentumSignalKey, 0, len(r.momentumSignals))
	for k := range r.momentumSignals {
		momentumSignalKeys = append(momentumSignalKeys, k)
	}
	sort.Slice(momentumSignalKeys, func(i, j int) bool {
		a, c := momentumSignalKeys[i], momentumSignalKeys[j]
		if a.symbol != c.symbol {
			return a.symbol < c.symbol
		}
		return a.side < c.side
	})
	for _, key := range momentumSignalKeys {
		b.WriteString(fmt.Sprintf(
			`engine_strategy_momentum_signal_total{symbol="%s",side="%s"} %d`+"\n",
			escapeLabel(key.symbol),
			escapeLabel(key.side),
			r.momentumSignals[key],
		))
	}

	b.WriteString("# HELP engine_market_ingest_events_total Market ingest events by venue and result.\n")
	b.WriteString("# TYPE engine_market_ingest_events_total counter\n")
	marketIngestKeys := make([]marketIngestKey, 0, len(r.marketIngest))
	for k := range r.marketIngest {
		marketIngestKeys = append(marketIngestKeys, k)
	}
	sort.Slice(marketIngestKeys, func(i, j int) bool {
		a, c := marketIngestKeys[i], marketIngestKeys[j]
		if a.venue != c.venue {
			return a.venue < c.venue
		}
		return a.result < c.result
	})
	for _, key := range marketIngestKeys {
		b.WriteString(fmt.Sprintf(
			`engine_market_ingest_events_total{venue="%s",result="%s"} %d`+"\n",
			escapeLabel(key.venue),
			escapeLabel(key.result),
			r.marketIngest[key],
		))
	}

	// --- Market latency histogram ---
	b.WriteString("# HELP engine_market_latency_ms End-to-end market data latency in milliseconds.\n")
	b.WriteString("# TYPE engine_market_latency_ms histogram\n")
	mLatKeys := make([]marketKey, 0, len(r.marketLatency))
	for k := range r.marketLatency {
		mLatKeys = append(mLatKeys, k)
	}
	sort.Slice(mLatKeys, func(i, j int) bool {
		a, c := mLatKeys[i], mLatKeys[j]
		if a.venue != c.venue {
			return a.venue < c.venue
		}
		return a.symbol < c.symbol
	})
	for _, key := range mLatKeys {
		writeHistogram(
			&b,
			"engine_market_latency_ms",
			map[string]string{"venue": key.venue, "symbol": key.symbol},
			r.marketLatency[key],
		)
	}

	// --- Market price gauge ---
	b.WriteString("# HELP engine_market_price Latest observed market price.\n")
	b.WriteString("# TYPE engine_market_price gauge\n")
	mPriceKeys := make([]marketPriceKey, 0, len(r.marketPrice))
	for k := range r.marketPrice {
		mPriceKeys = append(mPriceKeys, k)
	}
	sort.Slice(mPriceKeys, func(i, j int) bool {
		a, c := mPriceKeys[i], mPriceKeys[j]
		if a.venue != c.venue {
			return a.venue < c.venue
		}
		if a.symbol != c.symbol {
			return a.symbol < c.symbol
		}
		return a.side < c.side
	})
	for _, key := range mPriceKeys {
		b.WriteString(fmt.Sprintf(
			`engine_market_price{venue="%s",symbol="%s",side="%s"} %f`+"\n",
			escapeLabel(key.venue),
			escapeLabel(key.symbol),
			escapeLabel(key.side),
			r.marketPrice[key],
		))
	}

	// --- Market WS connected gauge ---
	b.WriteString("# HELP engine_market_ws_connected WebSocket connection status (1=connected, 0=disconnected).\n")
	b.WriteString("# TYPE engine_market_ws_connected gauge\n")
	wsConnKeys := make([]string, 0, len(r.marketWSConnected))
	for k := range r.marketWSConnected {
		wsConnKeys = append(wsConnKeys, k)
	}
	sort.Strings(wsConnKeys)
	for _, key := range wsConnKeys {
		b.WriteString(fmt.Sprintf(
			`engine_market_ws_connected{venue="%s"} %g`+"\n",
			escapeLabel(key),
			r.marketWSConnected[key],
		))
	}

	// --- Market WS reconnects counter ---
	b.WriteString("# HELP engine_market_ws_reconnects_total Total WebSocket reconnection attempts.\n")
	b.WriteString("# TYPE engine_market_ws_reconnects_total counter\n")
	wsReconnKeys := make([]string, 0, len(r.marketWSReconnects))
	for k := range r.marketWSReconnects {
		wsReconnKeys = append(wsReconnKeys, k)
	}
	sort.Strings(wsReconnKeys)
	for _, key := range wsReconnKeys {
		b.WriteString(fmt.Sprintf(
			`engine_market_ws_reconnects_total{venue="%s"} %d`+"\n",
			escapeLabel(key),
			r.marketWSReconnects[key],
		))
	}

	// --- Market tick count counter ---
	b.WriteString("# HELP engine_market_tick_total Total market data ticks received.\n")
	b.WriteString("# TYPE engine_market_tick_total counter\n")
	mTickKeys := make([]marketKey, 0, len(r.marketTickCount))
	for k := range r.marketTickCount {
		mTickKeys = append(mTickKeys, k)
	}
	sort.Slice(mTickKeys, func(i, j int) bool {
		a, c := mTickKeys[i], mTickKeys[j]
		if a.venue != c.venue {
			return a.venue < c.venue
		}
		return a.symbol < c.symbol
	})
	for _, key := range mTickKeys {
		b.WriteString(fmt.Sprintf(
			`engine_market_tick_total{venue="%s",symbol="%s"} %d`+"\n",
			escapeLabel(key.venue),
			escapeLabel(key.symbol),
			r.marketTickCount[key],
		))
	}

	// --- Go runtime metrics ---
	b.WriteString(ExposeGoRuntime())

	return b.String()
}

func observeHistogram(h *histogram, value float64) {
	for i, le := range h.buckets {
		if value <= le {
			h.counts[i]++
		}
	}
	h.counts[len(h.counts)-1]++
	h.count++
	h.sum += value
}

func writeHistogram(b *strings.Builder, metric string, labels map[string]string, h *histogram) {
	for i, le := range h.buckets {
		b.WriteString(fmt.Sprintf(
			`%s_bucket%s %d`+"\n",
			metric,
			withLELabel(labels, fmt.Sprintf("%g", le)),
			h.counts[i],
		))
	}
	b.WriteString(fmt.Sprintf(
		`%s_bucket%s %d`+"\n",
		metric,
		withLELabel(labels, "+Inf"),
		h.counts[len(h.counts)-1],
	))
	b.WriteString(fmt.Sprintf(`%s_sum%s %f`+"\n", metric, withLabels(labels), h.sum))
	b.WriteString(fmt.Sprintf(`%s_count%s %d`+"\n", metric, withLabels(labels), h.count))
}

func withLELabel(labels map[string]string, le string) string {
	out := make(map[string]string, len(labels)+1)
	for k, v := range labels {
		out[k] = v
	}
	out["le"] = le
	return withLabels(out)
}

func withLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf(`%s="%s"`, key, escapeLabel(labels[key])))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func escapeLabel(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `"`, `\"`)
	return v
}

func normalizeCacheName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return "default"
	}
	return name
}
