#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

if ! command -v kubectl >/dev/null 2>&1; then
  echo "[grafana-import] error: kubectl is not installed"
  exit 1
fi

NAMESPACE="${NAMESPACE:-observability}"

apply_dashboard_configmap() {
  local name="$1"
  local file="$2"

  if [[ ! -f "${file}" ]]; then
    echo "[grafana-import] error: dashboard file not found: ${file}"
    exit 1
  fi

  echo "[grafana-import] apply configmap ${name} from ${file}"
  kubectl create configmap "${name}" \
    -n "${NAMESPACE}" \
    --from-file="$(basename "${file}")=${file}" \
    --dry-run=client \
    -o yaml \
    | kubectl label --local -f - grafana_dashboard=1 -o yaml \
    | kubectl apply -f -
}

apply_dashboard_configmap "grafana-dashboard-engine-core-v1" "deploy/observability/grafana/engine-core-v1.json"
apply_dashboard_configmap "grafana-dashboard-engine-k8s-status-v1" "deploy/observability/grafana/engine-k8s-status-v1.json"

echo "[grafana-import] done"
