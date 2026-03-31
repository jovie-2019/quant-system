# TICKET-0005

## 1. 基本信息

- `ticket_id`: TICKET-0005
- `title`: Implement risk+execution skeleton and guardrail tests
- `owner_agent`: impl-agent
- `related_module`: internal/risk, internal/execution
- `priority`: P0
- `status`: accepted

## 2. 变更范围（必须精确）

- `allowed_write_paths`: `internal/risk/*`, `internal/execution/*`
- `forbidden_paths`: `internal/orderfsm/*`, `internal/position/*`
- `out_of_scope`: state-machine and position ledger implementation

## 3. 输入与依赖

- `input_docs`: `risk.md`, `execution.md`, `event-contracts-v1.md`
- `upstream_tickets`: `TICKET-0001`, `TICKET-0002`, `TICKET-0003`, `TICKET-0004`
- `external_constraints`: requires HITL Gate 2 approval

## 4. 验收条件（Definition of Done）

1. 代码实现项：risk evaluate flow + execution idempotency skeleton
2. 测试通过项：reject/allow path tests, idempotency tests
3. 门禁通过项：quality gates pass

## 5. 风险与回滚

- `risk_level`: high
- `rollback_plan`: revert module-level changes
- `hitl_required`: yes (`Gate 2`)

## 6. 输出产物

1. 代码变更清单
2. 测试报告
3. 关键路径风险说明

## 7. 执行记录

1. Implemented risk engine skeleton with idempotent decisions and fail-closed checks.
2. Implemented execution skeleton with idempotent submit and gateway passthrough cancel.
3. Added guardrail unit tests for allow/reject/idempotency paths.
4. Passed enabled quality gates (`gofmt`, `go vet`, `go test -race`).
