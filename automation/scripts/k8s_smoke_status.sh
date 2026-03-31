#!/usr/bin/env bash
set -euo pipefail

if ! command -v kubectl >/dev/null 2>&1; then
  echo "[smoke] error: kubectl is not installed"
  exit 1
fi

echo "[smoke] quant-system pods"
kubectl get pods -n quant-system -o wide

echo "[smoke] observability pods"
kubectl get pods -n observability -o wide

echo "[smoke] services"
kubectl get svc -n quant-system
kubectl get svc -n observability

echo "[smoke] workload readiness"
kubectl get deploy -n quant-system engine-core strategy-runner
kubectl get sts -n quant-system nats mysql

echo "[smoke] ServiceMonitor/PrometheusRule"
kubectl get servicemonitor -n quant-system || true
kubectl get prometheusrule -n quant-system || true

echo "[smoke] done"
