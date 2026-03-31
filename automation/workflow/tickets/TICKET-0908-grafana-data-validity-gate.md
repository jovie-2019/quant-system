# TICKET-0908

## 1. 基本信息

- `ticket_id`: TICKET-0908
- `title`: Grafana Data Validity and Coverage Gate
- `owner_agent`: release-agent
- `related_module`: deploy/observability/grafana, automation/scripts/verify_grafana_data.sh, docs/go-live-phase14.md
- `priority`: P0
- `status`: in_progress

## 2. 变更范围（必须精确）

- `allowed_write_paths`: `deploy/observability/grafana/*`, `automation/scripts/verify_grafana_data.sh`, `docs/go-live-phase14.md`, `docs/release-checklist-v1.md`, `docs/services/infra/observability.md`
- `forbidden_paths`: service implementation code
- `out_of_scope`: 新增复杂业务指标（仅做数据有效性与覆盖率门禁）

## 3. 输入与依赖

- `input_docs`: docs/go-live-phase14.md, docs/release-checklist-v1.md
- `upstream_tickets`: `TICKET-0403`, `TICKET-0907`
- `external_constraints`: single-human audit mode

## 4. 验收条件（Definition of Done）

1. `engine-core-v1` 与 `engine-k8s-status-v1` 看板存在且可查询。
2. 核心查询非空（含 `strategy-runner-up`）。
3. 发布前检查项显式包含 Grafana 数据有效性门禁。

## 5. 风险与回滚

- `risk_level`: low
- `rollback_plan`: 回滚 dashboard JSON 与验证脚本到上一版
- `hitl_required`: no

## 6. 输出产物

1. dashboard query revision
2. grafana data validation script
3. release checklist updates
