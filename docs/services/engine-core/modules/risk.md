# 模块规范：risk

- Package: `internal/risk`
- 目标：作为唯一风控入口，对 `OrderIntent` 做准入判定。

## 1. 职责

1. 规则校验：账户限额、单笔上限、价格偏离、频率限制。
2. 输出决策：`Allow` 或 `Reject`，并记录规则命中信息。
3. 将决策事件写入审计链路（异步）。

## 2. 边界

1. 不直接下单。
2. 不修改订单状态。
3. 不维护仓位真相（只读仓位快照用于判断）。

## 3. 接口建议

```go
type RiskEngine interface {
    Evaluate(ctx context.Context, intent OrderIntent) RiskDecision
}
```

## 4. 不变量

1. 同一 `intent_id` 必须幂等返回同一决策结果。
2. 拒单必须给出 `rule_id` 与 `reason_code`。
3. 风控规则版本必须可追踪。

## 5. 失败与降级

1. 依赖数据缺失（如仓位未知）时默认 `Reject`（Fail Closed）。
2. 规则执行超时默认 `Reject` 并告警。

## 6. 指标与日志

1. 指标：`risk_allow_count`、`risk_reject_count`、`risk_eval_latency_ms`
2. 日志：拒单明细、规则版本、输入摘要

## 7. 测试重点

1. 规则边界值与冲突规则优先级。
2. 幂等判断。
3. 数据缺失下 Fail Closed 行为。
