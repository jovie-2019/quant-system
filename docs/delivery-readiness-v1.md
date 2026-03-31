# Delivery Readiness V1

## 1. 交付包内容

1. K8s 基线清单：`deploy/k8s/base/*`
2. 环境覆盖层：`deploy/k8s/overlays/dev/*`, `deploy/k8s/overlays/prod/*`
3. 运维 runbook：`docs/runbook-v1.md`
4. 发布检查单：`docs/release-checklist-v1.md`
5. MCP 门禁章程：`automation/workflow/mcp-charter-v1.md`

## 2. 当前就绪度

1. 代码骨架：已完成（核心链路模块 + controlapi）。
2. 测试基线：已完成（unit/integration/replay/perf）。
3. 交付文档：已完成（runbook/checklist/MCP charter）。
4. 本地中间件安装：已完成（NATS/MySQL 直连安装与健康检查通过）。
5. MCP 自动门禁：已落地脚本，待 live 证据闭环。

## 3. 已知差距

1. `TICKET-0405` 尚未完成真实集群演练证据归档。
2. `TICKET-0404` 仍需真实 SLS endpoint/auth 连通性验证。
3. `TICKET-0301` 仍需 sandbox trade smoke 实测凭据证据。

## 4. 下一步

1. 在 k8s dev 执行 `automation/scripts/phase14_rehearsal.sh` 并归档证据。
2. 执行 `RUN_ROLLBACK=1 automation/scripts/phase14_rehearsal.sh` 完成回滚演练。
3. 以 MCP pass 证据触发 HITL 决策并收口 phase-14。
