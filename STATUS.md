# Reconcilyx v4.0.0-beta Status

Reconcilyx is an open-source Kubernetes optimization platform in active beta hardening.

## 🚀 Live Demo

- **Frontend UI:** [https://reconfigy.x.vercel.app](https://reconfigy.x.vercel.app)
- **Deployment Mechanics:** Supported via production VPS (Linode/Vultr/DigitalOcean), in-cluster Helm, or session-based local Kind + ngrok.

## Audit Baseline

| Status | Count |
|---|---:|
| Verified | 0 |
| Partial | 72 |
| Missing/Stubbed | 30 |

`Verified` means an end-to-end, production-like validation exists. A unit test, a compiled package, or a feature facade does not qualify on its own.

## Current Release Posture

- Release name: `v4.0.0-beta`
- Public dashboard hosting: Live on Vercel at [https://reconfigy.x.vercel.app](https://reconfigy.x.vercel.app); fallback GitHub Pages workflow in place.
- Hosted UI backend: operators run and secure their own Hub/API (via VPS or in-cluster Helm); no third-party managed backend is provided.
- Local demo backend: Kind plus port-forward / ngrok verified working on local workstation (2026-09-01).
- Production readiness: not claimed; requires independent security audit.
- eBPF telemetry and Karpenter / node consolidation: explicitly deferred to v5.0 planned roadmap.

## Verification Record

- `go build ./...`: passed locally; rerun before every release
- `go test ./...`: passed locally with envtest binaries; rerun before every release
- `npm run build` in `ui/`: verified clean build with TypeScript and Vite on 2026-09-01
- Controller Manager on Kind: verified running, leader election acquired, and API (`:8082`) serving live proposals (2026-09-01)

## Public-claim Rule

Use `v4.0.0-beta` in documentation and releases. Do not describe Reconcilyx as production-ready, fully verified, 95% complete, CNCF-approved, or commercially hosted without evidence for each specific claim.

See [FEATURE_MATRIX.md](FEATURE_MATRIX.md) for the feature inventory and [GITHUB_ISSUES.md](GITHUB_ISSUES.md) for the remediation tracker.
