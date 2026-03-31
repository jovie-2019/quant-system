# Quant System Technical Architecture V1

- Version: `v1.1`
- Date: `2026-03-24`
- Scope: 现货优先（Binance/OKX），后续扩展合约与更多交易所
- Runtime: `Go`（实盘全链路）
- Middleware: `MySQL + NATS`
- Orchestration: `Kubernetes`
- Service Docs: `docs/services/`

## 1. 设计目标

1. 行情中心内部链路低延迟，目标 `P99 < 5ms`（不含交易所外网波动）。
2. 策略到下单触发链路目标 `P99 < 15ms`（不含交易所外网波动）。
3. 代码与模块边界清晰，便于单人审计和长期维护。
4. 可扩展到第三/第四家交易所，不改核心策略接口。
5. 可观测性完整：指标、日志、链路可追踪。

## 2. 总体架构

采用“核心双服务 + 基础中间件”的精简架构：

- `engine-core`：承载交易内核（风控、执行、订单状态机、仓位、控制面）。
- `strategy-runner`：独立策略运行时，消费标准行情并产出 `OrderIntent`。
- `MySQL`：唯一持久化真相源（订单、成交、仓位、配置版本）。
- `NATS JetStream`：轻量异步事件总线（审计、回放、异步解耦）。
- `Prometheus + Grafana`：指标采集与可视化。
- `Fluent Bit -> SLS`：结构化日志采集与检索。

控制面使用 `REST`；策略与交易内核通过 NATS 契约解耦，避免策略发布影响交易内核稳定性。

## 3. 模块职责与边界

## 3.1 模块清单（engine-core）

1. `adapter`
- 职责：交易所 WS/REST 接入、心跳、重连、原始消息校验。
- 边界：不做策略、风控、订单状态写入。

2. `normalizer`
- 职责：交易所字段映射到统一模型；统一精度、时间戳、枚举。
- 边界：不维护仓位、不下单。

3. `book`
- 职责：本地 orderbook 重建；处理乱序、重复、缺包；输出一致快照。
- 边界：不做信号生成。

4. `hub`
- 职责：标准行情分发、缓存最新快照、提供策略订阅接口。
- 边界：不做交易决策。

5. `strategy`（运行在 `strategy-runner`）
- 职责：读取标准行情，产出 `OrderIntent`。
- 边界：不能直接调用交易所 API。

6. `risk`
- 职责：唯一风控入口；校验限额、频率、白黑名单、价格偏离。
- 边界：不直接下单。

7. `execution`
- 职责：唯一交易出口；负责幂等、限流、重试、超时控制。
- 边界：不改仓位，不绕过订单状态机。

8. `orderfsm`
- 职责：唯一订单状态真相源（`New/Ack/Partial/Filled/Canceled/Rejected`）。
- 边界：禁止其他模块直接改订单状态。

9. `position`
- 职责：唯一仓位/PnL真相源，仅基于成交回报更新。
- 边界：不从策略信号推断仓位。

10. `controlapi`
- 职责：控制面 REST（启停策略、参数更新、限额配置、只读查询）。
- 边界：不承载热路径数据分发。

## 3.2 边界强约束（必须遵守）

1. 只有 `execution` 可以调用交易所交易接口。
2. 只有 `orderfsm` 可以写订单状态。
3. 只有 `position` 可以写仓位与PnL。
4. `strategy` 只能消费标准化行情，不能消费交易所原始消息。
5. 任一 NATS/MySQL 异常不得阻塞内存热路径，系统必须进入可控降级。

## 4. 数据流与调用链

1. `adapter -> normalizer -> book -> NATS(market.normalized.*)`
2. `strategy-runner(strategy) <- NATS(market.normalized.*) -> NATS(strategy.intent.*)`
3. `engine-core(risk -> execution -> orderfsm -> position) <- NATS(strategy.intent.*)`
4. `adapter回报 -> orderfsm -> position`
5. 关键事件异步写入 NATS（审计/回放）。
6. 订单、成交、仓位、配置版本写入 MySQL。

## 5. 中间件设计（MySQL + NATS）

## 5.1 MySQL 用途

作为持久化真相源，保存：

1. `orders`：订单主表（状态以 `orderfsm` 为准）。
2. `fills`：成交明细。
3. `positions`：账户仓位与成本。
4. `risk_events`：风控拒单与关键判定。
5. `strategy_config_versions`：策略参数版本管理。
6. `venue_offsets`：交易所时间偏移、序列游标等恢复信息。

约束：

1. 线上链路不依赖 MySQL 同步查询做高频决策。
2. 写入采用批量/异步落盘策略，避免阻塞热路径。

## 5.2 NATS 用途

作为事件总线与审计回放通道，建议 subjects：

1. `market.normalized.spot.*`
2. `strategy.intent.*`
3. `risk.decision.*`
4. `order.lifecycle.*`
5. `trade.fill.*`
6. `audit.ops.*`

约束：

1. NATS 发布或订阅异常不阻塞交易主链路。
2. `order.lifecycle`、`trade.fill` 必须保证可追溯和重放。

## 6. API 设计边界

REST（控制面）示例：

1. `POST /api/v1/strategies/{id}/start`
2. `POST /api/v1/strategies/{id}/stop`
3. `PUT /api/v1/strategies/{id}/config`
4. `GET /api/v1/orders/{orderId}`
5. `GET /api/v1/positions`
6. `GET /api/v1/health`

原则：

1. REST 只做控制面和只读查询。
2. 实时行情与交易热路径不通过 REST 传输。

## 7. 可观测性与日志

## 7.1 指标（Prometheus）

1. 行情：`tick_rate`、`book_stale_ms`、`ws_reconnect_count`
2. 延迟：`market_pipeline_latency_ms`、`strategy_latency_ms`、`execution_latency_ms`
3. 交易：`order_reject_rate`、`order_timeout_count`、`fill_delay_ms`
4. 系统：CPU、内存、GC、goroutine、队列深度

## 7.2 日志（SLS）

日志链路：`stdout(JSON) -> Fluent Bit -> SLS`

日志分层：

1. `trading_audit`：订单、风控、成交、关键配置变更（长保留）
2. `runtime_ops`：运行日志、异常、重连、健康探针（短保留）

统一字段：
`ts trace_id strategy_id venue symbol order_id latency_ms err_code rule_id`

## 8. Kubernetes 部署基线

## 8.1 工作负载

1. `engine-core`：Deployment，2副本，支持滚动发布与回滚。
2. `strategy-runner`：Deployment，独立发布与回滚。
3. `nats`：StatefulSet（建议3节点，开启 JetStream）。
4. `mysql`：优先托管；自建需主从、备份、恢复演练。
5. `prometheus`、`grafana`、`fluent-bit`：独立部署。

## 8.2 关键配置

1. `readinessProbe` + `livenessProbe`
2. `PodDisruptionBudget`
3. `topologySpreadConstraints`
4. 核心服务 `resources.requests == resources.limits`（Guaranteed QoS）
5. 热路径服务优先调度到专用节点池

## 9. 代码质量与测试门禁

## 9.1 工程规范

1. 每个模块必须有包级说明（职责、边界、不变量）。
2. 注释只写约束与失败语义，避免冗余描述。
3. 接口与事件结构统一放在 `pkg/contracts`。

## 9.2 CI 必过项

1. `go fmt` / `go vet` / `golangci-lint`
2. `go test ./... -race`
3. 集成测试（仿真交易所 + 全链路）
4. 回放一致性测试（同输入同输出）
5. 性能基准测试（P99 超阈值阻断发布）

## 9.3 测试分层

1. 单元测试：normalizer、risk规则、orderfsm转移。
2. 集成测试：行情到下单闭环与异常路径。
3. 回放测试：历史行情重放下的策略确定性。
4. 压测：高并发行情输入下的延迟与丢包率。

## 10. 版本演进路线

1. `V1`：Binance/OKX 现货 + 单账户 + 单策略集。
2. `V1.1`：多策略并发、风控规则插件化。
3. `V1.2`：新增第三交易所 adapter。
4. `V2`：合约扩展（杠杆、保证金、标记价格、资金费率）。
