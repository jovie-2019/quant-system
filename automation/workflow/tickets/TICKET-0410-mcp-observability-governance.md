# TICKET-0410

## 1. 基本信息

- `ticket_id`: TICKET-0410
- `title`: Establish MCP observability governance and automated release gate
- `owner_agent`: mcp-gate-agent
- `related_module`: automation/workflow, automation/scripts, docs/services/infra, docs/go-live-phase14.md
- `priority`: P0
- `status`: in_progress

## 2. 变更范围（必须精确）

- `allowed_write_paths`: `automation/workflow/*`, `automation/agents/*`, `automation/scripts/*`, `docs/services/infra/*`, `docs/go-live-phase14.md`, `docs/release-checklist-v1.md`, `automation/README.md`
- `forbidden_paths`: core trading logic implementation
- `out_of_scope`: external MCP server procurement and org-level IAM rollout

## 3. 输入与依赖

- `input_docs`: workflow-v1.md, hitl-gates.md, observability.md, go-live-phase14.md
- `upstream_tickets`: `TICKET-0402`, `TICKET-0403`, `TICKET-0405`
- `external_constraints`: single-human audit mode

## 4. 验收条件（Definition of Done）

1. 新增 MCP 章程并纳入 workflow 主路径。
2. 提供可执行的 MCP 自动门禁脚本与证据输出目录。
3. release checklist 明确 MCP 为发布前硬门禁。
4. rehearsal 脚本默认执行 MCP 并归档证据。

## 5. 风险与回滚

- `risk_level`: medium
- `rollback_plan`: revert MCP policy docs/scripts and fallback to legacy grafana verification
- `hitl_required`: no

## 6. 输出产物

1. MCP charter doc
2. MCP gate script
3. workflow/reporting/hitl updates
4. release/go-live checklist updates

## 7. 执行记录

1. 已新增 `automation/workflow/mcp-charter-v1.md`。
2. 已新增 `automation/scripts/mcp_observability_gate.sh`。
3. 已将 `automation/scripts/phase14_rehearsal.sh` 接入 MCP gate。
4. 已更新 workflow/hitl/reporting/agent-catalog 与 observability/go-live/release checklist。
5. 待完成：真实集群执行 MCP gate 并回填审计包证据路径。
