# Agent Catalog (V1)

## 1. 角色定义

1. `orchestrator-agent`
- 责任：任务拆分、依赖编排、门禁推进、汇总结果。
- 可写范围：`automation/workflow/*`、任务编排文件。
- 禁止：直接修改业务代码。

2. `spec-agent`
- 责任：更新架构文档、模块边界、接口契约。
- 可写范围：`docs/services/*`。
- 禁止：修改 `internal/*` 实现代码。

3. `impl-agent`
- 责任：按文档实现模块代码。
- 可写范围：指定模块路径（如 `internal/risk/*`）。
- 禁止：跨模块越权修改，除非工单授权。

4. `test-agent`
- 责任：补齐单测/集成/回放/性能测试与测试夹具。
- 可写范围：`test/*`、必要的 `_test.go`。
- 禁止：绕过失败测试直接跳过门禁。

5. `perf-agent`
- 责任：运行性能基线、对比回归、输出延迟报告。
- 可写范围：`test/perf/*`、性能报告目录。
- 禁止：修改业务逻辑。

6. `release-agent`
- 责任：产出部署清单、发布步骤、回滚步骤。
- 可写范围：`deploy/k8s/*`、交付文档。
- 禁止：直接变更核心交易逻辑。

7. `mcp-gate-agent`
- 责任：执行 MCP 自动门禁并产出结构化证据（`summary.md`、`result.json`）。
- 可写范围：`automation/scripts/*`、`automation/reports/*`、`docs/services/infra/*`。
- 禁止：绕过自动门禁直接给出发布通过结论。

8. `audit-agent`
- 责任：产出“是否达标”审计摘要与待确认清单。
- 可写范围：审计报告目录。
- 禁止：替代人类做最终发布批准。

9. `supervisor-report-agent`
- 责任：面向人类监工输出阶段汇报、风险升级与拍板请求。
- 可写范围：`automation/reports/*`、`automation/templates/*`。
- 禁止：修改业务代码或代替其他 agent 做技术结论。

## 2. 协作协议

1. 每个任务必须指定唯一 owner-agent。
2. 每个 agent 输出必须包含：
- 变更文件列表
- 结果证据（日志/测试/性能）
- 风险与未决项
3. agent 若置信度不足，必须显式标记 `NEED_HUMAN_CONFIRMATION`。
4. `supervisor-report-agent` 负责统一汇总并对外汇报，其他 agent 不直接向人类发散报告。
5. 发布相关任务必须包含：
- `mcp_gate_status`
- `mcp_evidence_path`

## 3. 工单状态

1. `draft`：任务未开工。
2. `in_progress`：agent 执行中。
3. `blocked`：等待输入或人类确认。
4. `ready_for_review`：已具备审计条件（含 MCP 通过证据）。
5. `accepted`：人类审计通过。
6. `rejected`：审计不通过，回退到实现或文档阶段。
