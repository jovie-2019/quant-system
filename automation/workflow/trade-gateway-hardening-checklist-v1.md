# Trade Gateway Hardening Checklist V1

## 0. 目标

当工单涉及 `internal/adapter/*rest*` 或 `internal/execution/*` 交易网关路径时，
强制执行本清单，避免“能下单但不可运维/不可恢复”的上线风险。

## 1. 启动条件

满足任一条件即触发：

1. 新增交易所网关或改动签名/下单/撤单请求结构。
2. 改动 `execution -> gateway` 调用语义（重试、超时、错误处理）。
3. 改动交易相关观测指标、告警或回包映射规则。

## 2. 必检项（必须全部有结论）

1. 输入校验
- 下单参数（`symbol/side/price/quantity/clientOrderID`）有明确合法域。
- 撤单参数（`clientOrderID/venueOrderID`）至少一项必填并可追溯。

2. 错误分类
- 区分 `retryable`（429/5xx/网络瞬断）与 `non-retryable`（参数错误、权限错误）。
- 错误返回可被 execution/risk 层稳定识别，不依赖字符串匹配。

3. 重试与限流
- 明确每类请求的 `timeout/retry/backoff/jitter` 策略。
- 证明策略不会导致风暴重试或重复下单。

4. 对账与恢复
- 至少具备“按 clientOrderID / venueOrderID 查询状态”的恢复路径（可在后续票据补齐实现，但必须给计划）。
- 断链恢复路径写入 runbook。

5. 观测与证据
- 暴露 gateway 关键指标：请求量、失败率、重试次数、错误类别。
- MCP/发布证据包含 gateway 健康查询或替代证据。

6. 测试矩阵
- 单测覆盖：成功、4xx、5xx、超时、上下文取消、签名失败/验签要素。
- 集成或 sandbox 覆盖至少一条真实下单-撤单链路（可在受控环境）。

7. Gateway 完整性评估
- 逐 venue 输出能力矩阵：`place/cancel/query`、`clientOrderID/venueOrderID` 支持状态。
- 明确缺口归属：`必须上线前补齐` 与 `可后续票据跟进`，并绑定 `owner + 截止日期`。
- 若存在跨模块依赖（例如 execution 重试预算、熔断、告警联动），必须在 workflow ticket 中列为显式后续步骤。

## 3. 通过标准

1. 以上 6 类必检项均为 `pass`，或存在显式豁免（含截止日期、owner、风险说明）。
2. 工单报告必须附 `gateway_checklist_status` 与证据路径。
3. 未通过时默认 `NO-GO`（不可进入发布拍板）。
4. 若存在 gateway 缺口，必须附“能力矩阵 + 补齐计划”，否则视为未通过。
