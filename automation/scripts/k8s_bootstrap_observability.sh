#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

if ! command -v helm >/dev/null 2>&1; then
  echo "[obs] error: helm is not installed"
  exit 1
fi
if ! command -v kubectl >/dev/null 2>&1; then
  echo "[obs] error: kubectl is not installed"
  exit 1
fi

OBS_ROLLOUT_TIMEOUT="${OBS_ROLLOUT_TIMEOUT:-900s}"

check_cluster_connectivity() {
  if ! kubectl version --request-timeout=5s >/dev/null 2>&1; then
    echo "[obs] error: kubectl cannot reach apiserver (check kube context / network / permissions)"
    kubectl config current-context 2>/dev/null || true
    exit 1
  fi
}

rollout_or_debug() {
  local deploy="$1"
  if kubectl rollout status "deployment/${deploy}" -n observability --timeout="${OBS_ROLLOUT_TIMEOUT}"; then
    return 0
  fi

  echo "[obs] error: rollout timeout for deployment/${deploy}"
  echo "[obs] debug: deployment status"
  kubectl -n observability get deployment "${deploy}" -o wide || true
  echo "[obs] debug: related pods"
  kubectl -n observability get pods -l "app.kubernetes.io/instance=kube-prometheus-stack" -o wide || true
  echo "[obs] debug: recent events"
  kubectl -n observability get events --sort-by=.lastTimestamp | tail -n 20 || true
  return 1
}

check_cluster_connectivity

echo "[obs] install kube-prometheus-stack"
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts >/dev/null 2>&1 || true
helm repo update

kubectl create namespace observability --dry-run=client -o yaml | kubectl apply -f -

helm upgrade --install kube-prometheus-stack prometheus-community/kube-prometheus-stack \
  --namespace observability \
  --create-namespace \
  -f deploy/observability/kube-prometheus-stack-values.yaml

echo "[obs] apply ServiceMonitor and PrometheusRule"
kubectl apply -f deploy/observability/engine-core-servicemonitor.yaml
kubectl apply -f deploy/observability/strategy-runner-servicemonitor.yaml
kubectl apply -f deploy/observability/market-ingest-servicemonitor.yaml
kubectl apply -f deploy/observability/prometheus-rules/engine-core-rules.yaml
automation/scripts/grafana_import_dashboards.sh

echo "[obs] wait key deployments"
rollout_or_debug "kube-prometheus-stack-operator"
rollout_or_debug "kube-prometheus-stack-grafana"

echo "[obs] done"
echo "[obs] Grafana port-forward:"
echo "kubectl port-forward -n observability svc/kube-prometheus-stack-grafana 3000:80"
echo "[obs] Prometheus port-forward (if 9090 is occupied, use 19090):"
echo "kubectl port-forward -n observability svc/kube-prometheus-stack-prometheus 9090:9090"
