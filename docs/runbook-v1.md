# Runbook V1

## 1. 目标

提供 V1 系统的启动、健康检查、故障排查和回滚操作步骤。

## 2. 启动顺序

1. 启动 `nats`（JetStream enabled）。
2. 启动 `mysql` 并确认库 `quant` 可访问。
3. 启动 `market-ingest`。
4. 启动 `strategy-runner`。
5. 启动 `engine-core`。

## 3. 健康检查

1. `engine-core`: `GET /api/v1/health` 返回 `200`。
2. `strategy-runner`: `GET /api/v1/health` 返回 `200`。
3. `market-ingest`: `GET /api/v1/health` 返回 `200`。
4. `engine-core metrics`: `GET /metrics` 返回 Prometheus 文本格式。
4. `nats`:
- monitor 模式（启用 `-m 8222`）下：`:8222/healthz` 返回健康状态。
- 默认本地模式下：`nc -vz 127.0.0.1 4222` 连通成功。
5. `mysql`: `mysqladmin ping` 返回 `mysqld is alive`。

## 4. 关键故障排查

1. 行情不流动：
- 检查 adapter 输入日志、hub drop 计数、strategy 订阅状态。
2. 下单未触发：
- 检查 risk reject 计数、execution submit 错误、gateway 回包。
3. 状态不一致：
- 检查 orderfsm 迁移错误、position 幂等去重命中、fill 事件顺序。

## 5. 回滚步骤

1. 回滚 `engine-core` 到上一镜像 tag。
2. 保持 `nats/mysql` 不回滚（除非数据层变更）。
3. 回滚后执行健康检查与关键链路冒烟测试。

## 6. 发布后观察窗口

1. 观察 30 分钟：延迟、错误率、拒单率、状态机非法迁移计数。
2. 若出现关键告警，执行回滚并记录 incident。

## 7. Sandbox 联调命令（Phase-13）

1. 行情联调（Binance + OKX，默认符号 `BTC-USDT`）：
- `RUN_SANDBOX_TESTS=1 go test ./test/sandbox/... -count=1 -v`

2. 指定单交易所联调（示例：只测 Binance）：
- `RUN_SANDBOX_TESTS=1 SANDBOX_OKX_ENABLED=0 go test ./test/sandbox/... -count=1 -v`

3. 调整超时（示例：30 秒）：
- `RUN_SANDBOX_TESTS=1 SANDBOX_TIMEOUT=30s go test ./test/sandbox/... -count=1 -v`

4. 交易烟测（示例：Binance Testnet）：
- `RUN_SANDBOX_TESTS=1 RUN_SANDBOX_TRADE_TESTS=1 SANDBOX_TRADE_VENUE=binance SANDBOX_TRADE_SYMBOL=BTC-USDT SANDBOX_TRADE_PRICE=100 SANDBOX_TRADE_QTY=0.001 SANDBOX_BINANCE_API_KEY=xxx SANDBOX_BINANCE_API_SECRET=xxx go test ./test/sandbox/... -count=1 -v`

5. 交易烟测（示例：OKX Demo）：
- `RUN_SANDBOX_TESTS=1 RUN_SANDBOX_TRADE_TESTS=1 SANDBOX_TRADE_VENUE=okx SANDBOX_TRADE_SYMBOL=BTC-USDT SANDBOX_TRADE_PRICE=100 SANDBOX_TRADE_QTY=0.001 SANDBOX_OKX_API_KEY=xxx SANDBOX_OKX_API_SECRET=xxx SANDBOX_OKX_PASSPHRASE=xxx SANDBOX_OKX_SIMULATED=1 go test ./test/sandbox/... -count=1 -v`

## 8. k8s dev 可观测上线路径（Phase-14）

1. 推荐一键入口（含 MCP 自动门禁）：
- `automation/scripts/phase14_rehearsal.sh`

2. 构建镜像：
- `IMAGE_REPO=quant-system/engine-core IMAGE_TAG=dev automation/scripts/build_engine_core_image.sh`

3. 构建镜像：
- `IMAGE_REPO=quant-system/market-ingest IMAGE_TAG=dev automation/scripts/build_market_ingest_image.sh`

4. 部署业务基线：
- `automation/scripts/k8s_deploy_dev.sh`

5. 部署观测栈：
- `automation/scripts/k8s_bootstrap_observability.sh`

6. 部署日志采集（SLS）：
- `automation/scripts/k8s_bootstrap_logging.sh`

7. 冒烟检查：
- `automation/scripts/k8s_smoke_status.sh`

8. 访问 Grafana（本地）：
- `kubectl port-forward -n observability svc/kube-prometheus-stack-grafana 3000:80`
- `kubectl port-forward -n observability svc/kube-prometheus-stack-prometheus 19090:9090`（若本地 `9090` 已被占用）

9. MCP 自动观测门禁：
- `automation/scripts/mcp_observability_gate.sh`

10. legacy Grafana 数据有效性检查（可选）：
- `PROM_URL=http://127.0.0.1:19090 automation/scripts/verify_grafana_data.sh`（若 Prometheus 走 `19090`）
