# TICKET-0202

## 1. 基本信息

- `ticket_id`: TICKET-0202
- `title`: Implement production-grade Binance/OKX adapter (WS market + REST trade)
- `owner_agent`: impl-agent
- `related_module`: internal/adapter, internal/normalizer
- `priority`: P0
- `status`: accepted

## 2. 变更范围（必须精确）

- `allowed_write_paths`: `internal/adapter/*`, `internal/normalizer/*`, `test/integration/*`, `docs/services/engine-core/modules/adapter.md`
- `forbidden_paths`: risk/orderfsm/position semantics
- `out_of_scope`: futures/perpetual market support

## 3. 输入与依赖

- `input_docs`: adapter.md, event-contracts-v1.md
- `upstream_tickets`: `TICKET-0201`
- `external_constraints`: spot only in this milestone

## 4. 验收条件（Definition of Done）

1. Binance/OKX market stream WS connect/reconnect/heartbeat.
2. Spot order place/cancel REST with rate-limit handling.
3. Adapter integration test with replay/mock server.

## 5. 风险与回滚

- `risk_level`: high
- `rollback_plan`: switch back to stub adapter implementation
- `hitl_required`: yes (`Gate 2`)

## 6. 输出产物

1. `internal/adapter/binance_rest.go`
2. `internal/adapter/okx_rest.go`
3. `internal/adapter/binance_rest_test.go`
4. `internal/adapter/okx_rest_test.go`
5. `internal/adapter/rest_common.go`

## 7. 执行记录

1. Completed spot REST trade gateway for Binance/OKX (place/cancel, signing, minimal request pacing, error propagation).
2. Added adapter unit tests for request/response contract and auth headers.
3. Quality gates passed after gateway implementation.
4. Completed WS market stream connect/reconnect/heartbeat and subscription recovery for Binance/OKX.
5. Added mock WebSocket reconnect test and parser tests for Binance/OKX market events.
