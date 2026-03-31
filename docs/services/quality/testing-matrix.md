# 测试矩阵（V1）

## 1. 单元测试（按模块）

1. `adapter`
- 重连退避、订阅恢复、异常消息容错。

2. `normalizer`
- 字段映射正确性、精度处理、时间戳策略。

3. `book`
- 乱序、重复、缺包、快照重建一致性。

4. `strategy`
- 输入行情到 `OrderIntent` 输出的确定性。

5. `risk`
- 各规则 allow/reject 覆盖、边界值测试。

6. `execution`
- 幂等、超时、重试上限、限流行为。

7. `orderfsm`
- 所有合法状态迁移与非法迁移拒绝。

8. `position`
- 多笔成交累计成本、PnL 计算正确性。

9. `controlapi`
- 鉴权、参数校验、幂等更新。

## 2. 集成测试

1. 行情闭环：`adapter -> normalizer -> book -> hub`
2. 交易闭环：`strategy -> risk -> execution -> orderfsm -> position`
3. 异常链路：交易所断连、NATS/JetStream 不可用、MySQL 慢查询、部分模块重启

## 3. 回放测试

1. 输入固定历史行情数据集。
2. 输出订单事件序列必须与基线一致。
3. 差异报告必须定位到：策略版本、配置版本、事件偏移量。
4. 回测框架入口：`go test ./internal/backtest -count=1 -v`。

## 4. 性能测试

1. 指标：
- `market_pipeline_latency_ms`（P50/P95/P99）
- `strategy_latency_ms`（P50/P95/P99）
- `execution_latency_ms`（P50/P95/P99）

2. 阈值：
- 行情链路 `P99 < 5ms`
- 策略到触发 `P99 < 15ms`

3. 失败处理：
- 任一关键指标回退超阈值，阻断合并/发布。

## 5. 发布前验收清单

1. 全量测试通过。
2. 告警规则已更新并验证。
3. 回滚方案可执行并演练通过。
4. 文档与代码版本一致。

## 6. Sandbox E2E（phase-13）

1. 行情联调：真实 Binance/OKX WS 行情接入并完成标准化解析。
2. 交易联调：基于 sandbox 凭据执行最小下单烟测（下单+撤单+状态机一致性）。
3. 所有 sandbox 用例默认关闭，仅在 `RUN_SANDBOX_TESTS=1` 时执行。
4. 联调结果必须产出可审计摘要（成功率、失败点、下一步动作）。
