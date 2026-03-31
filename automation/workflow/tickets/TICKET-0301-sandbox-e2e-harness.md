# TICKET-0301

## 1. 基本信息

- `ticket_id`: TICKET-0301
- `title`: Build sandbox end-to-end integration harness (market -> strategy -> risk -> execution -> fill -> state consistency)
- `owner_agent`: impl-agent
- `related_module`: test/integration, test/sandbox, automation/scripts, docs/services/quality
- `priority`: P0
- `status`: in_progress

## 2. 变更范围（必须精确）

- `allowed_write_paths`: `test/integration/*`, `test/sandbox/*`, `automation/scripts/*`, `docs/services/quality/*`, `docs/runbook-v1.md`
- `forbidden_paths`: core trading logic math and risk rule defaults
- `out_of_scope`: futures/derivatives sandbox and production deploy

## 3. 输入与依赖

- `input_docs`: testing-matrix.md, quality-gates.md, runbook-v1.md
- `upstream_tickets`: `TICKET-0202`, `TICKET-0203`, `TICKET-0204`, `TICKET-0206`
- `external_constraints`: single-operator friendly, no proxy by default

## 4. 验收条件（Definition of Done）

1. 新增 sandbox E2E 用例入口（默认关闭，按环境变量启用）。
2. 至少覆盖真实交易所行情接入链路，并输出可审计的通过/失败证据。
3. 质量门禁脚本支持独立触发 sandbox tests。
4. 更新测试矩阵与执行说明文档。

## 5. 风险与回滚

- `risk_level`: medium
- `rollback_plan`: keep sandbox tests optional and isolated from default CI path
- `hitl_required`: no

## 6. 输出产物

1. sandbox test harness files
2. quality gate script updates
3. testing/runbook doc updates
4. phase report evidence path

## 7. 执行记录

1. Phase-13 已启动，先落地 sandbox E2E 测试基线文档与工单边界。
2. 已新增 `test/sandbox/market_ingress_test.go`，支持 Binance/OKX 实时行情接入探测（默认关闭，环境变量启用）。
3. 已更新质量门禁脚本，支持 `RUN_SANDBOX_TESTS=1` 独立触发 sandbox 测试。
4. 已更新测试矩阵、质量门禁、runbook 与 sandbox 计划文档，形成可执行说明。
5. 已新增 `test/sandbox/trade_smoke_test.go`：显式开关下执行最小交易烟测（风险放行 -> 下单 -> 撤单 -> FSM `ack -> canceled`）。
6. 已为 OKX gateway 增加 `x-simulated-trading: 1` 支持，降低误触真实环境风险。
7. 下一步产出实测证据包（在配置 sandbox 凭据后执行 live trade smoke）。
