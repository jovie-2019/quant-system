# TICKET-0401

## 1. 基本信息

- `ticket_id`: TICKET-0401
- `title`: Add container build and k8s dev deployment scripts for engine-core
- `owner_agent`: release-agent
- `related_module`: Dockerfile, automation/scripts, deploy/k8s
- `priority`: P0
- `status`: accepted

## 2. 变更范围（必须精确）

- `allowed_write_paths`: `Dockerfile`, `.dockerignore`, `automation/scripts/*`, `docs/runbook-v1.md`
- `forbidden_paths`: trading decision logic
- `out_of_scope`: CI/CD platform integration

## 3. 输入与依赖

- `input_docs`: runbook-v1.md, release-checklist-v1.md
- `upstream_tickets`: `TICKET-0301`
- `external_constraints`: single-operator execution on k8s dev

## 4. 验收条件（Definition of Done）

1. 提供可执行的镜像构建脚本。
2. 提供 k8s dev 一键部署脚本（含 rollout 检查）。
3. 提供 k8s dev 冒烟检查脚本。

## 5. 风险与回滚

- `risk_level`: medium
- `rollback_plan`: rollback deployment to previous image tag
- `hitl_required`: no

## 6. 输出产物

1. container build artifact files
2. k8s deploy/smoke scripts
3. runbook command update

## 7. 执行记录

1. 已新增 `Dockerfile` + `.dockerignore`。
2. 已新增 `automation/scripts/build_engine_core_image.sh`。
3. 已新增 `automation/scripts/k8s_deploy_dev.sh` + `automation/scripts/k8s_smoke_status.sh`。
4. 已在 `docs/runbook-v1.md` 补齐执行命令。
5. live 集群验证通过：`engine-core/mysql/nats` 全部 Running，`k8s_smoke_status` 可复现。
