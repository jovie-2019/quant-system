# TICKET-0403

## 1. 基本信息

- `ticket_id`: TICKET-0403
- `title`: Add Grafana k8s/service status dashboard and Prometheus alert rules
- `owner_agent`: release-agent
- `related_module`: deploy/observability/grafana, deploy/observability/prometheus-rules
- `priority`: P1
- `status`: accepted

## 2. 变更范围（必须精确）

- `allowed_write_paths`: `deploy/observability/*`, `docs/services/infra/observability.md`, `docs/runbook-v1.md`
- `forbidden_paths`: strategy/risk business behavior
- `out_of_scope`: enterprise oncall policy

## 3. 输入与依赖

- `input_docs`: observability.md, runbook-v1.md
- `upstream_tickets`: `TICKET-0402`
- `external_constraints`: single-screen operator dashboard

## 4. 验收条件（Definition of Done）

1. 提供 k8s + 服务状态 dashboard JSON。
2. 提供最小告警规则集（延迟、错误、重连、状态迁移异常）。
3. 提供 dashboard 导入与告警启用步骤。

## 5. 风险与回滚

- `risk_level`: low
- `rollback_plan`: disable alert rules and revert dashboards
- `hitl_required`: no

## 6. 输出产物

1. dashboard json
2. PrometheusRule yaml
3. doc update

## 7. 执行记录

1. 已新增 `deploy/observability/grafana/engine-k8s-status-v1.json`。
2. 已新增 `deploy/observability/prometheus-rules/engine-core-rules.yaml`。
3. 已在 observability/runbook/release-checklist 中补齐导入和启用步骤。
4. live 看板导入已完成：`engine-core-v1`、`engine-k8s-status-v1`。
5. 已修复 `engine-k8s-status-v1` 无数据查询（替换为当前运行态可用指标）。
6. 新增 `automation/scripts/verify_grafana_data.sh` 并在 live 环境执行通过。
7. live 告警触发演练待执行。
