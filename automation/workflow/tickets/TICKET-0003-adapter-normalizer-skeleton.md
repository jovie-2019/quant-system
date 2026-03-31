# TICKET-0003

## 1. 基本信息

- `ticket_id`: TICKET-0003
- `title`: Implement adapter+normalizer skeleton and baseline tests
- `owner_agent`: impl-agent
- `related_module`: internal/adapter, internal/normalizer, test/*
- `priority`: P1
- `status`: accepted

## 2. 变更范围（必须精确）

- `allowed_write_paths`: `internal/adapter/*`, `internal/normalizer/*`, `test/*`
- `forbidden_paths`: `internal/risk/*`, `internal/execution/*`, `internal/orderfsm/*`, `internal/position/*`
- `out_of_scope`: trading path implementation

## 3. 输入与依赖

- `input_docs`: module specs + event contracts
- `upstream_tickets`: `TICKET-0001`, `TICKET-0002`
- `external_constraints`: must pass quality gates

## 4. 验收条件（Definition of Done）

1. 文档更新项：必要时补模块细则
2. 代码实现项：skeleton interfaces and constructors
3. 测试通过项：unit tests for mapper and parse error handling
4. 性能约束：no regression in parser latency baseline

## 5. 风险与回滚

- `risk_level`: medium
- `rollback_plan`: revert module-level changes
- `hitl_required`: no (unless boundary drift)

## 6. 输出产物

1. 代码变更清单
2. 测试报告
3. 下一阶段输入（hub/strategy）

## 7. 执行记录

1. Added `go.mod` and module skeleton for adapter/normalizer.
2. Added unit tests for mapping success and parse/missing-field errors.
3. Passed enabled quality gates (`gofmt`, `go vet`, `go test -race`).
