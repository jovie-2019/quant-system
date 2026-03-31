#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

IMAGE_REPO="${IMAGE_REPO:-quant-system/market-ingest}"
IMAGE_TAG="${IMAGE_TAG:-dev}"
FULL_IMAGE="${IMAGE_REPO}:${IMAGE_TAG}"

echo "[build] root=${ROOT_DIR}"
echo "[build] image=${FULL_IMAGE}"

if ! command -v docker >/dev/null 2>&1; then
  echo "[build] error: docker is not installed"
  exit 1
fi

docker build -t "${FULL_IMAGE}" .

if [[ "${PUSH_IMAGE:-0}" == "1" ]]; then
  echo "[build] push image"
  docker push "${FULL_IMAGE}"
else
  echo "[build] skip push (set PUSH_IMAGE=1 to enable)"
fi

echo "[build] done"
