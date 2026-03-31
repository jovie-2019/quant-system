# Review Pack (Draft)

- `change_id`: phase14-live-20260330
- `date`: 2026-03-30
- `owner_agent`: supervisor-report-agent
- `reviewer`: Human
- `status`: draft

## 1. 审计结论（Go / No-Go）

- conclusion: `TBD`
- confidence: `TBD`

## 2. 变更摘要（仅高价值信息）

1. 核心改动点：phase-14 上线演练链路升级为 `MCP 自动门禁优先`，人类只做最终拍板。
2. 影响模块：`automation/workflow/*`, `automation/scripts/*`, `docs/go-live-phase14.md`, `docs/release-checklist-v1.md`, `docs/services/infra/observability.md`。
3. 不兼容变更：无协议破坏性变更证据（待最终确认）。

## 3. 证据

1. 文档一致性检查：
- [x] workflow/hitl/reporting/agent-catalog 已接入 MCP 约束。
- [x] go-live/release-checklist 已把 MCP 设为硬门禁。
- [ ] board 与 ticket 状态已与真实执行结果完全对齐。

2. 测试结果（单测/集成/回放/性能）：
- [x] `go test ./... -count=1`。
- [x] `go test ./... -race -count=1`。
- [ ] `RUN_INTEGRATION_TESTS=1 RUN_REPLAY_TESTS=1 RUN_PERF_TESTS=1 automation/scripts/run_quality_gates.sh`（待在非受限环境复核）。

3. 关键指标对比（P50/P95/P99）：
- [ ] `engine-core-v1` 看板导出（至少一段稳定窗口）。
- [ ] `engine-k8s-status-v1` 看板导出（包含 `strategy-runner-up`）。
- [ ] `controlapi-p99` 与 `controlapi-status-rate` 查询结果快照。

4. 发布与回滚演练结果：
- [x] dry-run rehearsal: `DRY_RUN=1 automation/scripts/phase14_rehearsal.sh`
- [x] dry-run evidence: `automation/reports/phase14-rehearsal-20260330-224020/summary.md`
- [ ] live rehearsal: `automation/scripts/phase14_rehearsal.sh`
- [ ] rollback rehearsal: `RUN_ROLLBACK=1 automation/scripts/phase14_rehearsal.sh`

5. MCP 门禁结果：
- status: `not_run`（live 环境待执行）
- evidence_path: `TBD`
- failed_items: `TBD`

## 4. 风险与未决项

1. 高风险点：
- `TICKET-0404` 仍依赖真实 SLS endpoint/auth 做最终连通性验证。
- `TICKET-0301` 仍依赖 sandbox 凭据补齐 live trade smoke 证据。

2. 未解决问题：
- `TICKET-0405` 未完成，当前缺少完整 rehearsal + MCP pass 依据。

3. NEED_HUMAN_CONFIRMATION 项目：
- 是否在缺少 SLS live 证据的情况下临时放行（建议 NO）。
- 是否对 MCP 失败项做临时阈值豁免（建议默认 NO）。

## 5. 人类确认记录

1. 决策：`TBD`
2. 决策理由：`TBD`
3. 附加约束（若有）：`TBD`

## 6. 执行记录附录（待填）

1. 命令执行时间窗口：`TBD`
2. 执行人：`TBD`
3. 环境：`kind-quant-dev` / namespace=`quant-system, observability`
4. 关键输出附件路径：`TBD`

## 7. 待你确认（HITL）

1. 本次变更是否涉及交易关键路径（risk/execution/orderfsm/position）。
2. 是否存在契约破坏性变更。
3. 性能是否达标（P99 阈值）。
4. MCP 门禁结果是否满足放行条件。
5. 是否批准进入交付。
