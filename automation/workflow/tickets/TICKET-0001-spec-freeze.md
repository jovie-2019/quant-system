# TICKET-0001

## 1. 基本信息

- `ticket_id`: TICKET-0001
- `title`: Freeze V1 Spec and Contracts for first coding wave
- `owner_agent`: spec-agent
- `related_module`: docs/services/*
- `priority`: P0
- `status`: accepted

## 2. 变更范围（必须精确）

- `allowed_write_paths`: `docs/services/*`, `docs/architecture-v1.md`
- `forbidden_paths`: `internal/*`, `cmd/*`, `deploy/*`
- `out_of_scope`: runtime implementation

## 3. 输入与依赖

- `input_docs`: architecture-v1, service-spec, event-contracts-v1
- `upstream_tickets`: none
- `external_constraints`: single-auditor readability first

## 4. 验收条件（Definition of Done）

1. 文档更新项：模块边界、事件契约、HITL 触发条件可执行
2. 代码实现项：none
3. 测试通过项：文档一致性检查通过
4. 性能约束：none

## 5. 风险与回滚

- `risk_level`: low
- `rollback_plan`: revert docs delta only
- `hitl_required`: no

## 6. 输出产物

1. 文件变更列表
2. 规格冻结说明
3. 下游实现输入清单

## 7. 执行记录

1. Added `docs/services/spec-freeze-v1.md`.
2. Freeze baseline aligned with architecture and module boundaries.
