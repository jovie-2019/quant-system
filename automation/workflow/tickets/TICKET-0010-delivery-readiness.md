# TICKET-0010

## 1. 基本信息

- `ticket_id`: TICKET-0010
- `title`: Prepare delivery readiness pack (k8s baseline, runbook, release checklist)
- `owner_agent`: release-agent
- `related_module`: deploy/k8s, docs
- `priority`: P0
- `status`: accepted

## 2. 变更范围（必须精确）

- `allowed_write_paths`: `deploy/k8s/*`, `docs/*`, `automation/reports/*`
- `forbidden_paths`: core trading module logic
- `out_of_scope`: production rollout execution

## 3. 输入与依赖

- `input_docs`: architecture-v1.md, services docs, quality/testing docs
- `upstream_tickets`: `TICKET-0001` ~ `TICKET-0009`
- `external_constraints`: requires Gate 4 approval

## 4. 验收条件（Definition of Done）

1. 产出部署基线清单（Deployment/PDB/Probe/Resources）
2. 产出 runbook（发布、回滚、故障排查）
3. 产出 release checklist（验收项、回归项、观测项）

## 5. 风险与回滚

- `risk_level`: high
- `rollback_plan`: revert deploy/docs changes
- `hitl_required`: yes (`Gate 4`)

## 6. 输出产物

1. delivery readiness pack
2. release checklist

## 7. 执行记录

1. Added Kubernetes base manifests for `engine-core`, `nats`, `mysql` with probes, resources, and PDB.
2. Added dev/prod overlay kustomization files.
3. Added `runbook-v1.md`, `release-checklist-v1.md`, and `delivery-readiness-v1.md`.
4. Passed quality gates after delivery-pack updates.
