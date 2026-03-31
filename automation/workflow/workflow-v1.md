# Workflow V1 (Automation First, Human Final)

## 0. 目标

将“开发 -> 测试 -> 验收 -> 交付”标准化为可重复流水线，
人类只做高风险门禁审批，不参与机械检查。

并通过 `supervisor-report-agent` 获得统一进度汇报与拍板请求。

新增约束：

1. 发布相关验收必须先通过 `MCP` 自动门禁（见 `mcp-charter-v1.md`）。
2. 未通过 MCP，默认 `NO-GO`，不得进入发布拍板。

## 1. 阶段一：任务编排（Orchestrator）

输入：

1. 用户需求
2. 当前架构文档（`docs/services/*`）

动作：

1. 生成任务工单（模板见 `task-ticket-template.md`）
2. 为每个工单分配唯一 owner-agent
3. 标注写入边界、依赖关系、通过条件

退出条件：

1. 所有工单具备 `scope/owner/gates` 三元信息
2. 生成阶段汇报并发送给人类监工

## 2. 阶段二：规格先行（Spec）

owner：`spec-agent`

动作：

1. 更新模块文档、契约文档
2. 运行 `skill-spec-guard` 与 `skill-contract-guard`

退出条件：

1. 文档变更完整
2. 无未解释边界漂移
3. 若有破坏性契约变更，进入 HITL
4. 生成阶段汇报并发送给人类监工

## 3. 阶段三：实现与测试（Impl/Test）

owners：`impl-agent`、`test-agent`

动作：

1. 按工单边界实现代码
2. 补齐对应测试（单测/集成/回放/性能）
3. 执行质量门禁脚本
4. 若涉及交易网关改动（`internal/adapter/*rest*` 或 `internal/execution/*`），执行 `trade-gateway-hardening-checklist-v1.md`

退出条件：

1. 无越权修改
2. 质量门禁通过
3. 交易网关相关工单的 checklist 结果为 `pass` 或已登记临时豁免
4. 生成阶段汇报并发送给人类监工

## 4. 阶段四：性能与发布准备（Perf/Release）

owners：`perf-agent`、`release-agent`

动作：

1. 执行性能回归对比
2. 生成 k8s 发布与回滚清单

退出条件：

1. 性能未回退超阈值
2. 交付清单完整可执行
3. 生成阶段汇报并发送给人类监工

## 5. 阶段四点五：MCP 自动门禁（Machine Conformance Gate）

owners：`mcp-gate-agent`、`release-agent`

动作：

1. 执行 `automation/scripts/mcp_observability_gate.sh`
2. 产出 `summary.md`、`result.json`、查询日志与环境快照
3. 将 MCP 证据路径写入审计包

退出条件：

1. MCP 结果为 `pass`
2. 证据目录完整且可追溯
3. 若 MCP 失败，流水线进入 `blocked`

## 6. 阶段五：审计与人类确认（Audit/HITL）

owners：`audit-agent` + Human

动作：

1. 生成审计包（模板见 `templates/review-pack`）
2. 对照 HITL 清单逐项确认
3. 核对 MCP 结果与证据路径

退出条件：

1. 人类给出 `GO` 或 `NO-GO`

## 7. 阶段六：交付执行（Delivery）

owner：`release-agent`

动作：

1. 按已批准发布单执行
2. 验证健康指标与核心交易链路
3. 触发发布后回顾

退出条件：

1. 发布完成并稳定运行
2. 文档和变更记录归档

## 8. 汇报与升级通道（Supervisor Report）

owner：`supervisor-report-agent`

动作：

1. 每个阶段结束后生成 `status report`
2. 发现阻塞时生成 `decision request`（请求人类拍板）
3. 汇总当天变更与剩余风险

输出要求：

1. 每份报告最多 1 页
2. 必须包含：当前状态、已完成、阻塞项、下一步、是否需要拍板
3. 若需要拍板，必须给出推荐选项和风险差异
4. 每份报告开头必须有“通俗一句话总结”，不用术语先说结论
5. 每个执行环节结束后必须追加“本步总结（通俗版，3~5 行）”
6. 涉及发布审批时必须包含：`mcp_gate_status` 与 `mcp_evidence_path`
7. 涉及交易网关改动时必须包含：`gateway_checklist_status` 与 `gateway_evidence_path`
