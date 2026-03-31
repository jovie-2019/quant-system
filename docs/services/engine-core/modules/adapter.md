# 模块规范：adapter

- Package: `internal/adapter`
- 目标：对接交易所行情与交易 API，向内部输出统一“原始事件/执行回报”。

## 1. 职责

1. 维护 Binance/OKX 的 WS 连接、鉴权、订阅、心跳。
2. 处理断连重连、指数退避、订阅恢复。
3. 提供交易下单/撤单/查询最小封装（给 `execution` 调用）。
4. 输出原始行情消息与原始执行回报给下游。

## 2. 边界

1. 不做策略逻辑。
2. 不做风控判断。
3. 不写订单状态和仓位。

## 3. 接口建议

```go
type MarketStream interface {
    Subscribe(ctx context.Context, symbols []string) (<-chan RawMarketEvent, error)
}

type TradeGateway interface {
    PlaceOrder(ctx context.Context, req VenueOrderRequest) (VenueOrderAck, error)
    CancelOrder(ctx context.Context, req VenueCancelRequest) (VenueCancelAck, error)
}

type OrderQueryGateway interface {
    QueryOrder(ctx context.Context, req VenueOrderQueryRequest) (VenueOrderStatus, error)
}
```

## 4. 不变量

1. 每条上游消息必须带 `venue`、`symbol`、`source_ts_ms`。
2. 重连后必须重建订阅并上报重连事件。
3. 同一连接内消息顺序保持原样，不做重排。

## 5. 失败与降级

1. WS 断开：退避重连，连续失败触发告警。
2. 下单失败：返回明确错误码给 `execution`，不吞错。
3. 交易所限流：暴露限流状态供 `execution` 调整节流。

## 6. 指标与日志

1. 指标：`ws_reconnect_count`、`ws_alive`、`venue_api_error_rate`
2. 日志：连接状态变化、订阅恢复、交易所错误响应（带 `trace_id`）

## 7. 测试重点

1. 断连重连与订阅恢复。
2. 交易所错误码映射。
3. 下单/撤单幂等请求透传。

## 8. 当前实现进度（2026-03-31）

1. 已完成：Binance/OKX spot REST 交易网关（下单/撤单/查单、签名、最小限频、错误处理）。
2. 已完成：Binance/OKX WS 行情接入基础能力（连接、重连、心跳、订阅恢复）。
3. 已完成：mock WebSocket server 重连测试与行情解析测试。
