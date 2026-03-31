# 事件契约规范（V1）

本文件定义 `engine-core` 内部与 NATS 异步事件的统一契约。  
目标是保证：模块解耦、可回放、可审计、可扩展。

## 0. 契约代码位置（单一事实源）

V1 契约类型已统一收敛到：

1. `pkg/contracts/contracts.go`

包含：

1. venue / raw event：`Venue`、`RawMarketEvent`、`RawExecEvent`
2. normalized / strategy / risk：`MarketNormalizedEvent`、`OrderIntent`、`RiskDecision`
3. execution / order / position：`VenueOrderRequest`、`OrderEvent`、`Order`、`TradeFillEvent`、`PositionSnapshot`

演进规则补充：

1. 新增事件字段先改 `pkg/contracts/contracts.go`，再改各模块实现与文档。
2. 禁止在模块内新增“平行定义”的同义结构体。

## 1. 通用 Envelope

所有事件统一外层字段：

1. `event_id`：全局唯一ID（UUID/雪花ID）
2. `event_type`：事件类型（如 `market.normalized`）
3. `event_version`：契约版本（当前 `v1`）
4. `trace_id`：链路追踪ID
5. `source`：来源模块（`adapter`/`risk`/`execution` 等）
6. `venue`：交易所（`binance`/`okx`）
7. `symbol`：交易对（标准格式，如 `BTC-USDT`）
8. `source_ts_ms`：源时间戳（交易所或上游时间）
9. `ingest_ts_ms`：本系统接收时间
10. `emit_ts_ms`：本系统发出时间

## 2. 核心业务事件

1. `MarketNormalizedEvent`
- 用途：标准化行情事件
- 关键字段：`bid_px`、`bid_sz`、`ask_px`、`ask_sz`、`last_px`、`seq`

2. `OrderIntentEvent`
- 用途：策略输出的下单意图
- 关键字段：`strategy_id`、`side`、`price`、`qty`、`time_in_force`、`intent_id`

3. `RiskDecisionEvent`
- 用途：风控结果
- 关键字段：`intent_id`、`decision(allow/reject)`、`rule_id`、`reason`

4. `OrderLifecycleEvent`
- 用途：订单生命周期变更
- 关键字段：`client_order_id`、`venue_order_id`、`state`、`filled_qty`、`avg_price`

5. `TradeFillEvent`
- 用途：成交回报
- 关键字段：`trade_id`、`fill_qty`、`fill_price`、`fee`、`liquidity_flag`

## 3. 契约演进规则

1. 新增字段只能追加，禁止重命名或复用旧字段语义。
2. 删除字段必须至少跨一个大版本并提供兼容期。
3. 事件消费者必须容忍“未知字段”。
4. 契约变更必须同步更新：
- `docs/services/contracts/event-contracts-v1.md`
- 对应模块文档
- 回放测试样例数据

## 4. Subject 路由建议（NATS）

1. 行情事件：`market.normalized.spot.{venue}.{symbol}`
2. 订单意图：`strategy.intent.{strategy_id}`
3. 风控结果：`risk.decision.{account_id}`
4. 订单生命周期：`order.lifecycle.{account_id}.{symbol}`
5. 成交事件：`trade.fill.{account_id}.{symbol}`

## 5. 幂等规则

1. `event_id` 全局幂等键。
2. 订单事件额外使用 `client_order_id + state_version` 幂等。
3. 成交事件额外使用 `trade_id` 幂等。
