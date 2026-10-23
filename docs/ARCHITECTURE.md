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
| Core actuation | `⚙️ Partial` | Proposal lifecycle, rollback, and `/resize` SSA are implemented; independent end-to-end cluster validation is still pending |
| Federated learning | `⚙️ Partial` | Aggregation, bootstrap, emitters, live reward updates, and stable/canary channel selection exist; production multi-cluster rollout is not claimed |
| Multi-cloud arbitrage | `⚙️ Partial` | AWS Price List Query (SigV4), GCP Cloud Billing Catalog, and Azure Retail Prices clients with TTL caching, retries, and bundled fallbacks; spot prices are on-demand estimates |
| Carbon and app intelligence | `⚙️ Partial` | JVM JMX heap and NVIDIA DCGM metrics are queried from Prometheus and feed the RL state buckets; live exporter validation is pending |
| ChatOps | `⚙️ Partial` | Tool-backed reads, confirmation tokens, and audit logging exist; LLM quality depends on the configured provider |
| Shift-left GitOps | `⚙️ Partial` | PR helpers exist, but merge-path wiring and validation are incomplete |
| Agentic execution | `⚙️ Partial` | Approve/reject/rollback execute real CRD changes with confirmation and audit |
| Node consolidation | `❌ Stub/Planned` | Deferred to v5.0; no Karpenter actuation loop |
| eBPF streaming | `❌ Stub/Planned` | Deferred to v5.0; v4 uses Prometheus telemetry |
| Model registry | `⚙️ Partial` | Converged snapshots publish to a stable channel and unstable ones to canary; spokes select with `RECONCILYX_MODEL_CHANNEL`, and operators can demote a canary; live multi-cluster rollout is unverified |

## Beta Architecture Priorities

The beta architecture is focused on five hardening goals:

1. Buildability and testability
2. Honest API/UI contracts
3. Correct CLI/API mutation behavior
4. Safe, explicit workload mutation semantics
5. Clear separation between implemented features and planned pillars

## Non-Verified Areas

The following areas should be treated as in development until upgraded by evidence:

- Federated learning end-to-end updates and cross-cluster channel rollout
- Live pricing calls against real cloud accounts (clients are unit-tested against mock endpoints)
- GPU/JVM exporter telemetry against real clusters
- Agentic write execution
- eBPF telemetry
- Karpenter node consolidation

## References

- [STATUS.md](../STATUS.md)
- [FEATURE_MATRIX.md](../FEATURE_MATRIX.md)
- [GITHUB_ISSUES.md](../GITHUB_ISSUES.md)
