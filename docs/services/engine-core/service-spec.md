# engine-core 服务规范（V1）

- Service Name: `engine-core`
- Language: `Go`
- Type: 模块化单体（单进程多模块，可后续按边界拆分）

## 1. 服务目标

1. 完成现货交易内核闭环：`OrderIntent -> 风控 -> 下单 -> 回报 -> 仓位`。
2. 提供稳定低延迟热路径：尽量使用进程内调用，避免跨服务开销。
3. 保证交易一致性：订单状态与仓位只有唯一写入口。

## 2. 模块拓扑

1. 输入链路：`NATS(strategy.intent.*) -> risk`
2. 交易链路：`risk -> execution -> adapter`
3. 回报链路：`adapter(execReport) -> orderfsm -> position`
4. 控制链路：`controlapi -> risk/config`
5. 异步事件：关键事件写入 NATS（JetStream）；持久状态写入 MySQL

## 3. 模块边界总约束

1. 只有 `execution` 可以调用交易所交易 API。
2. 只有 `orderfsm` 可以修改订单状态。
3. 只有 `position` 可以修改仓位和PnL。
4. `controlapi` 不承载热路径数据分发。

## 4. 进程内并发模型

1. 每个 `venue-symbol` 使用固定分片 worker，保证同一分片顺序处理。
2. 模块间通过无锁队列或 channel 传递轻量结构体指针，减少拷贝。
3. 慢路径（落库、发 NATS）异步化，禁止阻塞热路径 worker。

## 5. 依赖清单

1. 外部依赖：`MySQL`、`NATS`、交易所公网 API。
2. 可观测：`Prometheus` 指标、结构化日志到 `SLS`。
3. 配置：本地文件 + 环境变量，启动时加载，运行时经 `controlapi` 热更新。

## 6. 发布与恢复原则

1. 灰度发布：先非关键策略，再关键策略。
2. 异常降级：交易所异常时支持“只平不开”模式。
3. 恢复顺序：行情恢复 -> 状态校验 -> 策略放行 -> 下单恢复。

## 7. 文档到代码映射

1. `internal/<module>` 对应模块文档 `engine-core/modules/<module>.md`。
2. 统一事件定义放 `pkg/contracts`。
3. 模块公共依赖封装在 `pkg/obs`（日志/指标/trace）与 `internal/store/mysql`、`internal/bus/nats`。
