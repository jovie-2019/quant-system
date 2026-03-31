# Spec Freeze V1

- Version: `v1.0-freeze`
- Date: `2026-03-24`
- Scope: `spot only`, `Go runtime`, `MySQL + NATS`, `Kubernetes`

## 1. Frozen Boundaries

1. `execution` is the only module allowed to call venue trading APIs.
2. `orderfsm` is the only module allowed to mutate order states.
3. `position` is the only module allowed to mutate positions and pnl.
4. `strategy` can only consume normalized market events.
5. control-plane is REST only; hot path is in-process only.

## 2. Frozen Event Contract Rules

1. All business events must use the Envelope in `event-contracts-v1.md`.
2. NATS subject routing follows `event-contracts-v1.md` section 4.
3. Contract changes must be additive by default.
4. Breaking changes require HITL decision and version bump.

## 3. Frozen Acceptance Gates

1. Quality gates in `quality-gates.md` remain mandatory.
2. Testing matrix in `testing-matrix.md` remains the baseline.
3. Any changes to risk/execution/orderfsm/position trigger HITL.

## 4. Allowed Change Window (Pre-Implementation)

1. Wording and clarity improvements in docs are allowed.
2. No boundary changes without a decision request.
3. No contract breaking changes without a decision request.
