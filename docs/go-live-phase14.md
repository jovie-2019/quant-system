# Phase-14 执行手册：可观测到上线（Dev）

## 1. 目标

在 k8s dev 环境完成：

1. 业务服务可运行（engine-core + strategy-runner + market-ingest + nats + mysql）。
2. 可观测可视化可用（Prometheus + Grafana + Alertmanager）。
3. 服务状态可判读（k8s 工作负载状态 + 交易核心指标）。
4. 日志可检索（Fluent Bit -> SLS）。
5. MCP 自动门禁通过并形成可审计证据。

## 2. 推荐执行入口（一键演练 + 证据归档）

1. 默认执行全流程（含 MCP，不含回滚）：
- `automation/scripts/phase14_rehearsal.sh`

2. 含回滚演练：
- `RUN_ROLLBACK=1 automation/scripts/phase14_rehearsal.sh`

3. 仅演练不实际执行（检查命令顺序）：
- `DRY_RUN=1 automation/scripts/phase14_rehearsal.sh`

4. 脚本输出：
- 自动生成 `automation/reports/phase14-rehearsal-<timestamp>/`
- 包含 `summary.md`、每一步 `.log`、以及 `mcp-gate/` 子目录证据

## 3. 手动执行顺序（与脚本一致）

1. 构建镜像：
- `IMAGE_REPO=quant-system/engine-core IMAGE_TAG=dev automation/scripts/build_engine_core_image.sh`

2. 构建镜像：
- `IMAGE_REPO=quant-system/market-ingest IMAGE_TAG=dev automation/scripts/build_market_ingest_image.sh`

3. 部署业务：
- `automation/scripts/k8s_deploy_dev.sh`

4. 部署观测：
- `automation/scripts/k8s_bootstrap_observability.sh`

5. 部署日志：
- `automation/scripts/k8s_bootstrap_logging.sh`

6. 冒烟检查：
- `automation/scripts/k8s_smoke_status.sh`

7. MCP 自动门禁：
- `automation/scripts/mcp_observability_gate.sh`

8. （可选）legacy grafana 门禁：
- `automation/scripts/verify_grafana_data.sh`

## 4. 可视化验收标准

1. Grafana 可访问且可查询 Prometheus 数据。
2. `engine-core-v1` 看板有指标数据（controlapi、risk、execution、ttlcache、momentum）。
3. `engine-k8s-status-v1` 看板可看到：
- engine-core ready replicas
- market-ingest up
- nats/mysql ready replicas
- 重启次数与错误率
4. 关键查询“非空返回”通过：
- `engine-core-up`
- `strategy-runner-up`
- `market-ingest-up`
- `nats-up`
- `mysql-up`
- `controlapi-p99`
- `controlapi-status-rate`
- `execution-gateway-event-rate`
- `ttlcache-get-rate`
- `ttlcache-size`
- `momentum-eval-rate`
- `momentum-eval-p95`
- `momentum-signal-rate`
- `market-ingest-event-rate`

## 5. 上线前必须通过

1. `go test ./... -count=1` 通过。
2. k8s rollout 全部完成。
3. 告警规则已加载且无持续 firing。
4. sandbox E2E（phase-13）证据齐全。
5. MCP 门禁 `pass` 且证据目录完整。

## 6. 失败回退

1. 回滚业务：
- `kubectl rollout undo deployment/engine-core -n quant-system`

2. 卸载观测组件：
- `helm uninstall kube-prometheus-stack -n observability`

3. 卸载日志组件：
- `helm uninstall fluent-bit -n observability`
