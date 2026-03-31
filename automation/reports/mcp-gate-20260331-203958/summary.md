# MCP Observability Gate Summary

- generated_at: 2026-03-31 20:39:58 +0800
- status: fail
- failures: 4
- evidence_dir: /Users/lama/quant-system/automation/reports/mcp-gate-20260331-203958
- result_json: /Users/lama/quant-system/automation/reports/mcp-gate-20260331-203958/result.json
- grafana_log: /Users/lama/quant-system/automation/reports/mcp-gate-20260331-203958/verify-grafana-data.log

## Metrics Snapshot

1. engine-core-up: 1 (min 1)
2. strategy-runner-up: 0 (min 1)
3. market-ingest-up: 0 (min 1)
4. nats-up: 1 (min 1)
5. mysql-up: 1 (min 1)
6. critical_firing: 1 (max 0)
7. warning_firing: 8 (max 5)

## Failures

- strategy-runner-up=0 < 1
- market-ingest-up=0 < 1
- critical_firing=1 > 0
- warning_firing=8 > 5

