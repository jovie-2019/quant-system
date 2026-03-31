# TICKET-0204

## 1. 基本信息

- `ticket_id`: TICKET-0204
- `title`: Integrate NATS bus for async event fan-out and replay path
- `owner_agent`: impl-agent
- `related_module`: internal/bus, internal/hub, internal/execution
- `priority`: P0
- `status`: accepted

## 2. 变更范围（必须精确）

- `allowed_write_paths`: `internal/bus/*`, `internal/hub/*`, `internal/execution/*`, `docs/services/infra/nats.md`, `docs/services/contracts/*`
- `forbidden_paths`: external adapter network stack
- `out_of_scope`: multi-cluster disaster recovery

## 3. 输入与依赖

- `input_docs`: nats.md, event-contracts-v1.md
- `upstream_tickets`: `TICKET-0201`
- `external_constraints`: subjects follow contract naming conventions

## 4. 验收条件（Definition of Done）

1. 发布/订阅封装支持最小 ack/retry 策略。
2. 核心事件流入 NATS subject（market/risk/order/fill）。
3. 增加事件回放入口与一致性测试。

## 5. 风险与回滚

- `risk_level`: high
- `rollback_plan`: dual-write off, fallback to in-memory direct call path
- `hitl_required`: yes (`Gate 6`)

## 6. 输出产物

1. `internal/bus/natsbus/*`
2. `docs/services/infra/nats.md`（补充 subject 与发布策略）
3. event publish/subscribe tests

## 7. 执行记录

1. Added `internal/bus/natsbus` client wrapper with connect/ensure-stream/publish/durable-subscribe.
2. Added subject builders and typed publish helper for market/risk/order/fill events.
3. Added replay entry `ReplayTradeFill` with pull-consumer scan.
4. Added unit tests and optional live NATS integration tests (`RUN_NATS_TESTS=1`) covering retry and replay.
5. Full quality gates passed after bus integration changes.
