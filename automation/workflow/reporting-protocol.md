# Reporting Protocol (Supervisor Mode)

本协议定义 `supervisor-report-agent` 如何向你（监工）汇报。

## 1. 汇报类型

1. `status-report`
- 用途：阶段进展同步，不需要立即拍板。

2. `decision-request`
- 用途：触发 HITL 门禁，必须由你给出决策。

## 2. 触发时机

1. 每个 workflow 阶段结束后发送一次 `status-report`。
2. 命中任一 HITL Gate 时立即发送 `decision-request`。
3. 连续阻塞超过 30 分钟时追加一次 `status-report`（含阻塞详情）。
4. MCP 门禁失败时必须在 5 分钟内发送 `decision-request`。

## 3. 报告内容要求

每次报告必须包含：

1. `current_phase`
2. `overall_status`（on_track/at_risk/blocked）
3. `plain_summary`（通俗一句话：这一步做完了什么、为什么重要）
4. `completed_items`
5. `open_risks`
6. `next_actions`
7. `need_human_decision`（yes/no）
8. `mcp_gate_status`（pass/fail/not_run）
9. `mcp_evidence_path`（若已执行）
10. `gateway_checklist_status`（仅交易网关相关工单：pass/fail/not_run）
11. `gateway_evidence_path`（仅交易网关相关工单）

## 4. 拍板请求要求

`decision-request` 必须额外包含：

1. 需要你拍板的问题陈述（单句）
2. 推荐选项（A/B）
3. 每个选项的风险与影响
4. 建议截止时间（deadline）
5. 默认策略（超时未答时的安全处理）
6. MCP 失败项列表与豁免建议（若存在）

默认策略（已确认）：

1. 若你在截止时间前未回复，自动执行 `NO-GO`。
2. 流水线进入 `blocked` 状态并暂停后续交付动作。
3. `supervisor-report-agent` 发送一条超时汇报，等待你明确恢复指令。

## 5. 监工最小工作量原则

1. 每份报告控制在 1 页内。
2. 不展示实现细节，只展示决策信息。
3. 如果无需拍板，不向你抛开放性问题。

## 6. 通俗汇报规则（新增）

1. 每次阶段汇报开头必须有“给监工的一句话”：不用术语，先说结果。
2. 每个“已完成”条目按固定格式写：`做了什么 -> 带来什么价值 -> 下一步是什么`。
3. 如必须使用术语（如 replay、durable consumer），同一行给出中文白话解释。
4. 每个执行环节结束后，必须追加“本步总结（通俗版）”，长度 3~5 行。
