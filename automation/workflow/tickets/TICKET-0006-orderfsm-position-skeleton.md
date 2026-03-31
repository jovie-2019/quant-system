# TICKET-0006

## 1. 基本信息

- `ticket_id`: TICKET-0006
- `title`: Implement orderfsm+position skeleton and consistency tests
- `owner_agent`: impl-agent
- `related_module`: internal/orderfsm, internal/position
- `priority`: P0
- `status`: accepted

## 2. 变更范围（必须精确）

- `allowed_write_paths`: `internal/orderfsm/*`, `internal/position/*`
- `forbidden_paths`: cross-module direct state mutation from other modules
- `out_of_scope`: full persistence integration

## 3. 输入与依赖

- `input_docs`: `orderfsm.md`, `position.md`, `event-contracts-v1.md`
- `upstream_tickets`: `TICKET-0001`, `TICKET-0002`, `TICKET-0003`, `TICKET-0004`, `TICKET-0005`
- `external_constraints`: requires HITL Gate 2 approval

## 4. 验收条件（Definition of Done）

1. 代码实现项：order state transition graph + fill-driven position ledger skeleton
2. 测试通过项：legal/illegal transition tests, fill idempotency tests
3. 门禁通过项：quality gates pass

## 5. 风险与回滚

- `risk_level`: high
- `rollback_plan`: revert module-level changes
- `hitl_required`: yes (`Gate 2`)

## 6. 输出产物

1. 代码变更清单
2. 测试报告
3. 一致性风险说明

## 7. 执行记录

1. Implemented order state machine skeleton with legal/illegal transition guards.
2. Implemented position ledger skeleton with fill-driven updates and trade idempotency.
3. Added unit tests for transition legality and fill idempotency/oversell protection.
4. Passed enabled quality gates (`gofmt`, `go vet`, `go test -race`).
