# Feature Matrix

This file is the current source of truth for feature and release posture. `Verified` requires an end-to-end production-like validation; implemented code and unit tests remain `Partial` until that evidence exists.

Audit baseline as of `2026-07-31`:

| Summary | Count |
|---|---:|
| Verified | 0 |
| Partial | 72 |
| Missing/Stubbed | 30 |

## 🚀 Live Demo

- **Frontend UI:** [https://reconfigy.x.vercel.app](https://reconfigy.x.vercel.app)


## Section Summary

| Section | Verified | Partial | Missing/Stubbed | Current Reality |
|---|---:|---:|---:|---|
| Core Engine | 0 | 9 | 1 | Strong prototype; `/resize` + SSA actuation path is wired in controller |
| Federated Intelligence | 0 | 7 | 0 | Delta emitter, FedAvg aggregator, ConfigMap snapshot, spoke bootstrap all implemented; cold-start reduction validated in e2e test |
| Multi-Cloud Arbitrage | 0 | 5 | 0 | Live AWS/GCP/Azure pricing API via factory; static fallback updated monthly if env missing |
| Application Intelligence | 0 | 2 | 3 | JVM helpers exist, GPU/DCGM path is stubbed |
| BYOK ChatOps | 0 | 7 | 0 | Real CRUD tool-backed answers; confirm-token safety on writes; audit log captures every LLM action (reads + pending-confirm writes) |
| Shift-Left FinOps | 0 | 3 | 1 | PR logic exists, but runtime wiring and config depth are incomplete |
| Agentic AI | 0 | 4 | 0 | approve/reject/rollback → real CRD patches + audit + dispatcher events; confirm token format validated; read actions also audited |
| Live Node Optimization | 0 | 0 | 0 | ⏳ **Moved to v5.0 planned** — Karpenter actuation / node drain out of scope for v4.0-beta. See `README.md` section "v5.0 Planned". Heuristic stub still callable; emits WARN on first use. |
| Real-Time eBPF Telemetry | 0 | 0 | 0 | ⏳ **Moved to v5.0 planned** — Cilium/Pixie/Tetragon gRPC out of scope for v4.0-beta. Fallback = Prometheus WorkloadInspector (1 min granularity). Stub emits WARN on first use. |
| Model Registry & A/B Testing | 0 | 5 | 0 | Registry pieces exist, but live rollout wiring is incomplete |
| Observability & Monitoring | 0 | 5 | 0 | Good instrumentation foundation, but not fully validated end-to-end |
| User Experience | 0 | 4 | 2 | Dashboard + AI Assistant drawer; interactive confirmation prompts in UI & CLI |
| Deployment & Packaging | 0 | 5 | 1 | Helm/CLI/demo, a GitHub Pages UI workflow, and a Kind + ngrok evaluation guide exist; public Pages, tunnel, and backend validation remain pending |
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
6. GitHub Pages deployment succeeds, and hosted-UI CORS/authentication guidance is validated

## Maintenance Note

This file is intentionally conservative. A feature should only move to `Verified` after end-to-end production-like validation, not because a helper, test, or facade exists.
