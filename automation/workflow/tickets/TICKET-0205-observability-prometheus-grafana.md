# TICKET-0205

## 1. 基本信息

- `ticket_id`: TICKET-0205
- `title`: Implement observability baseline (metrics, logs, dashboards)
- `owner_agent`: release-agent
- `related_module`: observability stack, engine-core metrics
- `priority`: P1
- `status`: accepted

## 2. 变更范围（必须精确）

- `allowed_write_paths`: `deploy/k8s/*`, `docs/services/infra/observability.md`, `docs/runbook-v1.md`, `internal/*`
- `forbidden_paths`: trading decision logic
- `out_of_scope`: full SRE paging policy

## 3. 输入与依赖

- `input_docs`: observability.md, runbook-v1.md
- `upstream_tickets`: `TICKET-0202`, `TICKET-0203`, `TICKET-0204`
- `external_constraints`: single-operator friendly dashboards

## 4. 验收条件（Definition of Done）

1. 暴露核心延迟/错误率/拒单率指标。
2. Grafana dashboard JSON 可导入并可视化。
3. 最小日志采集方案可检索关键链路日志。

## 5. 风险与回滚

- `risk_level`: medium
- `rollback_plan`: retain metrics endpoint + disable extra exporters
- `hitl_required`: no

## 6. 输出产物

1. `internal/obs/metrics/*`
2. `deploy/k8s/observability/*` or overlay updates
3. `docs/services/infra/observability.md` updates
4. Grafana dashboard JSON

## 7. 执行记录

1. Added metrics registry and `/metrics` exposition for Prometheus text format.
2. Added core metrics:
- controlapi requests + latency histogram
- risk decision/reject counters + eval latency
- execution submit outcome counters + latency
3. Added Grafana dashboard JSON: `deploy/observability/grafana/engine-core-v1.json`.
4. Added K8s service scrape annotations on engine-core service.
5. Updated observability/runbook docs and passed full quality gates.
