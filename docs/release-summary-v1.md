# Release Summary V1

## 1. 交付范围（本地演练）

1. 核心模块骨架与测试基线完成：
- adapter/normalizer/hub/strategy/risk/execution/orderfsm/position/controlapi
2. 自动化流程与审计材料完成：
- workflow tickets/board/reports/review-pack
3. 部署与运维基线完成：
- k8s base/overlays、runbook、release checklist、delivery readiness
4. 本地运行能力完成：
- `nats-server`、`mysql`、`engine-core` 可本地验证

## 2. 关键证据

1. 质量门禁：
- `RUN_INTEGRATION_TESTS=1 RUN_REPLAY_TESTS=1 RUN_PERF_TESTS=1 automation/scripts/run_quality_gates.sh`
2. 性能基线：
- `go test ./test/perf -count=1 -v`
3. 运行态检查：
- `nc -vz 127.0.0.1 4222`
- `mysqladmin ping -uroot`
- `curl -sS http://127.0.0.1:8080/api/v1/health`

## 3. 已知差距（进入下个里程碑前）

1. 仍为内存态执行路径，需逐步替换为 NATS/MySQL 持久化链路。
2. 缺少交易所 sandbox 端到端联调证据。
3. Homebrew 默认 NATS 未启用 `8222`，若需 monitor 必须切换到 monitor 模式启动参数。

## 4. 建议下一里程碑

1. 实现真实交易所适配器接入与回放数据一致性校验。
2. 完成 Prometheus 指标闭环并补齐 P50/P95/P99 延迟看板。
3. 在 k8s dev 集群执行一次完整发布与回滚演练。
