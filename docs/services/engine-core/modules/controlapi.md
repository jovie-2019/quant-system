# 模块规范：controlapi

- Package: `internal/controlapi`
- 目标：提供控制面 REST API，不进入热路径。

## 1. 职责

1. 策略启停、参数更新、风险参数管理。
2. 提供只读查询（订单、仓位、健康状态）。
3. 提供运行时开关（如只平仓模式、策略熔断）。

## 2. 边界

1. 不处理高频行情流。
2. 不直接触发交易所 API。
3. 不绕过 `risk` 或 `orderfsm` 修改核心状态。

## 3. API 范围（V1）

1. `POST /api/v1/strategies/{id}/start`
2. `POST /api/v1/strategies/{id}/stop`
3. `PUT /api/v1/strategies/{id}/config`
4. `PUT /api/v1/risk/config`
5. `GET /api/v1/orders/{orderId}`
6. `GET /api/v1/positions`
7. `GET /api/v1/health`

## 4. 不变量

1. 所有变更接口必须记录审计日志。
2. 配置变更必须带版本号与操作者信息。
3. 幂等接口重复调用结果一致。

## 5. 失败与降级

1. 读接口失败不影响交易主链路。
2. 写配置失败不应产生半生效状态。

## 6. 指标与日志

1. 指标：`api_req_count`、`api_req_latency_ms`、`api_error_rate`
2. 日志：调用方、参数摘要、结果、配置版本变更

## 7. 测试重点

1. 接口鉴权与参数校验。
2. 配置热更新一致性。
3. 幂等语义验证。
