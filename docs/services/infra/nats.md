# 基础设施规范：NATS（JetStream）

- Role: 轻量异步事件总线（审计、回放、异步解耦）
- Scope: 非阻塞主链路，保证关键事件可追溯

## 1. 使用边界

1. NATS 用于异步事件，不承载同步下单决策链路。
2. NATS 故障不得阻塞交易热路径。
3. 关键事件通过 JetStream 持久化，支持回放和审计。

## 2. Subject 规划（V1）

1. `market.normalized.spot.*`
2. `strategy.intent.*`
3. `risk.decision.*`
4. `order.lifecycle.*`
5. `trade.fill.*`
6. `audit.ops.*`

## 3. Stream 与 Consumer 建议

1. `STREAM_MARKET`：`market.normalized.spot.>`
2. `STREAM_TRADING`：`strategy.intent.>`、`risk.decision.>`、`order.lifecycle.>`、`trade.fill.>`
3. `STREAM_AUDIT`：`audit.ops.>`
4. 消费者使用 durable consumer，启用 `AckExplicit`。

## 4. 投递语义与重试

1. 语义：至少一次投递（At-least-once）+ 业务幂等。
2. 失败重试：指数退避，超过阈值进入死信 subject（DLQ）。
3. 幂等键建议：
- 订单事件：`client_order_id + state_version`
- 成交事件：`trade_id`

## 5. 保留与回放

1. 审计相关 stream 建议保留至少 7~30 天（按成本调优）。
2. 回放服务通过 durable consumer 从指定序号/时间点重放。

## 6. 监控指标

1. `nats_publish_error_rate`
2. `nats_consumer_ack_pending`
3. `nats_stream_bytes`
4. `nats_stream_lag_seconds`

## 7. 当前实现进度（2026-03-27）

1. 已完成：`internal/bus/natsbus` 客户端封装（connect/ensure-stream/publish/durable-subscribe）。
2. 已完成：subject builder 与 typed publish helper（market/risk/order/fill）。
3. 已完成：replay 入口 `ReplayTradeFill`（基于 JetStream pull consumer）。
4. 已完成：单测与可选 live NATS 集成测试（`RUN_NATS_TESTS=1`）。

运行说明：

1. 依赖 JetStream 能力；本地默认 `brew services start nats-server` 不一定启用 JetStream。
2. 运行集成测试时建议使用 `nats-server -js -m 8222` 模式。
