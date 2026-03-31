# TICKET-0910

## 1. 基本信息

- `ticket_id`: TICKET-0910
- `title`: Trade Gateway Hardening and Recoverability
- `owner_agent`: impl-agent
- `related_module`: internal/adapter, internal/execution, docs/runbook-v1.md, automation/workflow
- `priority`: P0
- `status`: in_progress

## 2. 变更范围（必须精确）

- `allowed_write_paths`: `internal/adapter/*`, `internal/execution/*`, `test/sandbox/*`, `docs/runbook-v1.md`, `docs/services/engine-core/modules/execution.md`, `automation/workflow/*`
- `forbidden_paths`: strategy logic and risk policy semantics
- `out_of_scope`: 多资产统一撮合路由、跨交易所智能拆单

## 3. 输入与依赖

- `input_docs`: docs/services/engine-core/modules/execution.md, docs/services/engine-core/modules/adapter.md, automation/workflow/trade-gateway-hardening-checklist-v1.md
- `upstream_tickets`: `TICKET-0202`, `TICKET-0301`
- `external_constraints`: sandbox 凭据可用性、交易所 API 限频

## 4. 当前评估快照（2026-03-31）

- 完成度：`94/100`（主路径可用，重试/分流/恢复与观测门禁已打通，负向集成覆盖已补齐）
- 已完成：
1. Binance/OKX 下单/撤单主路径实现。
2. 签名和基础契约单测已覆盖。
3. 已增加请求参数校验与 `retryable/non-retryable` 错误分类基础能力。
4. Binance 撤单已支持 `venueOrderID` 路径，避免仅持有交易所订单号时无法撤单。
5. execution 层已增加有界重试与指数退避（仅 `retryable` 错误重试）。
6. execution 层已输出 gateway 事件指标（success/retry/retry_exhausted/non_retryable_error）。
7. Grafana 主看板已接入 execution gateway 事件速率面板。
8. Prometheus 已新增 gateway `retry_exhausted` 告警规则。
9. Binance/OKX gateway 已支持按 `clientOrderID/venueOrderID` 查单，execution 已提供 `Reconcile` 入口。
10. execution 重试策略已增加抖动（jitter）与分操作重试预算（place/cancel/query）。
11. 已补充 pipeline 负向集成测试：`retryable exhausted` 与 `non-retryable` 场景，验证重试边界和错误分流行为。
- 主要缺口：
1. 尚缺按 venue 动态预算与熔断联动策略。
2. 告警规则尚缺“真实集群触发-恢复”证据闭环。

## 5. 验收条件（Definition of Done）

1. 按 `trade-gateway-hardening-checklist-v1.md` 输出 `pass` 结论。
2. execution 层可区分并处理 `retryable/non-retryable` 错误。
3. 至少一种恢复路径可执行（查询订单状态或等价机制）。
4. sandbox 测试覆盖真实下单-撤单链路和至少一条负向用例。
5. runbook 补齐故障定位和恢复步骤。

## 6. 风险与回滚

- `risk_level`: high
- `rollback_plan`: 回滚到当前 gateway 实现并关闭重试/恢复新路径
- `hitl_required`: yes

## 7. 输出产物

1. gateway hardening 代码与测试
2. checklist 评估结果与证据路径
3. sandbox 验证记录
4. 审计摘要与风险残留说明
