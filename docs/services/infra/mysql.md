# 基础设施规范：MySQL

- Role: 持久化真相源
- Scope: 订单、成交、仓位、配置版本、审计关键记录

## 1. 使用边界

1. MySQL 是状态真相源，不是实时消息总线。
2. 高频热路径不依赖同步 SQL 查询做决策。
3. 状态变更以事务保证一致性，读路径可使用只读副本。

## 2. 核心表（V1）

1. `orders`
- 主键：`client_order_id`
- 关键字段：`venue_order_id`、`symbol`、`state`、`filled_qty`、`avg_price`、`state_version`、`updated_ms`
2. `positions`
- 主键：`(account_id, symbol)`
- 关键字段：`quantity`、`avg_cost`、`realized_pnl`、`updated_ms`
3. `risk_decisions`
- 主键：`intent_id`
- 关键字段：`strategy_id`、`symbol`、`side`、`price`、`quantity`、`decision`、`rule_id`、`reason_code`、`evaluated_ms`

说明：

1. V1 已落地最小集合 `orders/positions/risk_decisions`，用于恢复订单状态、仓位与风控审计轨迹。
2. `fills/strategy_config_versions/venue_offsets` 作为下一迭代扩展表。

## 3. 一致性要求

1. 订单状态变更与版本号更新同事务提交。
2. 成交入账与仓位更新需保证幂等。
3. 关键唯一键：
- `orders(client_order_id)`
- `risk_decisions(intent_id)`

## 4. 性能要求

1. 写入走批量化与异步落库队列。
2. 索引只保留高频查询字段，避免过度索引。
3. 慢查询必须可观测并告警。

## 5. 高可用要求（K8s）

1. 优先托管数据库；自建需主从+备份+恢复演练。
2. 定期备份，最小恢复演练周期每周一次。

## 6. 监控指标

1. `mysql_qps`、`mysql_tps`
2. `mysql_slow_query_count`
3. `mysql_replication_lag`
4. `mysql_conn_usage`

## 7. 恢复流程（V1）

1. 服务启动后先执行 schema ensure（`CREATE TABLE IF NOT EXISTS`）。
2. 启动恢复阶段：
- 加载 `orders` 重建订单状态快照
- 加载 `positions` 重建持仓快照
- 加载 `risk_decisions` 重建风控决策审计视图
3. 恢复完成后才切换到实时处理阶段，避免“先交易后恢复”。
