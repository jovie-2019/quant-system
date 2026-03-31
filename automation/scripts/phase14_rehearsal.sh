#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

TS="$(date +%Y%m%d-%H%M%S)"
EVIDENCE_DIR="${ROOT_DIR}/automation/reports/phase14-rehearsal-${TS}"
SUMMARY_FILE="${EVIDENCE_DIR}/summary.md"
ENV_FILE="${EVIDENCE_DIR}/env.txt"

RUN_TESTS="${RUN_TESTS:-1}"
RUN_BUILD="${RUN_BUILD:-1}"
RUN_DEPLOY="${RUN_DEPLOY:-1}"
RUN_OBS="${RUN_OBS:-1}"
RUN_LOGGING="${RUN_LOGGING:-1}"
RUN_SMOKE="${RUN_SMOKE:-1}"
RUN_MCP_GATE="${RUN_MCP_GATE:-1}"
RUN_GRAFANA_GATE="${RUN_GRAFANA_GATE:-0}"
RUN_SANDBOX_TRADE_SMOKE="${RUN_SANDBOX_TRADE_SMOKE:-0}"
RUN_ROLLBACK="${RUN_ROLLBACK:-0}"
DRY_RUN="${DRY_RUN:-0}"

IMAGE_REPO="${IMAGE_REPO:-quant-system/engine-core}"
IMAGE_TAG="${IMAGE_TAG:-dev}"
MARKET_INGEST_IMAGE_REPO="${MARKET_INGEST_IMAGE_REPO:-quant-system/market-ingest}"
MARKET_INGEST_IMAGE_TAG="${MARKET_INGEST_IMAGE_TAG:-${IMAGE_TAG}}"
PUSH_IMAGE="${PUSH_IMAGE:-0}"

mkdir -p "${EVIDENCE_DIR}"

check_cmd() {
  local cmd="$1"
  if ! command -v "${cmd}" >/dev/null 2>&1; then
    echo "[phase14] error: required command not found: ${cmd}"
    exit 1
  fi
}

run_step() {
  local step="$1"
  shift
  local log_file="${EVIDENCE_DIR}/${step}.log"

  echo "[phase14] step=${step}" | tee -a "${SUMMARY_FILE}"
  if [[ "${DRY_RUN}" == "1" ]]; then
    echo "[phase14] DRY_RUN=1 skip execution: $*" | tee -a "${SUMMARY_FILE}"
    return 0
  fi

  set +e
  "$@" > >(tee "${log_file}") 2>&1
  local code=$?
  set -e

  if [[ ${code} -ne 0 ]]; then
    echo "[phase14] FAIL step=${step} exit=${code} log=${log_file}" | tee -a "${SUMMARY_FILE}"
    exit ${code}
  fi

  echo "[phase14] OK step=${step} log=${log_file}" | tee -a "${SUMMARY_FILE}"
}

check_cmd go
check_cmd kubectl
check_cmd helm
check_cmd curl
check_cmd jq
if [[ "${RUN_BUILD}" == "1" ]]; then
  check_cmd docker
fi

echo "phase14_rehearsal" > "${ENV_FILE}"
echo "generated_at=$(date '+%Y-%m-%d %H:%M:%S %z')" >> "${ENV_FILE}"
echo "cwd=${ROOT_DIR}" >> "${ENV_FILE}"
echo "kube_context=$(kubectl config current-context 2>/dev/null || echo N/A)" >> "${ENV_FILE}"
echo "engine_core_image=${IMAGE_REPO}:${IMAGE_TAG}" >> "${ENV_FILE}"
echo "market_ingest_image=${MARKET_INGEST_IMAGE_REPO}:${MARKET_INGEST_IMAGE_TAG}" >> "${ENV_FILE}"
env | sort >> "${ENV_FILE}"

cat > "${SUMMARY_FILE}" <<MD
# Phase-14 Rehearsal Summary

- generated_at: $(date '+%Y-%m-%d %H:%M:%S %z')
- evidence_dir: ${EVIDENCE_DIR}
- kube_context: $(kubectl config current-context 2>/dev/null || echo N/A)
- engine_core_image: ${IMAGE_REPO}:${IMAGE_TAG}
- market_ingest_image: ${MARKET_INGEST_IMAGE_REPO}:${MARKET_INGEST_IMAGE_TAG}
- run_tests: ${RUN_TESTS}
- run_build: ${RUN_BUILD}
- run_deploy: ${RUN_DEPLOY}
- run_obs: ${RUN_OBS}
- run_logging: ${RUN_LOGGING}
- run_smoke: ${RUN_SMOKE}
- run_mcp_gate: ${RUN_MCP_GATE}
- run_grafana_gate: ${RUN_GRAFANA_GATE}
- run_sandbox_trade_smoke: ${RUN_SANDBOX_TRADE_SMOKE}
- run_rollback: ${RUN_ROLLBACK}
- dry_run: ${DRY_RUN}
MD

if [[ "${RUN_TESTS}" == "1" ]]; then
  run_step "00-go-test" go test ./... -count=1
fi

if [[ "${RUN_BUILD}" == "1" ]]; then
  run_step "01-build-engine-core-image" env IMAGE_REPO="${IMAGE_REPO}" IMAGE_TAG="${IMAGE_TAG}" PUSH_IMAGE="${PUSH_IMAGE}" automation/scripts/build_engine_core_image.sh
  run_step "01b-build-market-ingest-image" env IMAGE_REPO="${MARKET_INGEST_IMAGE_REPO}" IMAGE_TAG="${MARKET_INGEST_IMAGE_TAG}" PUSH_IMAGE="${PUSH_IMAGE}" automation/scripts/build_market_ingest_image.sh
fi

if [[ "${RUN_DEPLOY}" == "1" ]]; then
  run_step "02-k8s-deploy" automation/scripts/k8s_deploy_dev.sh
fi

if [[ "${RUN_OBS}" == "1" ]]; then
  run_step "03-k8s-observability" automation/scripts/k8s_bootstrap_observability.sh
fi

if [[ "${RUN_LOGGING}" == "1" ]]; then
  run_step "04-k8s-logging" automation/scripts/k8s_bootstrap_logging.sh
fi

if [[ "${RUN_SMOKE}" == "1" ]]; then
  run_step "05-k8s-smoke" automation/scripts/k8s_smoke_status.sh
fi

if [[ "${RUN_MCP_GATE}" == "1" ]]; then
  run_step "06-mcp-gate" env EVIDENCE_DIR="${EVIDENCE_DIR}/mcp-gate" automation/scripts/mcp_observability_gate.sh
fi

if [[ "${RUN_GRAFANA_GATE}" == "1" ]]; then
  run_step "07-grafana-gate-legacy" automation/scripts/verify_grafana_data.sh
fi

if [[ "${RUN_SANDBOX_TRADE_SMOKE}" == "1" ]]; then
  run_step "08-sandbox-trade-smoke" env RUN_SANDBOX_TESTS=1 RUN_SANDBOX_TRADE_TESTS=1 go test ./test/sandbox/... -count=1 -v
fi

if [[ "${RUN_ROLLBACK}" == "1" ]]; then
  run_step "09-rollback-engine-core" kubectl rollout undo deployment/engine-core -n quant-system
  run_step "10-rollback-observability" bash -lc 'helm uninstall kube-prometheus-stack -n observability || true'
  run_step "11-rollback-logging" bash -lc 'helm uninstall fluent-bit -n observability || true'
fi

echo "[phase14] rehearsal completed, evidence_dir=${EVIDENCE_DIR}" | tee -a "${SUMMARY_FILE}"
echo "${EVIDENCE_DIR}"
