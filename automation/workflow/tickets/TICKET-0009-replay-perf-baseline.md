# TICKET-0009

## 1. 基本信息

- `ticket_id`: TICKET-0009
- `title`: Add replay/perf baseline scaffolding and acceptance report
- `owner_agent`: perf-agent
- `related_module`: test/replay, test/perf, automation/reports
- `priority`: P0
- `status`: accepted

## 2. 变更范围（必须精确）

- `allowed_write_paths`: `test/replay/*`, `test/perf/*`, `automation/reports/*`
- `forbidden_paths`: trading business modules
- `out_of_scope`: production-grade benchmark harness

## 3. 输入与依赖

- `input_docs`: quality-gates.md, testing-matrix.md
- `upstream_tickets`: `TICKET-0008`
- `external_constraints`: none

## 4. 验收条件（Definition of Done）

1. 代码实现项：replay/perf baseline test skeleton
2. 测试通过项：local baseline report generation
3. 门禁通过项：quality gates pass with replay/perf flags

## 5. 风险与回滚

- `risk_level`: medium
- `rollback_plan`: revert replay/perf scaffolding
- `hitl_required`: no

## 6. 输出产物

1. baseline report
2. acceptance summary

## 7. 执行记录

1. Added replay determinism test scaffolding for order+position state consistency.
2. Added local performance baseline tests for risk evaluate and orderfsm apply loops.
3. Passed enabled quality gates including explicit replay/perf runs.
