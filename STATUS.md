# Reconcilyx v4.0.0-beta Status

Reconcilyx is an open-source Kubernetes optimization platform in active beta hardening.

## 🚀 Demo and Hosting

- **Dashboard:** Embedded in the Hub and served on `:8082` at `/`; no separate hosting is required.
- **Hosting:** Production VPS (Linode/Vultr/DigitalOcean), in-cluster Helm, or session-based local Kind + ngrok. See [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md).
- **Public instance:** None is currently operated. Earlier revisions pointed at a Vercel deployment that is no longer reachable; do not treat it as live.

## Audit Baseline

| Status | Count |
|---|---:|
| Verified | 0 |
| Partial | 72 |
| Missing/Stubbed | 30 |

`Verified` means an end-to-end, production-like validation exists. A unit test, a compiled package, or a feature facade does not qualify on its own.

## Current Release Posture

- Release name: `v4.0.0-beta`
- Public dashboard hosting: none currently operated; the UI is embedded in the Hub, and operators run and secure their own Hub/API via VPS or in-cluster Helm.
- GitHub Pages: workflow exists at `.github/workflows/deploy-ui.yml` but requires Pages to be enabled; no live Pages URL is claimed.
- Local demo backend: Kind plus port-forward / ngrok verified working on local workstation (2026-09-01).
- Production readiness: not claimed; requires independent security audit.
- eBPF telemetry and Karpenter / node consolidation: explicitly deferred to v5.0 planned roadmap.

## Verification Record

- `go build ./...`: passed locally on 2026-10-23 after `npm ci && npm run build` in `ui/` (the binary embeds `ui/dist`); rerun before every release
- `go test ./...`: passed locally with envtest binaries; rerun before every release
- `npm run build` in `ui/`: clean build with TypeScript and Vite on 2026-10-23
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

### 2026-10-23 — P0: clean-checkout Docker and Helm packaging

- `Dockerfile` is now a three-stage build: Node builds `ui/dist`, Go embeds it with `go:embed`, distroless nonroot runs the manager. A fresh clone can produce a working image without a local Node toolchain or committed UI assets.
- `.dockerignore` now re-includes the UI build inputs and Go module files required by those stages.
- CRDs are bundled in `charts/reconcilyx/crds/` (3 CRDs) so a Helm install no longer depends on `make install`; `make helm-crds` refreshes them after manifest generation.
- `config/rbac/role.yaml` and `config/crd/bases/*.yaml` were regenerated after adding `pods/resize`, `pods`, and `configmaps` markers. The chart RBAC gained `pods/resize` and `configmaps`, and its ClusterRole/ClusterRoleBinding no longer disappear when `serviceAccount.create=false`.
- The chart now sets liveness `/healthz` and readiness `/readyz` probes on port 8081, defaults the image tag to `.Chart.AppVersion`, and adds `helm-chart` for packaging.
- Added `.github/workflows/publish-image.yml` to publish `ghcr.io/keninjavelas/reconcilyx:<tag>` on `v*` tags.
- `docs/DEPLOYMENT.md` rewritten for the embedded-UI model, non-root kubeconfig mounting, bundled CRDs, and the current image name.
- Evidence: `npm run build` passed; `go build ./...` passed; `helm lint` passed; `helm template --include-crds` rendered 3 CRDs, resolved `ghcr.io/keninjavelas/reconcilyx:v4.0.0-beta`, and included both probes; controller gate/health tests passed (17 subtests).

See [FEATURE_MATRIX.md](FEATURE_MATRIX.md) for the feature inventory and [GITHUB_ISSUES.md](GITHUB_ISSUES.md) for the remediation tracker.
