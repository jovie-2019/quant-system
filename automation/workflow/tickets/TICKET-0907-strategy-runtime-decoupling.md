# TICKET-0907

## 1. 基本信息

- `ticket_id`: TICKET-0907
- `title`: Strategy Runtime Decoupling (engine-core -> strategy-runner)
- `owner_agent`: impl-agent
- `related_module`: cmd/strategy-runner, deploy/k8s/base, docs/services/strategy-runner
- `priority`: P0
- `status`: in_progress

## 2. 变更范围（必须精确）

- `allowed_write_paths`: `cmd/strategy-runner/*`, `internal/strategyrunner/*`, `deploy/k8s/base/*strategy-runner*`, `deploy/k8s/overlays/*`, `docs/services/strategy-runner/*`, `docs/architecture-v1.md`, `docs/runbook-v1.md`
- `forbidden_paths`: venue production credential files
- `out_of_scope`: strategy alpha logic研发（仅做运行时/部署解耦）

## 3. 输入与依赖

- `input_docs`: docs/architecture-v1.md, docs/services/engine-core/service-spec.md
- `upstream_tickets`: `TICKET-0401`, `TICKET-0402`
- `external_constraints`: single-human audit mode

## 4. 验收条件（Definition of Done）

1. `strategy-runner` 可独立启动并通过 `/api/v1/health`。
2. k8s dev overlay 可部署 `strategy-runner`（Deployment/Service/PDB）。
3. 观测链路能看到 `strategy-runner` 可用性（Prometheus + Grafana）。

## 5. 风险与回滚

- `risk_level`: medium
- `rollback_plan`: 回滚 `strategy-runner` Deployment 到上一个镜像版本
- `hitl_required`: no

## 6. 输出产物

1. strategy-runner service spec
2. k8s deployment resources
3. smoke/validation evidence links
