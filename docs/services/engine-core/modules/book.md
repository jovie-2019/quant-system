# 模块规范：book

- Package: `internal/book`
- 目标：维护本地 orderbook 一致状态并输出快照/增量。

## 1. 职责

1. 按 `venue+symbol` 重建并维护本地簿。
2. 处理消息乱序、重复、缺口。
3. 提供“最新一致快照”给 `hub`。

## 2. 边界

1. 不做交易决策。
2. 不写持久化数据库。
3. 不向交易所发请求（除必要快照拉取由 adapter 提供）。

## 3. 接口建议

```go
type BookEngine interface {
    Apply(evt MarketNormalizedEvent) (BookUpdate, error)
    Snapshot(key VenueSymbol) (BookSnapshot, bool)
}
```

## 4. 不变量

1. 每个分片内 `seq` 必须单调校验。
2. 检测到缺口后标记 `book_stale=true`，完成重同步后再恢复。
3. 对外只发布一致快照，不发布中间不一致状态。

## 5. 失败与降级

1. 缺口：触发重同步流程并告警。
2. 重同步失败：维持 stale 标记，策略层可配置拒绝交易。

## 6. 指标与日志

1. 指标：`book_seq_gap_count`、`book_stale_ms`、`book_apply_latency_ms`
2. 日志：缺口信息、重同步开始/结束、失败原因

## 7. 测试重点

1. 乱序/重复/缺包场景一致性。
2. 重同步前后状态切换。
3. 高并发符号分片稳定性。

## 8. 当前实现进度（2026-03-27）

1. 已完成：`internal/book` 本地簿引擎（snapshot + incremental）。
2. 已完成：序列异常处理（duplicate/out-of-order/gap）与 `stale` 标记。
3. 已完成：strategy 侧读取接口（runtime 可注入 `BookReader` 并读取快照）。
4. 已完成：replay 场景测试覆盖乱序/重复/缺口。
