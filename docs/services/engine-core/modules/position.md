# 模块规范：position

- Package: `internal/position`
- 目标：仓位与PnL唯一真相源，仅基于成交事件更新。

## 1. 职责

1. 消费 `TradeFillEvent` 更新仓位、持仓成本、已实现PnL。
2. 提供账户与symbol维度只读查询接口。
3. 持久化仓位快照与变更流水。

## 2. 边界

1. 不从策略信号推导仓位。
2. 不写订单状态。
3. 不触发交易动作。

## 3. 接口建议

```go
type PositionLedger interface {
    ApplyFill(ctx context.Context, fill TradeFillEvent) (PositionSnapshot, error)
    Get(accountID, symbol string) (PositionSnapshot, bool)
}
```

## 4. 不变量

1. 同一 `trade_id` 只能入账一次（严格幂等）。
2. 仓位更新顺序按成交事件时间与版本控制。
3. 查询结果与持久化状态一致。

## 5. 失败与降级

1. 落库失败：进入重试队列并告警，持续失败可触发下单保护开关。
2. 数据不一致检测失败：暂停受影响账户新开仓。

## 6. 指标与日志

1. 指标：`position_apply_latency_ms`、`position_dedup_hit_count`
2. 日志：仓位变更前后、trade_id、成本变化

## 7. 测试重点

1. 成交去重与幂等。
2. 多笔成交下成本计算正确性。
3. 异常回滚与恢复一致性。
