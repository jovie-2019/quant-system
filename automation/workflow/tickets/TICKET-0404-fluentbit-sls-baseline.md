# TICKET-0404

## 1. 基本信息

- `ticket_id`: TICKET-0404
- `title`: Add Fluent Bit to SLS logging baseline for k8s
- `owner_agent`: release-agent
- `related_module`: deploy/observability/fluent-bit, docs/services/infra/observability.md
- `priority`: P1
- `status`: in_progress

## 2. 变更范围（必须精确）

- `allowed_write_paths`: `deploy/observability/fluent-bit/*`, `automation/scripts/*`, `docs/services/infra/observability.md`, `docs/runbook-v1.md`
- `forbidden_paths`: core trade path code
- `out_of_scope`: advanced multi-tenant log routing

## 3. 输入与依赖

- `input_docs`: observability.md
- `upstream_tickets`: `TICKET-0402`
- `external_constraints`: minimum viable searchable logs

## 4. 验收条件（Definition of Done）

1. Fluent Bit helm values 支持采集容器日志。
2. 预留 SLS endpoint/project/logstore 参数化配置。
3. 提供部署与验证命令。

## 5. 风险与回滚

- `risk_level`: medium
- `rollback_plan`: uninstall fluent-bit release
- `hitl_required`: no

## 6. 输出产物

1. fluent-bit values
2. deployment helper script
3. doc update

## 7. 执行记录

1. 已新增 `deploy/observability/fluent-bit/values.yaml`（SLS 参数化占位）。
2. 已新增 `automation/scripts/k8s_bootstrap_logging.sh`。
3. 已在 observability/runbook 文档补齐部署命令。
4. live 集群已完成 Fluent Bit 安装与采集验证（容器日志采集正常，当前输出为 `stdout` 验证模式）。
5. live SLS 连通性验证待执行（需要真实 endpoint 和认证信息）。
