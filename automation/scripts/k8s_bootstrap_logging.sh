#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

if ! command -v helm >/dev/null 2>&1; then
  echo "[log] error: helm is not installed"
  exit 1
fi
if ! command -v kubectl >/dev/null 2>&1; then
  echo "[log] error: kubectl is not installed"
  exit 1
fi

echo "[log] install fluent-bit"
helm repo add fluent https://fluent.github.io/helm-charts >/dev/null 2>&1 || true
helm repo update

kubectl create namespace observability --dry-run=client -o yaml | kubectl apply -f -

helm upgrade --install fluent-bit fluent/fluent-bit \
  --namespace observability \
  --create-namespace \
  -f deploy/observability/fluent-bit/values.yaml

echo "[log] done"
echo "[log] verify:"
echo "kubectl get pods -n observability -l app.kubernetes.io/name=fluent-bit"
