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
| Multi-Cloud Arbitrage | 0 | 5 | 0 | AWS/GCP/Azure live list-price clients with TTL cache, retries, and bundled fallbacks; spot prices are on-demand estimates; no live-account validation yet |
| Application Intelligence | 0 | 2 | 3 | JVM JMX heap and DCGM GPU metrics are queried from Prometheus and feed RL state buckets; live exporter validation still pending |
| BYOK ChatOps | 0 | 7 | 0 | Real CRUD tool-backed answers; confirm-token safety on writes; audit log captures every LLM action (reads + pending-confirm writes) |
| Shift-Left FinOps | 0 | 3 | 1 | Policy-driven manifest patching, idempotent PR/MR creation, confidence/risk-gated auto-merge, webhook authorization, external-approval polling, and a git-backed `kubectl-reconcilyx diff` with cost/carbon/guardrail CI exit codes are wired; live repository validation pending |
| Agentic AI | 0 | 4 | 0 | approve/reject/rollback → real CRD patches + audit + dispatcher events; confirm token format validated; read actions also audited |
| Live Node Optimization | 0 | 0 | 0 | ⏳ **Moved to v5.0 planned** — Karpenter actuation / node drain out of scope for v4.0-beta. See `README.md` section "v5.0 Planned". Heuristic stub still callable; emits WARN on first use. |
| Real-Time eBPF Telemetry | 0 | 0 | 0 | ⏳ **Moved to v5.0 planned** — Cilium/Pixie/Tetragon gRPC out of scope for v4.0-beta. Fallback = Prometheus WorkloadInspector (1 min granularity). Stub emits WARN on first use. |
| Model Registry & A/B Testing | 0 | 5 | 0 | Converged snapshots publish to stable, unstable ones to canary; spokes select a channel and operators can roll back canary; live multi-cluster validation pending |
| Observability & Monitoring | 0 | 5 | 0 | Good instrumentation foundation, but not fully validated end-to-end |
| User Experience | 0 | 4 | 2 | Embedded console covers proposals, savings, forecast, policies, and settings; responsive proposal queue, assistant drawer, and confirmation prompts. Overview, Proposals, and Policies rendered live Kind-backed data on 2026-10-23 with zero console errors; Savings/Forecast/Settings were last exercised against a mock control plane. |
| Deployment & Packaging | 0 | 5 | 1 | Helm/CLI, embedded UI, and a Kind + ngrok evaluation guide exist; the release manager was exercised in a container against Kind, but a VPS/TLS-proxy deployment remains unverified and no public hosted instance is operated |
| Security | 0 | 6 | 1 | API-key auth and CORS were exercised against a running manager (401/200, allowed/denied origins, preflight); auto-merge webhook rejects unsigned/wrong-token events with 403 and requires a Reconcilyx marker plus a `reconcilyx/` head branch; federated endpoints reuse the same X-API-Key header |
| Integrations | 0 | 4 | 1 | Prometheus + GitOps (manifest patching, external approval, auto-merge webhook) + live cloud pricing + live carbon; Karpenter + eBPF moved to v5.0 |
| Documentation | 0 | 4 | 2 | README now lists real features + v5.0 planned section; FEATURE_MATRIX reflects reality |
| Roadmap | 0 | 0 | 8 | Planned items only (now includes eBPF + node consolidation deferred to v5.0) |

## Release Gate

`v4.0.0-beta` requires:

1. `go build ./...` passes
   Status: complete on `2026-07-31`
2. `go test ./...` passes with envtest binaries installed
   Status: complete on `2026-07-31`
3. API/UI contracts are validated against a running backend
   Status: partial on 2026-10-23 — the embedded console rendered live Kind-backed data on Overview (3 proposals, 1 policy, fleet `hub` READY), Proposals (3 rows with phases), and Policies (`demo/autoscaler-test`, AutoApply) with zero console messages; Savings, Forecast, and Settings were not re-exercised against the live backend
4. CLI actions report and propagate real API failures
   Status: complete on 2026-10-23 — `scan` no longer fabricates fallback workloads and fails on transport, non-2xx, and parse errors; `explain` and `what-if` propagate Hub errors; `diff` returns exit 1 for unreachable refs/malformed manifests and exit 2 for triggered `--fail-on` conditions. Command tests use `httptest` Hub responses and temporary git repositories; a live-Hub run of the new commands was not repeated.
5. Live actuation uses the intended Kubernetes mutation strategy in a production-like cluster
   Status: partial on 2026-10-23 — the manager resized both `demo/web` pods in place through the Pod `resize` subresource on Kind v1.36.1 and reached `Stabilizing`; a production-like cluster and Prometheus-backed stabilization completion remain unverified
6. The embedded console is served by the Hub, and its CORS/API-key guidance is validated against a running control plane
   Status: complete on 2026-10-23 — the Hub served the embedded console, API-key auth returned 401/200 as configured, and CORS allowed the configured origin (trailing slash normalized) while omitting the header for a disallowed origin; exercised over plain HTTP, not a TLS reverse proxy

## Maintenance Note

This file is intentionally conservative. A feature should only move to `Verified` after end-to-end production-like validation, not because a helper, test, or facade exists.
