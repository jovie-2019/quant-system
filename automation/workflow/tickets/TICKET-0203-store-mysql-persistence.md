# TICKET-0203

## 1. 基本信息

- `ticket_id`: TICKET-0203
- `title`: Add MySQL persistence for order/position/risk audit records
- `owner_agent`: impl-agent
- `related_module`: internal/store, internal/orderfsm, internal/position, internal/risk
- `priority`: P0
- `status`: accepted

## 2. 变更范围（必须精确）

- `allowed_write_paths`: `internal/store/*`, `internal/orderfsm/*`, `internal/position/*`, `internal/risk/*`, `deploy/k8s/*`, `docs/services/infra/mysql.md`
- `forbidden_paths`: strategy signal logic
- `out_of_scope`: OLAP/BI data warehouse

## 3. 输入与依赖

- `input_docs`: mysql.md, quality-gates.md
- `upstream_tickets`: `TICKET-0201`
- `external_constraints`: schema migration must be repeatable

## 4. 验收条件（Definition of Done）

1. 定义最小持久化 schema（orders/positions/risk_decisions）。
2. 提供 store repository 与幂等写入语义。
3. 增加重启恢复场景测试。

## 5. 风险与回滚

- `risk_level`: high
- `rollback_plan`: keep in-memory path + disable persistence feature flag
- `hitl_required`: yes (`Gate 2`)

## 6. 输出产物

1. `internal/store/mysqlstore/*`
2. `internal/store/schema/*`
3. `docs/services/infra/mysql.md`（补充 schema 与恢复说明）

## 7. 执行记录

1. Implemented MySQL schema and repository under `internal/store/mysqlstore`.
2. Added upsert/get/load support for orders/positions/risk_decisions with idempotent key semantics.
3. Added optional live MySQL recovery integration test (`RUN_MYSQL_TESTS=1`) and passed against local `quant` database.
4. Full quality gates passed after persistence changes.
