# MCP Observability Gate Summary

- generated_at: 2026-03-30 22:39:47 +0800
- status: fail
- failures: 5
- evidence_dir: /Users/lama/quant-system/automation/reports/mcp-gate-selfcheck
- result_json: /Users/lama/quant-system/automation/reports/mcp-gate-selfcheck/result.json
- grafana_log: /Users/lama/quant-system/automation/reports/mcp-gate-selfcheck/verify-grafana-data.log

## Metrics Snapshot

1. engine-core-up: 0 (min 1)
2. strategy-runner-up: 0 (min 1)
3. nats-up: 0 (min 1)
4. mysql-up: 0 (min 1)
5. critical_firing: 0 (max 0)
6. warning_firing: 0 (max 5)

## Failures

- verify_grafana_data.sh failed (see /Users/lama/quant-system/automation/reports/mcp-gate-selfcheck/verify-grafana-data.log)
- engine-core-up=0 < 1
- strategy-runner-up=0 < 1
- nats-up=0 < 1
- mysql-up=0 < 1

