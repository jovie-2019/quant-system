# Workflow Ticket Board

- Date: `2026-03-31`
- Current Phase: `phase-14 in progress (go-live rehearsal evidence closure + MCP governance)`
- Overall Status: `at_risk`

## Active Tickets

1. `TICKET-0001` `accepted`
- title: Freeze V1 Spec and Contracts for first coding wave
- owner: `spec-agent`

2. `TICKET-0002` `accepted`
- title: Bootstrap Go toolchain for quality gates
- owner: `orchestrator-agent`
- blocker: `none`

3. `TICKET-0003` `accepted`
- title: Implement adapter+normalizer skeleton and baseline tests
- owner: `impl-agent`
- depends_on: `TICKET-0001`, `TICKET-0002`

4. `TICKET-0004` `accepted`
- title: Implement hub+strategy skeleton and baseline tests
- owner: `impl-agent`
- depends_on: `TICKET-0001`, `TICKET-0002`, `TICKET-0003`

5. `TICKET-0005` `accepted`
- title: Implement risk+execution skeleton and guardrail tests
- owner: `impl-agent`
- depends_on: `TICKET-0001`, `TICKET-0002`, `TICKET-0003`, `TICKET-0004`
- blocker: `none`

6. `TICKET-0006` `accepted`
- title: Implement orderfsm+position skeleton and consistency tests
- owner: `impl-agent`
- depends_on: `TICKET-0001`, `TICKET-0002`, `TICKET-0003`, `TICKET-0004`, `TICKET-0005`
- blocker: `none`

7. `TICKET-0007` `accepted`
- title: Implement controlapi skeleton with strategy/risk config endpoints
- owner: `impl-agent`
- depends_on: `TICKET-0001`, `TICKET-0002`, `TICKET-0003`, `TICKET-0004`, `TICKET-0005`, `TICKET-0006`
- blocker: `none`

8. `TICKET-0008` `accepted`
- title: Implement minimal integration tests for end-to-end event flow
- owner: `test-agent`
- depends_on: `TICKET-0007`
- blocker: `none`

9. `TICKET-0009` `accepted`
- title: Add replay/perf baseline scaffolding and acceptance report
- owner: `perf-agent`
- depends_on: `TICKET-0008`
- blocker: `none`

10. `TICKET-0010` `accepted`
- title: Prepare delivery readiness pack (k8s baseline, runbook, release checklist)
- owner: `release-agent`
- depends_on: `TICKET-0001`, `TICKET-0002`, `TICKET-0003`, `TICKET-0004`, `TICKET-0005`, `TICKET-0006`, `TICKET-0007`, `TICKET-0008`, `TICKET-0009`
- blocker: `none`

11. `TICKET-0011` `accepted`
- title: Bootstrap local NATS and MySQL without proxy
- owner: `release-agent`
- depends_on: `TICKET-0010`
- blocker: `none`

12. `TICKET-0012` `accepted`
- title: Close phase-9 audit pack and prepare delivery-entry decision
- owner: `audit-agent`
- depends_on: `TICKET-0010`, `TICKET-0011`
- blocker: `none`

13. `TICKET-0013` `accepted`
- title: Execute phase-10 delivery checks in local environment
- owner: `release-agent`
- depends_on: `TICKET-0012`
- blocker: `none`

14. `TICKET-0014` `accepted`
- title: Freeze V1 local delivery baseline and archive evidence
- owner: `supervisor-report-agent`
- depends_on: `TICKET-0013`
- blocker: `none`

15. `TICKET-0201` `accepted`
- title: Centralize core event/data contracts into pkg/contracts
- owner: `impl-agent`
- depends_on: `TICKET-0014`
- blocker: `none`

16. `TICKET-0202` `accepted`
- title: Implement production-grade Binance/OKX adapter (WS market + REST trade)
- owner: `impl-agent`
- depends_on: `TICKET-0201`
- blocker: `none`

17. `TICKET-0203` `accepted`
- title: Add MySQL persistence for order/position/risk audit records
- owner: `impl-agent`
- depends_on: `TICKET-0201`
- blocker: `none`

18. `TICKET-0204` `accepted`
- title: Integrate NATS bus for async event fan-out and replay path
- owner: `impl-agent`
- depends_on: `TICKET-0201`
- blocker: `none`

19. `TICKET-0205` `accepted`
- title: Implement observability baseline (metrics, logs, dashboards)
- owner: `release-agent`
- depends_on: `TICKET-0202`, `TICKET-0203`, `TICKET-0204`
- blocker: `none`

20. `TICKET-0206` `accepted`
- title: Implement local order book module and strategy-facing snapshot API
- owner: `impl-agent`
- depends_on: `TICKET-0202`
- blocker: `none`

21. `TICKET-0301` `in_progress`
- title: Build sandbox end-to-end integration harness (market -> strategy -> risk -> execution -> fill -> state consistency)
- owner: `impl-agent`
- depends_on: `TICKET-0202`, `TICKET-0203`, `TICKET-0204`, `TICKET-0206`
- blocker: `none`

22. `TICKET-0401` `accepted`
- title: Add container build and k8s dev deployment scripts for engine-core
- owner: `release-agent`
- depends_on: `TICKET-0301`
- blocker: `none`

23. `TICKET-0402` `accepted`
- title: Bootstrap kube-prometheus-stack and engine-core metrics scraping in k8s
- owner: `release-agent`
- depends_on: `TICKET-0401`
- blocker: `none`

24. `TICKET-0403` `accepted`
- title: Add Grafana k8s/service status dashboard and Prometheus alert rules
- owner: `release-agent`
- depends_on: `TICKET-0402`
- blocker: `none`

25. `TICKET-0404` `in_progress`
- title: Add Fluent Bit to SLS logging baseline for k8s
- owner: `release-agent`
- depends_on: `TICKET-0402`
- blocker: `waiting real SLS endpoint/auth for final e2e`

26. `TICKET-0405` `in_progress`
- title: Run dev go-live rehearsal and produce release decision evidence pack
- owner: `supervisor-report-agent`
- depends_on: `TICKET-0401`, `TICKET-0402`, `TICKET-0403`, `TICKET-0404`
- blocker: `collecting rehearsal evidence pack and rollback proof`

27. `TICKET-0907` `in_progress`
- title: Strategy Runtime Decoupling (engine-core -> strategy-runner)
- owner: `impl-agent`
- depends_on: `TICKET-0401`, `TICKET-0402`
- blocker: `none`

28. `TICKET-0908` `in_progress`
- title: Grafana Data Validity and Coverage Gate
- owner: `release-agent`
- depends_on: `TICKET-0403`, `TICKET-0907`
- blocker: `none`

29. `TICKET-0410` `in_progress`
- title: Establish MCP observability governance and automated release gate
- owner: `mcp-gate-agent`
- depends_on: `TICKET-0405`
- blocker: `collecting first live MCP evidence pack`

30. `TICKET-0910` `in_progress`
- title: Trade Gateway Hardening and Recoverability
- owner: `impl-agent`
- depends_on: `TICKET-0202`, `TICKET-0301`
- blocker: `none`

31. `TICKET-0911` `in_progress`
- title: Market Ingest Production Hardening
- owner: `impl-agent`
- depends_on: `TICKET-0401`, `TICKET-0402`, `TICKET-0908`, `TICKET-0410`
- blocker: `none`

## Gate Snapshot

1. HITL required now: `no`
2. reason: `phase-14 evidence closure and MCP governance in progress; HITL decision after TICKET-0405 and MCP pass evidence are complete`

## Deferred Backlog (Post-Go-Live)

1. backlog file: `automation/workflow/tickets/post-go-live-backlog.md`
2. deferred tickets: `TICKET-0901` ~ `TICKET-0906`
3. activate_when: `observability + k8s上线主路径完成后`
