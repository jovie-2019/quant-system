# Release Checklist V1

## 1. 发布前

1. 所有目标工单状态为 `accepted`。
2. 质量门禁通过（含 integration/replay/perf）。
3. 关键配置已确认：
- NATS URL
- MySQL DSN
- 风控参数默认值
4. 回滚方案已准备并演练。
5. 可观测栈已就绪（Prometheus/Grafana/Alertmanager）。
6. Grafana 已导入以下看板：
- `engine-core-v1.json`
- `engine-k8s-status-v1.json`
7. MCP 门禁通过：
- `automation/scripts/mcp_observability_gate.sh`
8. MCP 查询清单（controlapi、execution-gateway、ttlcache、momentum、market-ingest）全部非空返回。
9. phase-14 演练证据已归档：
- `automation/scripts/phase14_rehearsal.sh` 产出的 `summary.md` 与 step logs 可追溯
- 包含 `mcp-gate/summary.md` 与 `mcp-gate/result.json`

## 2. 发布中

1. 先发布 `nats/mysql`（若有变更）。
2. 再发布 `engine-core` 与 `strategy-runner`。
3. 发布窗口期间禁止并行高风险变更。
4. 若 MCP 运行中出现 `fail`，立即中止发布窗口。

## 3. 发布后

1. `engine-core` 健康探针通过。
2. `strategy-runner` 健康探针通过。
3. `market-ingest` 健康探针通过。
4. `nats` 健康检查通过：
- monitor 模式使用 `:8222/healthz`
- 默认模式使用 `4222` 连通性检查
5. 关键链路冒烟：
- 行情事件可达 strategy
- allow/reject 分支可达 risk
- allow 分支可达 execution
- orderfsm/position 更新正常
6. 30 分钟观测窗口内无关键告警。
7. 关键告警规则处于 `firing=0`（稳定窗口内）。
8. MCP 发布后复验通过并归档证据。

## 4. 不通过条件

1. 非法状态迁移出现。
2. 关键链路错误率异常升高。
3. 核心服务持续不可用超过阈值。
4. MCP 门禁失败或证据不完整。
