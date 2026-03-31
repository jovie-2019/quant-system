# 服务文档总览（Documentation-First）

本目录用于“先文档、后实现”。后续编码必须以这里的职责边界与契约为准。

## 文档结构

1. `engine-core/service-spec.md`
- 实盘核心服务总规范（模块拓扑、调用链、统一约束）。

2. `strategy-runner/service-spec.md`
- 策略运行时规范（独立发布、契约输入输出、故障隔离边界）。

3. `engine-core/modules/*.md`
- `engine-core` 内各子模块详细规范（职责、边界、接口、测试）。

4. `infra/mysql.md`
- MySQL 在系统中的定位、表级边界、容量与可用性要求。

5. `infra/nats.md`
- NATS subject 规范、投递语义、JetStream 回放策略。

6. `infra/observability.md`
- Prometheus/Grafana/SLS 的指标、日志、告警规范。

7. `contracts/event-contracts-v1.md`
- 统一事件契约（Envelope + 业务事件结构）。

8. `quality/quality-gates.md`
- 代码质量门禁与提交标准。

9. `quality/testing-matrix.md`
- 测试矩阵与通过条件。

10. `quality/sandbox-e2e-plan.md`
- sandbox 端到端联调计划与执行开关。

## 使用规则

1. 新增或修改模块前，先更新对应文档，再写代码。
2. 任一模块跨边界依赖，必须先在文档中声明并评审。
3. 代码实现若与文档冲突，以文档为准并先修正文档。
