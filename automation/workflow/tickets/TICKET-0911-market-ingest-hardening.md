# TICKET-0911

## 1. 基本信息

- `ticket_id`: TICKET-0911
- `title`: Market Ingest Production Hardening
- `owner_agent`: impl-agent
- `related_module`: cmd/market-ingest, deploy/k8s, automation/scripts, deploy/observability
- `priority`: P1
- `status`: in_progress

## 2. 变更范围（必须精确）

- `allowed_write_paths`: `cmd/market-ingest/*`, `internal/obs/metrics/*`, `deploy/k8s/*`, `deploy/observability/*`, `automation/scripts/*`, `docs/go-live-phase14.md`, `docs/runbook-v1.md`
- `forbidden_paths`: strategy/risk business logic
- `out_of_scope`: 新增交易策略、跨交易所聚合引擎

## 3. 输入与依赖

- `input_docs`: docs/go-live-phase14.md, docs/runbook-v1.md, automation/workflow/mcp-charter-v1.md
- `upstream_tickets`: `TICKET-0401`, `TICKET-0402`, `TICKET-0908`, `TICKET-0410`
- `external_constraints`: k8s dev 环境和 Prometheus 可用性

## 4. 当前评估快照（2026-03-31）

- 完成度：`84/100`
- 已完成：
1. `market-ingest` 服务、部署、ServiceMonitor、Grafana 和 MCP 门禁均已纳入主流程。
2. 增加 `engine_market_ingest_events_total` 指标与命令层单测。
3. phase14 演练脚本与 dev overlay 已接入 market-ingest 镜像链路。
4. 修复 MCP gate `market_ingest_up` 结果落盘变量缺失问题。
- 剩余缺口：
1. WebSocket 断连重连与退避策略需增强为显式可验证行为。
2. 错误样本（normalize/publish）缺少结构化落盘或 DLQ 回溯路径。
3. 缺少端到端集成测试（真实流输入 -> NATS -> 订阅验证）的常态化门禁。

## 5. 验收条件（Definition of Done）

1. market-ingest 在断链场景可自动恢复并有可观测证据。
2. ingest 错误可追溯（日志或 DLQ）且 runbook 有定位步骤。
3. 增加至少一条 market-ingest 端到端测试门禁。
4. MCP 报告包含 market-ingest 可用性和事件速率快照。

## 6. 风险与回滚

- `risk_level`: medium
- `rollback_plan`: 回滚 market-ingest deployment 与观测脚本改动，恢复至当前稳定版本
- `hitl_required`: no

## 7. 输出产物

1. market-ingest hardening 代码与测试
2. k8s/observability 配置更新
3. 演练证据与审计摘要
