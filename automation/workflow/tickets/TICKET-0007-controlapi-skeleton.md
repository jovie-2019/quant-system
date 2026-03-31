# TICKET-0007

## 1. 基本信息

- `ticket_id`: TICKET-0007
- `title`: Implement controlapi skeleton with strategy/risk config endpoints
- `owner_agent`: impl-agent
- `related_module`: internal/controlapi
- `priority`: P1
- `status`: accepted

## 2. 变更范围（必须精确）

- `allowed_write_paths`: `internal/controlapi/*`
- `forbidden_paths`: direct mutation of execution/orderfsm/position internals
- `out_of_scope`: auth integration and production RBAC

## 3. 输入与依赖

- `input_docs`: `controlapi.md`, `service-spec.md`
- `upstream_tickets`: `TICKET-0001` ~ `TICKET-0006`
- `external_constraints`: none

## 4. 验收条件（Definition of Done）

1. 代码实现项：health + strategy start/stop/config + risk config endpoints
2. 测试通过项：handler unit tests
3. 门禁通过项：quality gates pass

## 5. 风险与回滚

- `risk_level`: medium
- `rollback_plan`: revert controlapi module changes
- `hitl_required`: no

## 6. 输出产物

1. 代码变更清单
2. 测试报告
3. API 变更说明

## 7. 执行记录

1. Implemented HTTP control API skeleton for health, strategy start/stop/config, and risk config update.
2. Added handler unit tests and risk-config propagation test.
3. Passed enabled quality gates including integration/replay/perf test runs.
