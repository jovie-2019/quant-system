# TICKET-0008

## 1. 基本信息

- `ticket_id`: TICKET-0008
- `title`: Implement minimal integration tests for end-to-end event flow
- `owner_agent`: test-agent
- `related_module`: test/integration
- `priority`: P0
- `status`: accepted

## 2. 变更范围（必须精确）

- `allowed_write_paths`: `test/integration/*`
- `forbidden_paths`: core module business logic files
- `out_of_scope`: full exchange simulation

## 3. 输入与依赖

- `input_docs`: testing-matrix.md
- `upstream_tickets`: `TICKET-0003` ~ `TICKET-0007`
- `external_constraints`: none

## 4. 验收条件（Definition of Done）

1. 代码实现项：最小闭环集成测试场景
2. 测试通过项：pipeline happy-path + reject-path
3. 门禁通过项：quality gates pass with integration flag

## 5. 风险与回滚

- `risk_level`: medium
- `rollback_plan`: revert integration tests only
- `hitl_required`: no

## 6. 输出产物

1. 测试报告
2. 失败场景定位说明

## 7. 执行记录

1. Added end-to-end minimal integration tests for happy-path and reject-path flows.
2. Verified market->strategy->risk->execution->orderfsm->position chain behavior.
3. Passed enabled quality gates including explicit integration run.
