# 模块规范：orderfsm

- Package: `internal/orderfsm`
- 目标：订单状态唯一真相源，统一处理订单状态转移。

## 1. 职责

1. 接收执行回报并驱动订单状态迁移。
2. 校验状态转移合法性，拒绝非法迁移。
3. 将状态变更持久化到 MySQL 并异步发送 NATS。

## 2. 边界

1. 不发起交易请求。
2. 不做策略和风控判定。
3. 不计算仓位（仅输出成交事件给 `position`）。

## 3. 接口建议

```go
type OrderStateMachine interface {
    Apply(event OrderLifecycleEvent) (OrderState, error)
    Get(orderID string) (OrderState, bool)
}
```

## 4. 不变量

1. 订单状态只能沿合法图迁移。
2. 同一状态事件重复到达必须幂等处理。
3. 每次迁移必须持久化 `state_version`。

## 5. 失败与降级

1. 持久化失败：状态先落内存队列重试，超过阈值触发只读降级。
2. 非法迁移：拒绝并记录高优先级告警。

## 6. 指标与日志

1. 指标：`orderfsm_apply_latency_ms`、`orderfsm_illegal_transition_count`
2. 日志：迁移前后状态、版本号、触发事件ID

## 7. 测试重点

1. 完整状态图测试。
2. 重复事件幂等测试。
3. 持久化失败重试与恢复。
