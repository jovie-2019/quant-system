# TICKET-0013

## 1. 基本信息

- `ticket_id`: TICKET-0013
- `title`: Execute phase-10 delivery checks in local environment
- `owner_agent`: release-agent
- `related_module`: cmd/engine-core, docs, automation reports
- `priority`: P0
- `status`: accepted

## 2. 变更范围（必须精确）

- `allowed_write_paths`: `cmd/engine-core/*`, `docs/*`, `automation/reports/*`, `automation/workflow/tickets/*`
- `forbidden_paths`: core trading logic semantics
- `out_of_scope`: production cluster rollout

## 3. 输入与依赖

- `input_docs`: runbook-v1.md, release-checklist-v1.md, delivery-readiness-v1.md
- `upstream_tickets`: `TICKET-0012`
- `external_constraints`: local-only delivery verification

## 4. 验收条件（Definition of Done）

1. 提供 `engine-core` 可运行入口并通过健康检查。
2. 完整质量门禁通过（含 integration/replay/perf）。
3. 本地中间件健康检查通过并形成交付证据。
4. phase-10 状态报告输出并更新看板。

## 5. 风险与回滚

- `risk_level`: medium
- `rollback_plan`: revert cmd/docs/automation changes
- `hitl_required`: no

## 6. 输出产物

1. `cmd/engine-core/main.go`
2. `automation/reports/20260326-100200-phase-10-complete-status.md`
3. `automation/workflow/tickets/board.md`

## 7. 执行记录

1. Added runnable `engine-core` entrypoint with graceful shutdown.
2. Re-ran full quality gates with integration/replay/perf enabled; all passed.
3. Performed local smoke checks for control API endpoints and middleware health.
4. Updated runbook/checklist to handle both NATS monitor-mode and default mode checks.
