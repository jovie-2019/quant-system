# TICKET-0402

## 1. 基本信息

- `ticket_id`: TICKET-0402
- `title`: Bootstrap kube-prometheus-stack and engine-core metrics scraping in k8s
- `owner_agent`: release-agent
- `related_module`: deploy/observability, automation/scripts
- `priority`: P0
- `status`: accepted

## 2. 变更范围（必须精确）

- `allowed_write_paths`: `deploy/observability/*`, `automation/scripts/*`, `docs/services/infra/observability.md`
- `forbidden_paths`: trading core logic
- `out_of_scope`: long-term storage/HA tuning

## 3. 输入与依赖

- `input_docs`: observability.md
- `upstream_tickets`: `TICKET-0401`
- `external_constraints`: minimal-cost default setup

## 4. 验收条件（Definition of Done）

1. 提供 kube-prometheus-stack values 配置。
2. 提供 engine-core ServiceMonitor 资源。
3. 提供可执行安装脚本。

## 5. 风险与回滚

- `risk_level`: medium
- `rollback_plan`: uninstall observability namespace release
- `hitl_required`: no

## 6. 输出产物

1. helm values and k8s manifests
2. bootstrap script
3. doc update

## 7. 执行记录

1. 已新增 `deploy/observability/kube-prometheus-stack-values.yaml`。
2. 已新增 `deploy/observability/engine-core-servicemonitor.yaml`。
3. 已新增 `automation/scripts/k8s_bootstrap_observability.sh`。
4. live 集群安装验证已完成（Prometheus/Grafana/Blackbox 均 Running）。
5. Prometheus 目标抓取已验证：`engine-core` + `blackbox-engine-core-health` + `blackbox-nats-health` + `blackbox-mysql-tcp`。
