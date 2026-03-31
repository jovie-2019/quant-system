# 模块规范：strategy

- Package: `internal/strategy`
- Runtime Service: `strategy-runner`（不在 `engine-core` 进程内运行）
- 目标：消费标准行情并产出可审计的 `OrderIntent`。

## 1. 职责

1. 加载策略实例与参数。
2. 处理行情事件并计算交易信号。
3. 输出 `OrderIntent` 给 `risk`。

## 2. 边界

1. 不直接调用交易所 API。
2. 不写订单状态与仓位。
3. 不绕过风控模块。

## 3. 接口建议

```go
type Strategy interface {
    ID() string
    OnMarket(evt MarketNormalizedEvent) []OrderIntent
}

type Runtime interface {
    Register(s Strategy) error
    Start(ctx context.Context) error
}
```

## 4. 不变量

1. 同一输入事件在同一策略版本下必须输出确定性结果。
2. 每个 `OrderIntent` 必须带 `intent_id` 和 `strategy_id`。
3. 策略参数变更必须具备版本号与生效时间。

## 5. 失败与降级

1. 策略异常 panic 要隔离，不能影响其他策略。
2. 策略超时执行应中断并计入超时指标。

## 6. 指标与日志

1. 指标：`strategy_eval_latency_ms`、`strategy_panic_count`、`strategy_intent_count`
2. 日志：参数版本、信号摘要、异常栈（脱敏）

## 7. 测试重点

1. 策略输入输出确定性。
2. 参数热更新生效顺序。
3. 多策略并发隔离。
