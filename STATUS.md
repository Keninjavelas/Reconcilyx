# Reconcilyx v4.0.0-beta Status

Reconcilyx is a beta project in active hardening. It is a strong prototype, not a production-ready platform.

## Audit Baseline

| Status | Count |
|---|---:|
| Verified | 0 |
| Partial | 72 |
| Missing/Stubbed | 30 |

`Verified` means an end-to-end, production-like validation exists. A unit test, a compiled package, or a feature facade does not qualify on its own.

## Current Release Posture

- Release name: `v4.0.0-beta`
- Public dashboard hosting: workflow prepared; no public URL is claimed until GitHub Pages is enabled and a deployment succeeds
- Hosted UI backend: users operate and secure their own Hub/API; no managed service is provided
- Local demo backend: Kind plus ngrok is documented, but not verified on this workstation because Docker Desktop is stopped and ngrok is not installed/configured
- Production readiness: not claimed
- eBPF and Karpenter/node consolidation: v5.0 planned, not beta guarantees

## Verification Record

- `go build ./...`: passed locally on 2026-07-31; rerun before every release
- `go test ./...`: passed locally with envtest binaries on 2026-07-31; rerun before every release
- `npm run build` in `ui/`: passed on 2026-08-27 from a clean `npm ci` copy of `ui/`, including a `/Reconcilyx/` GitHub Pages base path
- GitHub Pages deployment: pending repository configuration and an actual workflow run

## Public-claim Rule

Use `v4.0.0-beta` in documentation and releases. Do not describe Reconcilyx as production-ready, fully verified, 95% complete, CNCF-approved, or commercially hosted without evidence for each specific claim.

See [FEATURE_MATRIX.md](FEATURE_MATRIX.md) for the feature inventory and [GITHUB_ISSUES.md](GITHUB_ISSUES.md) for the remediation tracker.
