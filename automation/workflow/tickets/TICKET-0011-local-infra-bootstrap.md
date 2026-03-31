# TICKET-0011

## 1. 基本信息

- `ticket_id`: TICKET-0011
- `title`: Bootstrap local NATS and MySQL without proxy
- `owner_agent`: release-agent
- `related_module`: local runtime environment
- `priority`: P0
- `status`: accepted

## 2. 变更范围（必须精确）

- `allowed_write_paths`: local install/runtime scripts and docs updates
- `forbidden_paths`: core trading module logic
- `out_of_scope`: production database setup

## 3. 输入与依赖

- `input_docs`: architecture-v1.md, delivery-readiness-v1.md
- `upstream_tickets`: `TICKET-0010`
- `external_constraints`: direct network only, no proxy

## 4. 验收条件（Definition of Done）

1. `nats-server` and `mysql` binaries installed locally.
2. Both services started locally.
3. Connectivity checks passed (NATS port + MySQL ping/query).

## 5. 风险与回滚

- `risk_level`: medium
- `rollback_plan`: stop local services and remove local runtime artifacts
- `hitl_required`: no

## 6. 输出产物

1. local bootstrap evidence
2. service health check evidence

## 7. 执行记录

1. Installed binaries in direct-network mode (no proxy):
- `nats-server`: `2.12.6`
- `mysql`: `9.6.0`
2. Started local services:
- `brew services start nats-server`
- `brew services start mysql`
3. Connectivity and health checks passed (outside sandbox verification):
- `nc -vz 127.0.0.1 4222` -> succeeded
- `mysqladmin ping -uroot` -> `mysqld is alive`
- `mysql -uroot -e "SELECT VERSION();"` -> `9.6.0`
- `mysql -uroot -e "SHOW DATABASES LIKE 'quant';"` -> `quant`
4. Notes:
- Homebrew default `nats-server` service enables client port `4222` for V1 use.
- NATS monitor port `8222` is not enabled by default service profile; optional custom config in next ticket.
