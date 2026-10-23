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

## Remediation Log

### 2026-10-23 — P0: JIT gate accepts engine-generated RightSize proposals

- `internal/controller/actuator.go` and `internal/controller/execution_gate.go` now share `isActuatableRecommendation`, removing the drift that rejected every engine-generated `RightSize` proposal at execution time.
- Added `internal/controller/execution_gate_test.go`: RightSize, ReduceCPURequest, and ReduceMemoryRequest execute; ScaleToZero and MigrateCloud are rejected; kill switch, health, phase, and TTL paths are covered.
- Evidence: `go test -c` + `controller.test.exe -test.run TestExecutionGate` passed (13 subtests).

### 2026-10-23 — P0: WorkloadHealthGate performs real liveness checks

- `internal/controller/health_gate.go` now verifies the target Deployment exists, is not deleting or scaled to zero, has observed its latest generation, is fully ready, and reports no `ReplicaFailure`.
- Wired the manager client into the gate at `optimizationproposal_controller.go:529`.
- Added `internal/controller/health_gate_test.go` covering healthy, missing, unsupported-kind, partial-readiness, zero-replica, stale-generation, and ReplicaFailure cases.
- Design intent documented in `docs/RECONCILER_DESIGN_V2.md`. Evidence: health and gate test suite passed.

See [FEATURE_MATRIX.md](FEATURE_MATRIX.md) for the feature inventory and [GITHUB_ISSUES.md](GITHUB_ISSUES.md) for the remediation tracker.
