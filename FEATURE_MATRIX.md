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
| Core Engine | 0 | 9 | 1 | Strong prototype; `/resize` + SSA actuation resizes only the containers named in `spec.proposedResourcesByContainer` and fails closed for multi-container proposals without it; live cooldown/exclusion evaluation and change-budget/blast-radius guards re-check at execution |
| Federated Intelligence | 0 | 7 | 0 | Delta emitter, FedAvg aggregator, ConfigMap snapshot, spoke bootstrap, and stable/canary channel selection all implemented; cold-start reduction validated in e2e test |
| Multi-Cloud Arbitrage | 0 | 5 | 0 | AWS/GCP/Azure live list-price clients with TTL cache, retries, and bundled fallbacks; spot prices are on-demand estimates; no live-account validation yet |
| Application Intelligence | 0 | 2 | 3 | JVM JMX heap and DCGM GPU metrics are queried from Prometheus and feed RL state buckets; live exporter validation still pending |
| BYOK ChatOps | 0 | 7 | 0 | Real CRUD tool-backed answers; confirm-token safety on writes; audit log captures every LLM action (reads + pending-confirm writes) |
| Shift-Left FinOps | 0 | 3 | 1 | Policy-driven manifest patching, idempotent PR/MR creation, confidence/risk-gated auto-merge, webhook authorization, external-approval polling, and a git-backed `kubectl-reconcilyx diff` with cost/carbon/guardrail CI exit codes are wired; a live GitHub PR round-trip on the maintainer-owned repository was verified on 2026-10-26 (create, close unmerged, branch delete, rejection, no local actuation), while a branch-protected production repository remains unverified |
| Agentic AI | 0 | 4 | 0 | approve/reject/rollback → real CRD patches + audit + dispatcher events; confirm token format validated; read actions also audited |
| Live Node Optimization | 0 | 0 | 0 | ⏳ **Moved to v5.0 planned** — Karpenter actuation / node drain out of scope for v4.0-beta. See `README.md` section "v5.0 Planned". Heuristic stub still callable; emits WARN on first use. |
| Real-Time eBPF Telemetry | 0 | 0 | 0 | ⏳ **Moved to v5.0 planned** — Cilium/Pixie/Tetragon gRPC out of scope for v4.0-beta. Fallback = Prometheus WorkloadInspector (1 min granularity). Stub emits WARN on first use. |
| Model Registry & A/B Testing | 0 | 5 | 0 | Converged snapshots publish to stable, unstable ones to canary; spokes select a channel and operators can roll back canary; live multi-cluster validation pending |
| Observability & Monitoring | 0 | 5 | 0 | Good instrumentation foundation, but not fully validated end-to-end |
| User Experience | 0 | 4 | 2 | Embedded console covers proposals, savings, forecast, policies, and settings; responsive proposal queue, assistant drawer, and confirmation prompts. All six views rendered live Kind-backed data on 2026-10-25 with zero console messages. That pass found and fixed four truth defects: the savings ledger counted approved-but-failed proposals, the proposal list emitted a fabricated `0` baseline when no observed request was recorded, the forecast simulator never POSTed a what-if because it selected zero-baseline workloads, and SPA deep links returned 401. A same-day rebuild also replaced the savings chart's interpolation of same-day rows with daily buckets over a true 30-day window. Re-exercised on 2026-10-26 through ingress-nginx on a three-node Kind cluster over TLS 1.3 (9 of 9 probe checks; all six views) alongside a 66-check TLS e2e run that reached `Completed`. |
| Deployment & Packaging | 0 | 5 | 1 | Helm/CLI, embedded UI, and a Kind + ngrok evaluation guide exist; the release manager was exercised in a container against Kind and, on 2026-10-25, behind a locally CA-verified TLS 1.3 reverse proxy, and on 2026-10-26 behind ingress-nginx on a three-node Kind cluster with a locally trusted TLS 1.3 endpoint, but a VPS deployment with an operator-managed TLS terminator remains unverified and no public hosted instance is operated |
| Security | 0 | 6 | 1 | API-key auth and CORS were exercised against a running manager (401/200, allowed/denied origins, preflight); auto-merge webhook rejects unsigned/wrong-token events with 403 and requires a Reconcilyx marker plus a `reconcilyx/` head branch; federated endpoints reuse the same X-API-Key header; policy evaluations and execution outcomes persist to the `reconcilyx-audit` ConfigMap; GitOps PR-creation failures now fail closed with `PhaseFailed` and no local actuation (envtest), after a live run exposed and fixed the previous fallback |
| Integrations | 0 | 4 | 1 | Prometheus + GitOps (manifest patching, external approval, auto-merge webhook) + live Azure pricing (anonymous API validated 2026-10-26) + live carbon; AWS/GCP pricing clients are credential-gated, and the GitHub PR flow was exercised live on a maintainer-owned repository on 2026-10-26; Karpenter + eBPF moved to v5.0 |
| Documentation | 0 | 4 | 2 | README now lists real features + v5.0 planned section; FEATURE_MATRIX reflects reality |
| Roadmap | 0 | 0 | 8 | Planned items only (now includes eBPF + node consolidation deferred to v5.0) |

## Release Gate

`v4.0.0-beta` requires:

1. `go build ./...` passes
   Status: complete on `2026-07-31`
2. `go test ./...` passes with envtest binaries installed
   Status: complete on `2026-07-31`
3. API/UI contracts are validated against a running backend
   Status: complete on 2026-10-25 — all six console views were exercised against the live Kind Hub with zero console messages. Overview reported 141 proposals, 1 policy, `$18`/`4.2 kg CO₂`, fleet `hub` READY. Savings reported `$18.25`/`4.2 kg CO₂`/`230 g per $` with one matching ledger row; Forecast read `live-demo/storefront` at `250m/192Mi`, sent `POST /api/what-if` (200) and rendered risk 15, confidence 78, `$1.76/mo`; Settings reported the control plane reachable, health `ok`, and a masked key. This pass fixed a failed-proposal savings overcount, a fabricated `0` baseline in the proposal list, an inert forecast simulation, and 401s on SPA deep links (API routes stay gated). A later same-day rebuild replaced the chart's same-day interpolation with daily buckets and a true 30-day window; six ledger unit tests cover the aggregation, zero-fill, and window math. Re-exercised on 2026-10-25 through a local TLS 1.3 reverse proxy on Kind with a locally trusted CA: a 9-check probe passed with verification enabled, and all six views reported issuer `Reconcilyx Local Test CA` over TLS 1.3. Re-exercised again on 2026-10-26 through ingress-nginx on a three-node Kind cluster over TLS 1.3, with the 9-check probe and all six views; a public TLS terminator on a cloud cluster remains unverified.
4. CLI actions report and propagate real API failures
   Status: complete on 2026-10-24 — `scan`, `proposals`, `explain`, and `what-if` ran against the live Kind Hub over its ClusterIP. `what-if` exited 2 for `cost-increase` (+$51.50/mo) and `risk>=70`, and 0 for a $25.75/mo reduction and `risk>=80`; `explain` listed proposal events after the events list/watch RBAC fix. `diff` remains covered by temporary git repositories, not a release branch.
5. Live actuation uses the intended Kubernetes mutation strategy in a production-like cluster
   Status: partial on 2026-10-26 — the manager resized pods in place through the Pod `resize` subresource on Kind v1.36.1 and reached `Stabilizing`. A two-container proposal with `proposedResourcesByContainer` changed only `app` and preserved `istio-proxy` at `100m/64Mi`; the same proposal without the map failed closed and left both pods unchanged. With Prometheus installed, `checkout-learning-1` completed stabilization and finalized a learning receipt; that receipt exposed a template-based snapshot bug, which was fixed and revalidated by `storefront-learning-2` (`Completed`, live-pod snapshot, sidecar preserved, `costSavedUSD=4106m`). The scripted harness (`make validate-e2e`) reproduced the lifecycle on run `20261024155951` with 66 checks and 0 failures, and again over TLS on a three-node Kind cluster on run `multinode-tls-20261026055631` with 66 checks and 0 failures; it is wired into `.github/workflows/validate-e2e.yml` (Kind + Prometheus + Helm), though no GitHub-hosted run has completed. The GitOps PR lifecycle was exercised live against the maintainer-owned GitHub repository on 2026-10-26. A production-like cloud cluster remains unverified.
6. The embedded console is served by the Hub, and its CORS/API-key guidance is validated against a running control plane
   Status: complete on 2026-10-26 — the Hub served the embedded console, API-key auth returned 401/200 as configured, and CORS allowed the configured origin (trailing slash normalized) while omitting the header for a disallowed origin. First exercised over plain HTTP on 2026-10-23 and repeated through a local CA-verified TLS 1.3 reverse proxy on Kind on 2026-10-25: 9 of 9 probe checks passed, and containerized Chromium loaded all six views with issuer `Reconcilyx Local Test CA`. Repeated on 2026-10-26 through ingress-nginx on a three-node Kind cluster with a locally trusted CA: 9 of 9 probe checks passed and all six views rendered over TLS 1.3. A public TLS terminator on a cloud cluster remains unverified.

## Maintenance Note

This file is intentionally conservative. A feature should only move to `Verified` after end-to-end production-like validation, not because a helper, test, or facade exists.
