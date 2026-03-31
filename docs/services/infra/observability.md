# 基础设施规范：Observability

- Metrics: `Prometheus`
- Dashboard: `Grafana`
- Logs: `Fluent Bit -> SLS`

## 1. 目标

1. 快速发现延迟回退、交易异常、连接抖动。
2. 支持按 `trace_id` 追踪“信号 -> 风控 -> 下单 -> 回报”。
3. 提供可执行告警，不做噪音告警。
4. 观测验收自动化，降低对单一人工目检的依赖。

## 2. 指标规范

关键指标：

1. 行情：`tick_rate`、`book_stale_ms`、`ws_reconnect_count`
2. 策略：`strategy_eval_latency_ms`、`strategy_intent_count`
3. 风控：`risk_reject_count`、`risk_eval_latency_ms`
4. 执行：`execution_submit_latency_ms`、`execution_error_rate`、`execution_gateway_events_total`
5. 状态机：`orderfsm_illegal_transition_count`
6. 仓位：`position_apply_latency_ms`

## 3. 告警规则（V1）

1. `market_pipeline_latency_ms_p99 > 5ms` 持续 3 分钟
2. `strategy_to_execution_latency_p99 > 15ms` 持续 3 分钟
3. `ws_reconnect_count` 异常突增
4. `orderfsm_illegal_transition_count > 0`
5. `execution_error_rate` 超阈值
6. `execution_gateway_retry_exhausted_rate > 0`

## 4. 日志规范（SLS）

日志分类：

1. `trading_audit`：订单、风控、成交、配置变更
2. `runtime_ops`：系统运行、重连、异常

统一字段：

1. `ts`
2. `level`
3. `trace_id`
4. `strategy_id`
5. `venue`
6. `symbol`
7. `order_id`
8. `latency_ms`
9. `err_code`
10. `message`

## 5. 排障流程建议

1. 先看 Grafana 指标确认异常模块与时间窗口。
2. 再到 SLS 用 `trace_id/order_id` 查询完整链路日志。
3. 最后到 NATS/MySQL 对照事件与状态落地情况。

## 6. 当前实现进度（2026-03-31）

1. 已落地：`engine-core` 暴露 `/metrics`（Prometheus text format）。
2. 已落地指标：
- `engine_controlapi_http_requests_total`
- `engine_controlapi_http_request_duration_ms_bucket`
- `engine_controlapi_http_request_duration_ms_sum`
- `engine_controlapi_http_request_duration_ms_count`
- `engine_risk_decision_total`
- `engine_risk_eval_duration_ms_bucket`
- `engine_execution_submit_total`
- `engine_execution_submit_duration_ms_bucket`
- `engine_execution_gateway_events_total`
- `engine_ttlcache_get_total`
- `engine_ttlcache_eviction_total`
- `engine_ttlcache_purge_total`
- `engine_ttlcache_size`
- `engine_strategy_momentum_eval_total`
- `engine_strategy_momentum_eval_duration_ms_bucket`
- `engine_strategy_momentum_signal_total`
- `engine_market_ingest_events_total`
3. 已落地 Grafana Dashboard：
- `deploy/observability/grafana/engine-core-v1.json`（含 controlapi、ttlcache、momentum、market-ingest、execution-gateway 面板）
- `deploy/observability/grafana/engine-k8s-status-v1.json`
4. K8s scrape 注解与 ServiceMonitor 已覆盖：
- `engine-core`
- `strategy-runner`
- `market-ingest`

## 7. k8s 观测栈部署（Phase-14）

新增交付物：

1. kube-prometheus-stack values：
- `deploy/observability/kube-prometheus-stack-values.yaml`

2. 指标采集：
- `deploy/observability/engine-core-servicemonitor.yaml`
- `deploy/observability/strategy-runner-servicemonitor.yaml`
- `deploy/observability/market-ingest-servicemonitor.yaml`

3. 告警规则：
- `deploy/observability/prometheus-rules/engine-core-rules.yaml`

4. dashboard：
- `deploy/observability/grafana/engine-core-v1.json`
- `deploy/observability/grafana/engine-k8s-status-v1.json`

5. 部署脚本：
- `automation/scripts/k8s_bootstrap_observability.sh`
- `automation/scripts/k8s_bootstrap_logging.sh`

6. 验收脚本：
- `automation/scripts/verify_grafana_data.sh`
- `automation/scripts/mcp_observability_gate.sh`

执行顺序：

1. 先部署业务服务：
- `automation/scripts/k8s_deploy_dev.sh`
2. 再部署观测栈：
- `automation/scripts/k8s_bootstrap_observability.sh`
3. 再部署日志：
- `automation/scripts/k8s_bootstrap_logging.sh`
4. 再做状态冒烟：
- `automation/scripts/k8s_smoke_status.sh`
5. 最后执行 MCP 门禁：
- `automation/scripts/mcp_observability_gate.sh`

## 8. 最小日志采集方案（SLS）

1. `stdout/stderr` 统一 JSON 行日志输出（建议字段：`ts`,`level`,`trace_id`,`module`,`message`）。
2. Fluent Bit 采集容器日志并按 `trading_audit/runtime_ops` 分流到 SLS Logstore。
3. 已提供 Fluent Bit 基线 values：
- `deploy/observability/fluent-bit/values.yaml`
4. 部署脚本：
- `automation/scripts/k8s_bootstrap_logging.sh`
5. 关键检索键：
- `trace_id`（全链路）
- `client_order_id` / `trade_id`（交易审计）

## 9. MCP 自动验收规范（新增）

1. 章程：`automation/workflow/mcp-charter-v1.md`。
2. 默认门禁脚本：`automation/scripts/mcp_observability_gate.sh`。
3. 证据目录：`automation/reports/mcp-gate-<timestamp>/`。
4. 最小输出：
- `summary.md`
- `result.json`
- `verify-grafana-data.log`
- `env.txt`
5. 流程约束：
- MCP 未通过，不允许进入发布 GO 决策。
- 如需豁免，必须进入 HITL Gate 并保留豁免记录。
