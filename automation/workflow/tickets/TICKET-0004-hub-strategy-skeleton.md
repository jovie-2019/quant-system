# TICKET-0004

## 1. 基本信息

- `ticket_id`: TICKET-0004
- `title`: Implement hub+strategy skeleton and baseline tests
- `owner_agent`: impl-agent
- `related_module`: internal/hub, internal/strategy
- `priority`: P1
- `status`: accepted

## 2. 变更范围（必须精确）

- `allowed_write_paths`: `internal/hub/*`, `internal/strategy/*`
- `forbidden_paths`: `internal/risk/*`, `internal/execution/*`, `internal/orderfsm/*`, `internal/position/*`
- `out_of_scope`: trade execution path

## 3. 输入与依赖

- `input_docs`: `docs/services/engine-core/modules/hub.md`, `strategy.md`
- `upstream_tickets`: `TICKET-0001`, `TICKET-0002`, `TICKET-0003`
- `external_constraints`: must pass quality gates

## 4. 验收条件（Definition of Done）

1. 代码实现项：hub/strategy interfaces + in-memory runtime skeleton
2. 测试通过项：publish/subscribe and strategy intent dispatch unit tests
3. 门禁通过项：quality gates pass

## 5. 风险与回滚

- `risk_level`: low
- `rollback_plan`: revert module-level changes
- `hitl_required`: no (unless boundary drift)

## 6. 输出产物

1. 代码变更清单
2. 测试报告
3. 下一阶段输入（risk/execution）

## 7. 执行记录

1. Added in-memory hub with snapshot + subscribe/publish flow.
2. Added strategy runtime skeleton with intent sink dispatch.
3. Added unit tests for hub and strategy behavior.
4. Passed enabled quality gates (`gofmt`, `go vet`, `go test -race`).
