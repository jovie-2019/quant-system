# strategy-runner 服务规范（V1）

- Service Name: `strategy-runner`
- Language: `Go`
- Type: 独立策略运行时（与交易内核解耦）

## 1. 服务目标

1. 独立消费标准化行情事件，不直接依赖交易所协议细节。
2. 输出统一 `OrderIntent` 契约到 NATS，供 `engine-core` 风控/执行消费。
3. 支持策略逻辑独立发布、回滚，不影响交易内核稳定性。

## 2. 输入输出边界

1. 输入：`market.normalized.spot.>`（NATS JetStream）。
2. 输出：`strategy.intent.{strategy_id}`（NATS JetStream）。
3. 控制面：仅暴露健康接口 `/api/v1/health`（V1）。

## 3. 责任与非责任

责任：

1. 行情事件解码与策略调用。
2. 意图事件发布与错误返回（触发消息重试）。
3. 策略运行时生命周期管理（进程级）。

非责任：

1. 不做风控决策。
2. 不做交易下单。
3. 不写订单状态与仓位。

## 4. 故障隔离约束

1. `strategy-runner` 崩溃不应影响 `engine-core` 健康与控制面。
2. 策略异常仅影响对应消息处理，不允许导致进程退出。
3. 策略消费失败允许按 durable consumer 语义重试。

## 5. k8s 部署要求（V1）

1. Deployment：`strategy-runner`，单独副本与滚动发布。
2. Service：暴露 `8081` 健康检查端口。
3. ServiceMonitor：采集 `/api/v1/health` 可用性（up 指标）。

## 6. 验收条件

1. `strategy-runner` Pod `Ready=1/1`。
2. Prometheus `up{job="strategy-runner"} >= 1`。
3. Grafana `engine-k8s-status-v1` 看板可见 `strategy-runner up` 非空。
