#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

echo "[k8s-dev] apply overlay: deploy/k8s/overlays/dev"

if ! command -v kubectl >/dev/null 2>&1; then
  echo "[k8s-dev] error: kubectl is not installed"
  exit 1
fi

kubectl apply -k deploy/k8s/overlays/dev

echo "[k8s-dev] wait rollout engine-core"
kubectl rollout status deployment/engine-core -n quant-system --timeout=300s
kubectl rollout status deployment/strategy-runner -n quant-system --timeout=300s
kubectl rollout status deployment/market-ingest -n quant-system --timeout=300s

echo "[k8s-dev] wait rollout nats/mysql"
kubectl rollout status statefulset/nats -n quant-system --timeout=300s
kubectl rollout status statefulset/mysql -n quant-system --timeout=300s

echo "[k8s-dev] pod status"
kubectl get pods -n quant-system -o wide
