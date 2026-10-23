# Feature Matrix

This file is the current source of truth for feature and release posture. `Verified` requires an end-to-end production-like validation; implemented code and unit tests remain `Partial` until that evidence exists.

Audit baseline as of `2026-07-31`. The P0 remediation on `2026-10-23` did not re-score this inventory; it fixed release blockers and the console, and [STATUS.md](STATUS.md) records that evidence separately. No item is promoted to `Verified` without end-to-end production-like validation.

| Summary | Count |
|---|---:|
| Verified | 0 |
| Partial | 72 |
| Missing/Stubbed | 30 |

## Hosted Console

No public demo instance is operated. The React console is embedded in the Hub and served on the API port; a static build can be pointed at a Hub from the in-app settings. See [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md).

## Section Summary

| Section | Verified | Partial | Missing/Stubbed | Current Reality |
|---|---:|---:|---:|---|
| Core Engine | 0 | 9 | 1 | Strong prototype; `/resize` + SSA actuation path is wired in controller |
| Federated Intelligence | 0 | 7 | 0 | Delta emitter, FedAvg aggregator, ConfigMap snapshot, spoke bootstrap, and stable/canary channel selection all implemented; cold-start reduction validated in e2e test |
| Multi-Cloud Arbitrage | 0 | 5 | 0 | Live AWS/GCP/Azure pricing API via factory; bundled static fallback if env is missing |
| Application Intelligence | 0 | 2 | 3 | JVM JMX heap and DCGM GPU metrics are queried from Prometheus and feed RL state buckets; live exporter validation still pending |
| BYOK ChatOps | 0 | 7 | 0 | Real CRUD tool-backed answers; confirm-token safety on writes; audit log captures every LLM action (reads + pending-confirm writes) |
| Shift-Left FinOps | 0 | 3 | 1 | PR logic exists, but runtime wiring and config depth are incomplete |
| Agentic AI | 0 | 4 | 0 | approve/reject/rollback → real CRD patches + audit + dispatcher events; confirm token format validated; read actions also audited |
| Live Node Optimization | 0 | 0 | 0 | ⏳ **Moved to v5.0 planned** — Karpenter actuation / node drain out of scope for v4.0-beta. See `README.md` section "v5.0 Planned". Heuristic stub still callable; emits WARN on first use. |
| Real-Time eBPF Telemetry | 0 | 0 | 0 | ⏳ **Moved to v5.0 planned** — Cilium/Pixie/Tetragon gRPC out of scope for v4.0-beta. Fallback = Prometheus WorkloadInspector (1 min granularity). Stub emits WARN on first use. |
| Model Registry & A/B Testing | 0 | 5 | 0 | Converged snapshots publish to stable, unstable ones to canary; spokes select a channel and operators can roll back canary; live multi-cluster validation pending |
| Observability & Monitoring | 0 | 5 | 0 | Good instrumentation foundation, but not fully validated end-to-end |
| User Experience | 0 | 4 | 2 | Embedded console covers proposals, savings, forecast, policies, and settings; responsive proposal queue, assistant drawer, and confirmation prompts. UI behaviour was exercised against a mock control plane, not a live cluster. |
| Deployment & Packaging | 0 | 5 | 1 | Helm/CLI, embedded UI, and a Kind + ngrok evaluation guide exist; no public hosted instance is operated, and backend validation remains pending |
| Security | 0 | 6 | 1 | Webhooks and API key auth exist; federated endpoints reuse the same X-API-Key header |
| Integrations | 0 | 4 | 1 | Prometheus + GitOps + live cloud pricing + live carbon; Karpenter + eBPF moved to v5.0 |
| Documentation | 0 | 4 | 2 | README now lists real features + v5.0 planned section; FEATURE_MATRIX reflects reality |
| Roadmap | 0 | 0 | 8 | Planned items only (now includes eBPF + node consolidation deferred to v5.0) |

## Release Gate

`v4.0.0-beta` requires:

1. `go build ./...` passes
   Status: complete on `2026-07-31`
2. `go test ./...` passes with envtest binaries installed
   Status: complete on `2026-07-31`
3. API/UI contracts are validated against a running backend
4. CLI actions report and propagate real API failures
5. Live actuation uses the intended Kubernetes mutation strategy in a production-like cluster
6. The embedded console is served by the Hub, and its CORS/API-key guidance is validated against a running control plane

## Maintenance Note

This file is intentionally conservative. A feature should only move to `Verified` after end-to-end production-like validation, not because a helper, test, or facade exists.
