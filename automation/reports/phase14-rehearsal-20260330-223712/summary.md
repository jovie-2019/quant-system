# Phase-14 Rehearsal Summary

- generated_at: 2026-03-30 22:37:12 +0800
- evidence_dir: /Users/lama/quant-system/automation/reports/phase14-rehearsal-20260330-223712
- kube_context: kind-quant-dev
- image: quant-system/engine-core:dev
- run_tests: 1
- run_build: 1
- run_deploy: 1
- run_obs: 1
- run_logging: 1
- run_smoke: 1
- run_mcp_gate: 1
- run_grafana_gate: 0
- run_sandbox_trade_smoke: 0
- run_rollback: 0
- dry_run: 1
[phase14] step=00-go-test
[phase14] DRY_RUN=1 skip execution: go test ./... -count=1
[phase14] step=01-build-image
[phase14] DRY_RUN=1 skip execution: env IMAGE_REPO=quant-system/engine-core IMAGE_TAG=dev PUSH_IMAGE=0 automation/scripts/build_engine_core_image.sh
[phase14] step=02-k8s-deploy
[phase14] DRY_RUN=1 skip execution: automation/scripts/k8s_deploy_dev.sh
[phase14] step=03-k8s-observability
[phase14] DRY_RUN=1 skip execution: automation/scripts/k8s_bootstrap_observability.sh
[phase14] step=04-k8s-logging
[phase14] DRY_RUN=1 skip execution: automation/scripts/k8s_bootstrap_logging.sh
[phase14] step=05-k8s-smoke
[phase14] DRY_RUN=1 skip execution: automation/scripts/k8s_smoke_status.sh
[phase14] step=06-mcp-gate
[phase14] DRY_RUN=1 skip execution: env EVIDENCE_DIR=/Users/lama/quant-system/automation/reports/phase14-rehearsal-20260330-223712/mcp-gate automation/scripts/mcp_observability_gate.sh
[phase14] rehearsal completed, evidence_dir=/Users/lama/quant-system/automation/reports/phase14-rehearsal-20260330-223712
