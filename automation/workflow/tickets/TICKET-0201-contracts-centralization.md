# TICKET-0201

## 1. 基本信息

- `ticket_id`: TICKET-0201
- `title`: Centralize core event/data contracts into pkg/contracts
- `owner_agent`: impl-agent
- `related_module`: pkg/contracts, internal/*
- `priority`: P0
- `status`: accepted

## 2. 变更范围（必须精确）

- `allowed_write_paths`: `pkg/contracts/*`, `internal/adapter/*`, `internal/normalizer/*`, `internal/strategy/*`, `internal/risk/*`, `internal/orderfsm/*`, `internal/position/*`, `docs/services/contracts/*`
- `forbidden_paths`: venue-specific adapter implementation
- `out_of_scope`: external middleware integration

## 3. 输入与依赖

- `input_docs`: event-contracts-v1.md
- `upstream_tickets`: `TICKET-0014`
- `external_constraints`: backward compatible for existing tests

## 4. 验收条件（Definition of Done）

1. 新增统一契约包 `pkg/contracts`。
2. 至少 6 个核心模块改为使用统一契约类型（可通过 type alias 兼容）。
3. 完整质量门禁通过（unit/integration/replay/perf）。
4. 契约文档补充“单一事实源”位置。

## 5. 风险与回滚

- `risk_level`: medium
- `rollback_plan`: revert contracts extraction changes
- `hitl_required`: no

## 6. 输出产物

1. `pkg/contracts/contracts.go`
2. changed modules under `internal/*`
3. updated `docs/services/contracts/event-contracts-v1.md`

## 7. 执行记录

1. Added canonical contracts in `pkg/contracts/contracts.go`.
2. Refactored adapter/normalizer/strategy/risk/orderfsm/position to use shared contracts (compat alias style).
3. Re-ran quality gates with integration/replay/perf enabled; all passed.
