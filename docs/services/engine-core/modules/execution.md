# 模块规范：execution

- Package: `internal/execution`
- 目标：作为唯一交易执行出口，负责下单/撤单编排与交易所交互。

## 1. 职责

1. 接收 `risk` 放行的意图并转为交易所订单请求。
2. 维护幂等键、防重复下单、限流和重试。
3. 接收交易所回报并转交 `orderfsm`。

## 2. 边界

1. 不做策略计算。
2. 不直接修改订单状态（只能提交事件给 `orderfsm`）。
3. 不直接修改仓位。

## 3. 接口建议

```go
type Executor interface {
    Submit(ctx context.Context, decision RiskDecision) (SubmitResult, error)
    Cancel(ctx context.Context, req CancelIntent) (CancelResult, error)
    Reconcile(ctx context.Context, req ReconcileIntent) (ReconcileResult, error)
}
```

## 4. 不变量

1. 每个 `intent_id` 只能生成一个有效 `client_order_id`。
2. 下单请求必须带超时与可观测上下文（trace）。
3. 重试必须遵守“只在可安全重试错误码重试”规则。

## 5. 失败与降级

1. 交易所超时：进入受控重试，超阈值后返回失败并告警。
2. 交易所限流：触发内部节流，保护上游。
3. 网络故障：切换只平仓模式（由控制面开关决定）。

## 6. 指标与日志

1. 指标：`execution_submit_latency_ms`、`execution_error_rate`、`execution_gateway_events_total`
2. 日志：请求摘要、响应摘要、重试原因、最终结果

## 7. 测试重点

1. 幂等性（重复 intent 不重复下单）。
2. 错误码分层重试。
3. 限流触发与恢复。
4. 对账恢复（`client_order_id` / `venue_order_id` 查单）。
