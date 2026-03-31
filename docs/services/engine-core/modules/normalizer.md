# 模块规范：normalizer

- Package: `internal/normalizer`
- 目标：将不同交易所原始消息标准化为统一事件结构。

## 1. 职责

1. 字段映射：交易所字段映射到统一契约。
2. 精度统一：价格、数量、成交额统一小数策略。
3. 时间处理：补全 `ingest_ts_ms`、`emit_ts_ms`。
4. 事件版本化：输出 `event_version=v1`。

## 2. 边界

1. 不维护 orderbook 状态。
2. 不做策略、风控、执行逻辑。
3. 不直接写 NATS/MySQL。

## 3. 接口建议

```go
type Normalizer interface {
    NormalizeMarket(raw RawMarketEvent) (MarketNormalizedEvent, error)
    NormalizeExec(raw RawExecEvent) (OrderLifecycleEvent, error)
}
```

## 4. 不变量

1. 标准化失败必须返回结构化错误，不允许 panic。
2. 标准化输出必须包含 `venue/symbol/seq`（若上游缺失则补策略）。
3. 精度处理必须可配置且可测试。

## 5. 失败与降级

1. 单条消息解析失败只丢弃该消息并计数，不影响主循环。
2. 连续解析失败超过阈值触发告警并可触发 venue 熔断。

## 6. 指标与日志

1. 指标：`normalize_ok_count`、`normalize_error_count`、`normalize_latency_ms`
2. 日志：错误原始片段（脱敏）、错误码、schema版本

## 7. 测试重点

1. 字段映射正确性（Binance/OKX 各类型消息）。
2. 精度舍入边界值。
3. 非法数据容错与错误分类。
