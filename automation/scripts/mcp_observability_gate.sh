#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

PROM_URL="${PROM_URL:-http://127.0.0.1:9090}"
GRAFANA_URL="${GRAFANA_URL:-http://127.0.0.1:3000}"
GRAFANA_USER="${GRAFANA_USER:-admin}"
GRAFANA_PASS="${GRAFANA_PASS:-admin}"

MIN_ENGINE_CORE_UP="${MIN_ENGINE_CORE_UP:-1}"
MIN_STRATEGY_RUNNER_UP="${MIN_STRATEGY_RUNNER_UP:-1}"
MIN_MARKET_INGEST_UP="${MIN_MARKET_INGEST_UP:-1}"
MIN_NATS_UP="${MIN_NATS_UP:-1}"
MIN_MYSQL_UP="${MIN_MYSQL_UP:-1}"
MAX_CRITICAL_FIRING="${MAX_CRITICAL_FIRING:-0}"
MAX_WARNING_FIRING="${MAX_WARNING_FIRING:-5}"

TS="$(date +%Y%m%d-%H%M%S)"
EVIDENCE_DIR="${EVIDENCE_DIR:-${ROOT_DIR}/automation/reports/mcp-gate-${TS}}"
SUMMARY_FILE="${EVIDENCE_DIR}/summary.md"
RESULT_FILE="${EVIDENCE_DIR}/result.json"
ENV_FILE="${EVIDENCE_DIR}/env.txt"
GRAFANA_LOG="${EVIDENCE_DIR}/verify-grafana-data.log"

mkdir -p "${EVIDENCE_DIR}"

require_cmd() {
  local cmd="$1"
  if ! command -v "${cmd}" >/dev/null 2>&1; then
    echo "[mcp-gate] error: required command not found: ${cmd}" >&2
    exit 1
  fi
}

float_ge() {
  local a="$1"
  local b="$2"
  awk -v a="${a}" -v b="${b}" 'BEGIN { exit !((a+0) >= (b+0)) }'
}

float_le() {
  local a="$1"
  local b="$2"
  awk -v a="${a}" -v b="${b}" 'BEGIN { exit !((a+0) <= (b+0)) }'
}

prom_scalar() {
  local query="$1"
  local value
  value="$(curl -fsS "${PROM_URL}/api/v1/query" --get --data-urlencode "query=${query}" 2>/dev/null | jq -r '.data.result[0].value[1] // empty' 2>/dev/null || true)"
  if [[ -z "${value}" ]]; then
    return 1
  fi
  echo "${value}"
}

status="pass"
failures=0
failure_notes=""

record_failure() {
  local msg="$1"
  failures=$((failures + 1))
  status="fail"
  failure_notes+="- ${msg}"$'\n'
  echo "[mcp-gate] FAIL ${msg}"
}

record_ok() {
  local msg="$1"
  echo "[mcp-gate] OK   ${msg}"
}

require_cmd curl
require_cmd jq
require_cmd awk

{
  echo "generated_at=$(date '+%Y-%m-%d %H:%M:%S %z')"
  echo "cwd=${ROOT_DIR}"
  echo "evidence_dir=${EVIDENCE_DIR}"
  echo "prom_url=${PROM_URL}"
  echo "grafana_url=${GRAFANA_URL}"
  echo "kube_context=$(kubectl config current-context 2>/dev/null || echo N/A)"
  env | sort
} > "${ENV_FILE}"

echo "[mcp-gate] step=verify_grafana_data"
if automation/scripts/verify_grafana_data.sh > "${GRAFANA_LOG}" 2>&1; then
  record_ok "verify_grafana_data.sh"
else
  record_failure "verify_grafana_data.sh failed (see ${GRAFANA_LOG})"
fi

engine_core_up="$(prom_scalar 'max(up{job="engine-core"})' || echo 0)"
strategy_runner_up="$(prom_scalar 'max(up{job="strategy-runner"})' || echo 0)"
market_ingest_up="$(prom_scalar 'max(up{namespace="quant-system",service="market-ingest"} or up{namespace="quant-system",job=~".*market-ingest.*"} or count(container_start_time_seconds{namespace="quant-system",pod=~"market-ingest-.*",container="market-ingest"})) or vector(0)' || echo 0)"
nats_up="$(prom_scalar 'max(up{job="blackbox-nats-health"} or count(container_start_time_seconds{namespace="quant-system",pod=~"nats-.*",container="nats"})) or vector(0)' || echo 0)"
mysql_up="$(prom_scalar 'max(up{job="blackbox-mysql-tcp"} or count(container_start_time_seconds{namespace="quant-system",pod=~"mysql-.*",container="mysql"})) or vector(0)' || echo 0)"
critical_firing="$(prom_scalar 'sum(ALERTS{alertstate="firing",severity="critical"})' || echo 0)"
warning_firing="$(prom_scalar 'sum(ALERTS{alertstate="firing",severity="warning"})' || echo 0)"

if ! float_ge "${engine_core_up}" "${MIN_ENGINE_CORE_UP}"; then
  record_failure "engine-core-up=${engine_core_up} < ${MIN_ENGINE_CORE_UP}"
else
  record_ok "engine-core-up=${engine_core_up}"
fi

if ! float_ge "${strategy_runner_up}" "${MIN_STRATEGY_RUNNER_UP}"; then
  record_failure "strategy-runner-up=${strategy_runner_up} < ${MIN_STRATEGY_RUNNER_UP}"
else
  record_ok "strategy-runner-up=${strategy_runner_up}"
fi

if ! float_ge "${market_ingest_up}" "${MIN_MARKET_INGEST_UP}"; then
  record_failure "market-ingest-up=${market_ingest_up} < ${MIN_MARKET_INGEST_UP}"
else
  record_ok "market-ingest-up=${market_ingest_up}"
fi

if ! float_ge "${nats_up}" "${MIN_NATS_UP}"; then
  record_failure "nats-up=${nats_up} < ${MIN_NATS_UP}"
else
  record_ok "nats-up=${nats_up}"
fi

if ! float_ge "${mysql_up}" "${MIN_MYSQL_UP}"; then
  record_failure "mysql-up=${mysql_up} < ${MIN_MYSQL_UP}"
else
  record_ok "mysql-up=${mysql_up}"
fi

if ! float_le "${critical_firing}" "${MAX_CRITICAL_FIRING}"; then
  record_failure "critical_firing=${critical_firing} > ${MAX_CRITICAL_FIRING}"
else
  record_ok "critical_firing=${critical_firing}"
fi

if ! float_le "${warning_firing}" "${MAX_WARNING_FIRING}"; then
  record_failure "warning_firing=${warning_firing} > ${MAX_WARNING_FIRING}"
else
  record_ok "warning_firing=${warning_firing}"
fi

jq -n \
  --arg status "${status}" \
  --arg generated_at "$(date '+%Y-%m-%d %H:%M:%S %z')" \
  --arg evidence_dir "${EVIDENCE_DIR}" \
  --arg prom_url "${PROM_URL}" \
  --arg grafana_url "${GRAFANA_URL}" \
  --arg kube_context "$(kubectl config current-context 2>/dev/null || echo N/A)" \
  --argjson failures "${failures}" \
  --arg engine_core_up "${engine_core_up}" \
  --arg strategy_runner_up "${strategy_runner_up}" \
  --arg market_ingest_up "${market_ingest_up}" \
  --arg nats_up "${nats_up}" \
  --arg mysql_up "${mysql_up}" \
  --arg critical_firing "${critical_firing}" \
  --arg warning_firing "${warning_firing}" \
  --arg min_engine_core_up "${MIN_ENGINE_CORE_UP}" \
  --arg min_strategy_runner_up "${MIN_STRATEGY_RUNNER_UP}" \
  --arg min_market_ingest_up "${MIN_MARKET_INGEST_UP}" \
  --arg min_nats_up "${MIN_NATS_UP}" \
  --arg min_mysql_up "${MIN_MYSQL_UP}" \
  --arg max_critical_firing "${MAX_CRITICAL_FIRING}" \
  --arg max_warning_firing "${MAX_WARNING_FIRING}" \
  '{
    status: $status,
    generated_at: $generated_at,
    evidence_dir: $evidence_dir,
    context: {
      prom_url: $prom_url,
      grafana_url: $grafana_url,
      kube_context: $kube_context
    },
    failures: $failures,
    metrics: {
      engine_core_up: ($engine_core_up|tonumber? // $engine_core_up),
      strategy_runner_up: ($strategy_runner_up|tonumber? // $strategy_runner_up),
      market_ingest_up: ($market_ingest_up|tonumber? // $market_ingest_up),
      nats_up: ($nats_up|tonumber? // $nats_up),
      mysql_up: ($mysql_up|tonumber? // $mysql_up),
      critical_firing: ($critical_firing|tonumber? // $critical_firing),
      warning_firing: ($warning_firing|tonumber? // $warning_firing)
    },
    thresholds: {
      min_engine_core_up: ($min_engine_core_up|tonumber? // $min_engine_core_up),
      min_strategy_runner_up: ($min_strategy_runner_up|tonumber? // $min_strategy_runner_up),
      min_market_ingest_up: ($min_market_ingest_up|tonumber? // $min_market_ingest_up),
      min_nats_up: ($min_nats_up|tonumber? // $min_nats_up),
      min_mysql_up: ($min_mysql_up|tonumber? // $min_mysql_up),
      max_critical_firing: ($max_critical_firing|tonumber? // $max_critical_firing),
      max_warning_firing: ($max_warning_firing|tonumber? // $max_warning_firing)
    }
  }' > "${RESULT_FILE}"

cat > "${SUMMARY_FILE}" <<MD
# MCP Observability Gate Summary

- generated_at: $(date '+%Y-%m-%d %H:%M:%S %z')
- status: ${status}
- failures: ${failures}
- evidence_dir: ${EVIDENCE_DIR}
- result_json: ${RESULT_FILE}
- grafana_log: ${GRAFANA_LOG}

## Metrics Snapshot

1. engine-core-up: ${engine_core_up} (min ${MIN_ENGINE_CORE_UP})
2. strategy-runner-up: ${strategy_runner_up} (min ${MIN_STRATEGY_RUNNER_UP})
3. market-ingest-up: ${market_ingest_up} (min ${MIN_MARKET_INGEST_UP})
4. nats-up: ${nats_up} (min ${MIN_NATS_UP})
5. mysql-up: ${mysql_up} (min ${MIN_MYSQL_UP})
6. critical_firing: ${critical_firing} (max ${MAX_CRITICAL_FIRING})
7. warning_firing: ${warning_firing} (max ${MAX_WARNING_FIRING})

## Failures

${failure_notes:-- none}
MD

echo "[mcp-gate] summary: ${SUMMARY_FILE}"
echo "[mcp-gate] result:  ${RESULT_FILE}"

if [[ "${status}" != "pass" ]]; then
  exit 1
fi
