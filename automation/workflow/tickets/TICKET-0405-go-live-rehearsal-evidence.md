# TICKET-0405

## 1. 基本信息

- `ticket_id`: TICKET-0405
- `title`: Run dev go-live rehearsal and produce release decision evidence pack
- `owner_agent`: supervisor-report-agent
- `related_module`: automation/reports, automation/review-packs, automation/scripts, docs/release-checklist-v1.md
- `priority`: P0
- `status`: in_progress

## 2. 变更范围（必须精确）

- `allowed_write_paths`: `automation/reports/*`, `automation/review-packs/*`, `automation/scripts/*`, `docs/release-summary-v1.md`, `docs/go-live-phase14.md`, `automation/workflow/tickets/board.md`
- `forbidden_paths`: service implementation code
- `out_of_scope`: production change window execution

## 3. 输入与依赖

- `input_docs`: release-checklist-v1.md, runbook-v1.md, go-live-phase14.md
- `upstream_tickets`: `TICKET-0401`, `TICKET-0402`, `TICKET-0403`, `TICKET-0404`, `TICKET-0410`
- `external_constraints`: single-human audit mode

## 4. 验收条件（Definition of Done）

1. dev 环境完成发布与回滚演练记录。
2. 可观测看板截图或关键指标导出证据。
3. GO/NO-GO 决策包就绪。
4. MCP 门禁 `pass` 且证据归档。

## 5. 风险与回滚

- `risk_level`: high
- `rollback_plan`: NO-GO and hold release
- `hitl_required`: yes

## 6. 输出产物

1. rehearsal report
2. review pack
3. decision request draft
4. rehearsal execution logs
5. MCP gate evidence

## 7. 执行记录

1. 已落地状态草稿：`automation/reports/20260330-134819-phase-14-live-progress-status.md`。
2. 已落地审计草稿：`automation/review-packs/phase14-live-20260330-draft.md`。
3. 已将 `board.md` 更新为 phase-14 evidence closure + MCP governance in_progress。
4. 已新增 `automation/scripts/phase14_rehearsal.sh`（含 MCP gate 步骤）。
5. 已新增 `automation/scripts/mcp_observability_gate.sh` 与章程文档 `automation/workflow/mcp-charter-v1.md`。
6. 下一步：真实集群执行演练脚本并将 MCP 证据路径回填到 review pack，准备 GO/NO-GO 决策请求。
