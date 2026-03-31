# Automation System (V1)

本目录定义项目自动化开发体系：`agents + skills + workflow + templates + scripts`。
目标是让你作为唯一审计者，只在高风险节点做人类确认。

新增原则：

1. 发布验收采用 `MCP`（Machine Check First）优先。
2. 人类只对 MCP 结果做最终拍板，不执行机械校验。

## 目录

1. `agents/agent-catalog.md`
- Agent 角色、写入边界、输入输出。

2. `skills/skill-catalog.md`
- Skill 定义、触发条件、产出物要求。

3. `workflow/workflow-v1.md`
- 端到端流水线（开发/测试/验收/交付）。

4. `workflow/hitl-gates.md`
- Human-in-the-loop 强制确认点。

5. `workflow/reporting-protocol.md`
- 监工汇报协议（状态汇报、拍板请求、升级规则）。

6. `workflow/mcp-charter-v1.md`
- MCP 门禁章程（自动验收标准、证据规范、豁免策略）。

7. `workflow/task-ticket-template.md`
- 任务拆分模板（供 orchestrator 使用）。

8. `templates/review-pack/review-pack-template.md`
- 审计包模板（供 audit-agent 输出）。

9. `templates/reporting/*.md`
- 监工汇报模板（状态汇报 + 决策请求）。

10. `scripts/run_quality_gates.sh`
- 质量门禁入口脚本（本地/CI）。

11. `scripts/mcp_observability_gate.sh`
- MCP 自动观测门禁脚本（输出结构化证据）。

12. `scripts/phase14_rehearsal.sh`
- phase-14 一键演练（部署/观测/日志/MCP）与证据归档。

13. `scripts/build_engine_core_image.sh`
- 构建（可选推送）`engine-core` 镜像。

14. `scripts/build_market_ingest_image.sh`
- 构建（可选推送）`market-ingest` 镜像。

15. `scripts/k8s_deploy_dev.sh`
- 在 k8s dev 环境部署 `engine-core+strategy-runner+market-ingest+nats+mysql` 并等待 rollout。

16. `scripts/k8s_bootstrap_observability.sh`
- 安装 `kube-prometheus-stack` 并接入指标采集/告警规则，自动导入 Grafana 看板（支持 `OBS_ROLLOUT_TIMEOUT`，失败时输出 deployment/pod/event 诊断）。

17. `scripts/grafana_import_dashboards.sh`
- 将 `engine-core-v1`、`engine-k8s-status-v1` dashboard 以 ConfigMap 导入 Grafana sidecar。

18. `scripts/k8s_bootstrap_logging.sh`
- 安装 Fluent Bit 日志采集基线（SLS 参数化）。

19. `scripts/k8s_smoke_status.sh`
- 输出 k8s 服务状态与观测对象状态。

20. `scripts/verify_grafana_data.sh`
- legacy 数据有效性检查脚本（已并入 MCP 主流程）。

## 使用顺序（最小闭环）

1. Orchestrator 按 `task-ticket-template.md` 拆任务。
2. Spec 与 Impl/Test Agent 并行执行。
3. 执行 `scripts/run_quality_gates.sh`。
4. 执行 `scripts/mcp_observability_gate.sh`（发布前必过）。
5. 由 Audit Agent 产出审计包（模板见 review-pack）。
6. 由 Supervisor Report Agent 生成阶段汇报或拍板请求。
7. 人类在 HITL 门禁做确认后才能进入交付。
