#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPORTS_DIR="${ROOT_DIR}/automation/reports"
TEMPLATE_STATUS="${ROOT_DIR}/automation/templates/reporting/status-report-template.md"
TEMPLATE_DECISION="${ROOT_DIR}/automation/templates/reporting/decision-request-template.md"

TYPE="${1:-status}"
PHASE="${2:-unknown-phase}"
STATUS="${3:-on_track}"

mkdir -p "${REPORTS_DIR}"

TS="$(date +%Y%m%d-%H%M%S)"

case "${TYPE}" in
  status)
    TEMPLATE="${TEMPLATE_STATUS}"
    OUT_FILE="${REPORTS_DIR}/${TS}-${PHASE}-status.md"
    ;;
  decision)
    TEMPLATE="${TEMPLATE_DECISION}"
    OUT_FILE="${REPORTS_DIR}/${TS}-${PHASE}-decision.md"
    ;;
  *)
    echo "[report] error: unsupported type '${TYPE}', use 'status' or 'decision'"
    exit 1
    ;;
esac

if [[ ! -f "${TEMPLATE}" ]]; then
  echo "[report] error: template not found: ${TEMPLATE}"
  exit 1
fi

cp "${TEMPLATE}" "${OUT_FILE}"

{
  echo
  echo "## Auto Filled Context"
  echo
  echo "- generated_at: $(date '+%Y-%m-%d %H:%M:%S %z')"
  echo "- phase: ${PHASE}"
  echo "- status: ${STATUS}"
  if [[ "${TYPE}" == "decision" ]]; then
    echo "- timeout_default_action: NO-GO and pause workflow"
  fi
} >> "${OUT_FILE}"

echo "[report] generated: ${OUT_FILE}"
