#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TEMPLATE="${ROOT_DIR}/automation/templates/review-pack/review-pack-template.md"
OUT_DIR="${ROOT_DIR}/automation/review-packs"
CHANGE_ID="${1:-change-$(date +%Y%m%d-%H%M%S)}"
OUT_FILE="${OUT_DIR}/${CHANGE_ID}.md"

mkdir -p "${OUT_DIR}"

if [[ ! -f "${TEMPLATE}" ]]; then
  echo "[review-pack] error: template not found: ${TEMPLATE}"
  exit 1
fi

GIT_SHA="N/A"
CHANGED_FILES="N/A"
if git -C "${ROOT_DIR}" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  GIT_SHA="$(git -C "${ROOT_DIR}" rev-parse --short HEAD || echo 'N/A')"
  CHANGED_FILES="$(git -C "${ROOT_DIR}" status --short || true)"
fi

cp "${TEMPLATE}" "${OUT_FILE}"

{
  echo
  echo "## 6. 自动附加信息"
  echo
  echo "- generated_at: $(date '+%Y-%m-%d %H:%M:%S %z')"
  echo "- git_sha: ${GIT_SHA}"
  echo "- changed_files:"
  if [[ -n "${CHANGED_FILES}" ]]; then
    echo '```text'
    echo "${CHANGED_FILES}"
    echo '```'
  else
    echo "  - none"
  fi
  echo
  echo "## 7. 待你确认（HITL）"
  echo
  echo "1. 本次变更是否涉及交易关键路径（risk/execution/orderfsm/position）"
  echo "2. 是否存在契约破坏性变更"
  echo "3. 性能是否达标（P99 阈值）"
  echo "4. 是否批准进入交付"
} >> "${OUT_FILE}"

echo "[review-pack] generated: ${OUT_FILE}"
