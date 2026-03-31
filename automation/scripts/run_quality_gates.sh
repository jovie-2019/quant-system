#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

echo "[gate] root=${ROOT_DIR}"

if ! command -v go >/dev/null 2>&1; then
  echo "[gate] error: go is not installed"
  exit 1
fi

GO_FILES_COUNT="$(find . -type f -name '*.go' | wc -l | tr -d ' ')"
if [[ "${GO_FILES_COUNT}" == "0" ]]; then
  echo "[gate] warning: no go files found, skip go gates"
  exit 0
fi

echo "[gate] gofmt check"
UNFORMATTED="$(gofmt -l . | sed '/^$/d' || true)"
if [[ -n "${UNFORMATTED}" ]]; then
  echo "[gate] error: gofmt violations found"
  echo "${UNFORMATTED}"
  exit 1
fi

echo "[gate] go vet"
go vet ./...

if command -v golangci-lint >/dev/null 2>&1; then
  echo "[gate] golangci-lint"
  golangci-lint run
else
  echo "[gate] warning: golangci-lint not installed, skip lint gate"
fi

echo "[gate] unit + race"
go test ./... -race

if [[ "${RUN_INTEGRATION_TESTS:-0}" == "1" ]]; then
  echo "[gate] integration tests"
  go test ./test/integration/... -count=1
else
  echo "[gate] skip integration tests (set RUN_INTEGRATION_TESTS=1 to enable)"
fi

if [[ "${RUN_REPLAY_TESTS:-0}" == "1" ]]; then
  echo "[gate] replay tests"
  go test ./test/replay/... -count=1
else
  echo "[gate] skip replay tests (set RUN_REPLAY_TESTS=1 to enable)"
fi

if [[ "${RUN_PERF_TESTS:-0}" == "1" ]]; then
  echo "[gate] performance tests"
  go test ./test/perf/... -count=1
else
  echo "[gate] skip performance tests (set RUN_PERF_TESTS=1 to enable)"
fi

if [[ "${RUN_SANDBOX_TESTS:-0}" == "1" ]]; then
  echo "[gate] sandbox tests"
  go test ./test/sandbox/... -count=1 -v
else
  echo "[gate] skip sandbox tests (set RUN_SANDBOX_TESTS=1 to enable)"
fi

echo "[gate] all enabled quality gates passed"
