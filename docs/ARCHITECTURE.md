# Reconcilyx v4.0.0-beta Architecture

## Status First

This document describes the intended architecture and the current implementation posture.

- Current milestone: `v4.0.0-beta`
- Current audit baseline: `0 Verified / 72 Partial / 30 Missing-Stubbed`
- Current reality: strong prototype, not production-ready

## Current Shape Of The System

The codebase currently consists of four main layers:

1. API types and controllers for `OptimizationPolicy`, `OptimizationProposal`, and cluster registration
2. An analyzer layer for telemetry inspection, rules, anomaly checks, forecasting, and recommendation synthesis
3. Supporting subsystems for federated learning, GitOps, chat, notifications, and UI
4. Experimental or incomplete pillars that still need wiring, live integrations, or both

## Pillar Posture

| Pillar | Current Status | Reality Check |
|---|---|---|
| Core actuation | `⚙️ Partial` | Proposal lifecycle and rollback scaffolding exist, but live `/resize` + SSA parity is still missing |
| Federated learning | `⚙️ Partial` | Aggregation and bootstrap exist, but live RL update wiring is incomplete |
| Multi-cloud arbitrage | `⚙️ Partial` | Arbitrage logic exists, but pricing providers are still static |
| Carbon and app intelligence | `⚙️ Partial` | JVM helpers exist; GPU data path is still stubbed |
| ChatOps | `⚙️ Partial` | Routing and provider selection exist, but answers are still heavily canned |
| Shift-left GitOps | `⚙️ Partial` | PR helpers exist, but runtime wiring and validation are incomplete |
| Agentic execution | `⚙️ Partial` | Action surfaces exist, but confirmation and real execution are incomplete |
| Node consolidation | `❌ Stub/Planned` | Proposal helper exists, but no real Karpenter actuation loop |
| eBPF streaming | `❌ Stub/Planned` | Current stream is synthetic and should not be presented as production telemetry |
| Model registry | `⚙️ Partial` | Snapshot versioning and canary helpers exist, but rollout wiring is incomplete |

## Beta Architecture Priorities

The beta architecture is focused on five hardening goals:

1. Buildability and testability
2. Honest API/UI contracts
3. Correct CLI/API mutation behavior
4. Safe, explicit workload mutation semantics
5. Clear separation between implemented features and planned pillars

## Non-Verified Areas

The following areas should be treated as in development until upgraded by evidence:

- Federated learning end-to-end updates
- Live cloud pricing
- Real GPU telemetry
- Agentic write execution
- eBPF telemetry
- Karpenter node consolidation

## References

- [STATUS.md](../STATUS.md)
- [FEATURE_MATRIX.md](../FEATURE_MATRIX.md)
- [GITHUB_ISSUES.md](../GITHUB_ISSUES.md)
