# TICKET-0206

## 1. 基本信息

- `ticket_id`: TICKET-0206
- `title`: Implement local order book module and strategy-facing snapshot API
- `owner_agent`: impl-agent
- `related_module`: internal/book, internal/hub, internal/strategy
- `priority`: P1
- `status`: accepted

## 2. 变更范围（必须精确）

- `allowed_write_paths`: `internal/book/*`, `internal/hub/*`, `internal/strategy/*`, `docs/services/engine-core/modules/book.md`
- `forbidden_paths`: execution gateway internals
- `out_of_scope`: derivatives depth model

## 3. 输入与依赖

- `input_docs`: book.md, hub.md
- `upstream_tickets`: `TICKET-0202`
- `external_constraints`: spot depth first, futures later

## 4. 验收条件（Definition of Done）

1. 维护本地 orderbook snapshot + incremental 更新。
2. 暴露策略侧读取 API（best bid/ask, depth levels）。
3. 回放测试验证序列乱序/丢包处理。

## 5. 风险与回滚

- `risk_level`: medium
- `rollback_plan`: keep top-of-book only mode
- `hitl_required`: no

## 6. 输出产物

1. `internal/book/*`
2. strategy-facing orderbook read API
3. replay tests for orderbook sequence behavior

## 7. 执行记录

1. Added `internal/book` in-memory engine with snapshot + incremental update.
2. Added sequence anomaly handling (duplicate/out-of-order/gap) with stale state tracking.
3. Integrated hub with book engine and exposed `GetBookSnapshot` / `BookSeqGapCount`.
4. Added strategy-side read API via runtime-injected `BookReader`.
5. Added replay test for sequence behavior and passed full quality gates.
