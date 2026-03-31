#!/usr/bin/env bash
set -euo pipefail

PROM_URL="${PROM_URL:-http://127.0.0.1:9090}"
GRAFANA_URL="${GRAFANA_URL:-http://127.0.0.1:3000}"
GRAFANA_USER="${GRAFANA_USER:-admin}"
GRAFANA_PASS="${GRAFANA_PASS:-admin}"

if ! command -v curl >/dev/null 2>&1; then
  echo "[grafana-data] error: curl is required"
  exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "[grafana-data] error: jq is required"
  exit 1
fi

failures=0

check_query_non_empty() {
  local name="$1"
  local query="$2"
  local count
  count="$(curl -sS "${PROM_URL}/api/v1/query" --get --data-urlencode "query=${query}" | jq -r '.data.result | length')"
  if [[ "${count}" == "null" || "${count}" == "" ]]; then
    echo "[grafana-data] FAIL ${name}: query result parse failed"
    failures=$((failures + 1))
    return
  fi
  if (( count <= 0 )); then
    echo "[grafana-data] FAIL ${name}: no data"
    failures=$((failures + 1))
    return
  fi
  echo "[grafana-data] OK   ${name}: series=${count}"
}

check_dashboard_exists() {
  local uid="$1"
  local found
  found="$(curl -sS -u "${GRAFANA_USER}:${GRAFANA_PASS}" "${GRAFANA_URL}/api/dashboards/uid/${uid}" | jq -r '.dashboard.uid // empty')"
  if [[ "${found}" != "${uid}" ]]; then
    echo "[grafana-data] FAIL dashboard ${uid}: not found"
    failures=$((failures + 1))
    return
  fi
  echo "[grafana-data] OK   dashboard ${uid}: present"
}

echo "[grafana-data] verify dashboards"
check_dashboard_exists "engine-core-v1"
check_dashboard_exists "engine-k8s-status-v1"

echo "[grafana-data] verify prom queries"
check_query_non_empty "engine-core-up" 'max(up{job="engine-core"})'
check_query_non_empty "strategy-runner-up" 'max(up{job="strategy-runner"})'
check_query_non_empty "market-ingest-up" 'max(up{namespace="quant-system",service="market-ingest"} or up{namespace="quant-system",job=~".*market-ingest.*"} or count(container_start_time_seconds{namespace="quant-system",pod=~"market-ingest-.*",container="market-ingest"})) or vector(0)'
check_query_non_empty "nats-up" 'max(up{job="blackbox-nats-health"} or count(container_start_time_seconds{namespace="quant-system",pod=~"nats-.*",container="nats"})) or vector(0)'
check_query_non_empty "mysql-up" 'max(up{job="blackbox-mysql-tcp"} or count(container_start_time_seconds{namespace="quant-system",pod=~"mysql-.*",container="mysql"})) or vector(0)'
check_query_non_empty "controlapi-p99" 'histogram_quantile(0.99, sum by (le) (rate(engine_controlapi_http_request_duration_ms_bucket[5m])))'
check_query_non_empty "controlapi-status-rate" 'sum by (status) (rate(engine_controlapi_http_requests_total[5m]))'
check_query_non_empty "execution-gateway-event-rate" 'sum by (operation, result) (rate(engine_execution_gateway_events_total[5m])) or vector(0)'
check_query_non_empty "ttlcache-get-rate" 'sum by (cache, result) (rate(engine_ttlcache_get_total[5m])) or vector(0)'
check_query_non_empty "ttlcache-size" 'max by (cache) (engine_ttlcache_size) or vector(0)'
check_query_non_empty "momentum-eval-rate" 'sum by (symbol, outcome) (rate(engine_strategy_momentum_eval_total[5m])) or vector(0)'
check_query_non_empty "momentum-eval-p95" 'histogram_quantile(0.95, sum by (le, symbol) (rate(engine_strategy_momentum_eval_duration_ms_bucket[5m]))) or vector(0)'
check_query_non_empty "momentum-signal-rate" 'sum by (symbol, side) (rate(engine_strategy_momentum_signal_total[5m])) or vector(0)'
check_query_non_empty "market-ingest-event-rate" 'sum by (venue, result) (rate(engine_market_ingest_events_total[5m])) or vector(0)'

if (( failures > 0 )); then
  echo "[grafana-data] FAILED checks=${failures}"
  exit 1
fi

echo "[grafana-data] all checks passed"
