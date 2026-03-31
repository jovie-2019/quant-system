# MCP Charter V1 (Machine Check First, Human Final)

## 0. 宗旨

本章程将数据观测验收从“人工目检”升级为“`MCP` 自动门禁优先 + 人类最终拍板”。

`MCP` 在本项目定义为：

1. `M`onitoring Data Plane：Prometheus/Grafana/Alertmanager/SLS 的数据面采集与查询。
2. `C`onformance Policy：可执行、可版本化的验收规则（查询、阈值、通过条件）。
3. `P`roof & Promotion Gate：自动生成证据并决定是否允许进入下一阶段（晋级或阻断）。

## 1. 适用范围

1. 所有 phase-14 及其后续发布演练。
2. 任何涉及可观测链路、发布门禁、上线决策的变更。
3. 所有 GO/NO-GO 决策前的证据收集。

## 2. 组织与职责

1. `mcp-gate-agent`（主责）：执行 `automation/scripts/mcp_observability_gate.sh`，产出证据与结果。
2. `release-agent`：负责部署、回滚、环境准备，不替代 MCP 门禁判定。
3. `audit-agent`：核对 MCP 证据完整性与一致性。
4. Human：仅在 HITL Gate 做最终拍板，不做机械校验。

## 3. 强制门禁规则

1. 未通过 MCP 门禁，禁止进入发布决策（默认 `NO-GO`）。
2. MCP 证据目录必须可追溯（含 `summary.md`、`result.json`、step logs）。
3. 任何手工豁免必须进入 HITL，且记录豁免原因、时效、补救动作。
4. 证据缺失视同失败（Fail Closed）。

## 4. MCP 最小通过条件（V1）

1. Grafana 关键看板存在：`engine-core-v1`、`engine-k8s-status-v1`。
2. Prometheus 关键查询非空：
- `engine-core-up`
- `strategy-runner-up`
- `market-ingest-up`
- `nats-up`
- `mysql-up`
- `controlapi-p99`
- `controlapi-status-rate`
- `execution-gateway-event-rate`
- `ttlcache-get-rate`
- `ttlcache-size`
- `momentum-eval-rate`
- `momentum-eval-p95`
- `momentum-signal-rate`
- `market-ingest-event-rate`
3. 关键可用性阈值满足（默认）：
- `engine-core-up >= 1`
- `strategy-runner-up >= 1`
- `market-ingest-up >= 1`
- `nats-up >= 1`
- `mysql-up >= 1`
4. 告警阈值满足（默认）：
- `critical_firing <= 0`
- `warning_firing <= 5`

## 5. 证据标准

每次 MCP 执行必须输出到：`automation/reports/mcp-gate-<timestamp>/`

1. `summary.md`：可读摘要（结论、失败点、下一步）。
2. `result.json`：结构化结果（status、metrics、thresholds）。
3. `verify-grafana-data.log`：Grafana/Prom 查询检查日志。
4. `env.txt`：执行环境、上下文、关键参数。

## 6. 生命周期接入点

1. 发布前：必须执行一次 MCP gate。
2. 发布后稳定窗口（建议 30 分钟）：必须再执行一次 MCP gate。
3. 回滚后：必须执行一次 MCP gate 验证恢复状态。

## 7. 升级与异常策略

1. 任一步失败：流水线状态置 `blocked`，并自动触发 `decision-request`。
2. 超时/连接异常：按失败处理，不允许“默认成功”。
3. 允许临时降级阈值，但必须：
- 进入 HITL
- 记录有效期
- 创建补偿工单

## 8. 审计要求

1. 审计包必须引用 MCP 证据路径。
2. `GO` 决策必须绑定一份最近、完整、通过的 MCP 证据。
3. 若 `GO` 基于豁免，审计包必须包含风险承担人和回滚预案。
