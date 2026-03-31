# 模块规范：hub

- Package: `internal/hub`
- 目标：作为标准行情分发中心，向策略提供低开销订阅。

## 1. 职责

1. 接收 `book/normalizer` 输出的标准行情事件。
2. 缓存每个 `venue+symbol` 的最新快照。
3. 向策略模块推送增量或快照更新。

## 2. 边界

1. 不产生交易信号。
2. 不做风控判断。
3. 不直接下单。

## 3. 接口建议

```go
type MarketHub interface {
    Publish(evt MarketNormalizedEvent)
    Subscribe(strategyID string, symbols []string) (<-chan MarketNormalizedEvent, error)
    GetSnapshot(key VenueSymbol) (BookSnapshot, bool)
}
```

## 4. 不变量

1. 同一分片内事件按接收顺序分发。
2. 慢消费者不能阻塞全局分发，需隔离背压。
3. 快照读取必须无锁或低锁争用。

## 5. 失败与降级

1. 策略订阅通道拥塞：丢弃策略侧消息并记录告警（可配置）。
2. 快照缺失：返回显式不可用状态，策略不得盲目下单。

## 6. 指标与日志

1. 指标：`hub_publish_rate`、`hub_subscriber_lag`、`hub_drop_count`
2. 日志：订阅变更、背压告警、快照缺失

## 7. 测试重点

1. 多策略订阅隔离。
2. 慢消费者背压策略。
3. 高频行情下分发延迟。
