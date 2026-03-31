# Post-Go-Live Backlog

- Date: `2026-03-28`
- Scope: 上线后回补（不阻塞当前可观测上线主路径）
- Status: `deferred_until_go_live`

## Deferred Tickets

1. `TICKET-0901` `deferred`
- title: Adapter depth stream support (L2 20/50 levels for Binance/OKX)
- why: 吃单套利需要真实档位深度，单纯 ticker 不够
- done_when:
1. Binance/OKX 深度流都可接入（快照+增量）
2. 支持至少 `top20`，可切换 `top50`
3. 深度数据断线重建可自动恢复

2. `TICKET-0902` `deferred`
- title: Book robustness in live path (snapshot + incremental + gap recover)
- why: 盘口一致性是报价和执行的基础
- done_when:
1. 本地 orderbook 可检测并修复 seq gap
2. 断连重连后可快速回到一致状态
3. 提供 stale 状态暴露给策略和监控

3. `TICKET-0903` `deferred`
- title: QuoteEngine for taker routing (size -> executable price curve)
- why: 决定“吃到哪个档位最划算”的核心引擎
- done_when:
1. 输入 `symbol + qty + side` 输出 `预估成交均价/滑点/可成交量`
2. 支持多交易所比较并给出最优 venue
3. 计算包含交易费和最小交易单位约束

4. `TICKET-0904` `deferred`
- title: Fee + instrument metadata registry
- why: 不引入费率和最小交易单位会导致报价偏差
- done_when:
1. 统一管理 `tick_size/lot_size/min_notional/fee_rate`
2. 支持按 venue + symbol 查询
3. 配置变更可审计且可回滚

5. `TICKET-0905` `deferred`
- title: IndexEngine (internal fair index and venue spread index)
- why: 套利判断需要稳定参考价，不只看单点盘口
- done_when:
1. 输出 `fair_index`（多 venue 中间价融合）
2. 异常值剔除和失效 venue 降权
3. 输出可直接供策略消费的指数流

6. `TICKET-0906` `deferred`
- title: Market observability enhancement
- why: 需可视化监控盘口健康和延迟抖动
- done_when:
1. 增加指标：`book_stale_ms`, `depth_gap_count`, `effective_spread_bps`, `quote_latency_ms`
2. Grafana 增加 depth/quote 专项看板
3. 异常阈值告警可触发并回放定位

## Execution Rule

1. 仅在“可观测 + k8s 上线链路”完成后启动本清单。
2. 启动顺序建议：`0901 -> 0902 -> 0904 -> 0903 -> 0905 -> 0906`。
3. 每个票据仍需走既有 HITL/Gate 规则。
