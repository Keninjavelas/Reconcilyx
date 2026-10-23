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

### 2026-10-23 — P1: learning feedback after optimization outcomes

- The controller now captures the pre-actuation telemetry state before applying a resize and persists it in `status.learning.pre`; the reward update runs inside the same reconcile that marks a proposal `Completed` or `RolledBack` instead of waiting for an unrelated event.
- `applyRLLearning` computes reward from the stored pre-state, post-actuation telemetry, replica count, cost/carbon deltas, throttling, and OOM evidence. It persists per-container results and a finalized outcome in `status.learning`, which also makes the update idempotent per proposal; the process-wide learning flag is gone.
- Actions are trained with the snapped percentage that was actually applied, and rollbacks restore the pre-actuation requests in the next state while penalizing the attempted action. A rolled-back proposal is no longer silently skipped.
- The shared Q-agent already carries the federated emitter, so live `Update` calls now produce deltas for the spoke path. A failed post-actuation telemetry read leaves the record unfinalized and requeues the terminal proposal after one minute for a retry.
- Evidence: `go build ./...` passed; focused controller tests for completion, rollback, idempotency, legacy skip, bucket capture, and action snapping passed. CRDs and deepcopy code were regenerated for the new status fields.

### 2026-10-23 — P0: public claims truth-reset

- Removed the unreachable Vercel demo link from `README.md` and `FEATURE_MATRIX.md`; replaced it with the embedded-console model. No public hosted instance is claimed.
- Deleted the non-existent Helm repository (`https://charts.reconcilyx.io`) from `docs/user-guide.md`; installation now uses the bundled `charts/reconcilyx` and states that no remote chart repository is published.
- Rewrote `docs/user-guide.md` around workflows that exist in the beta: policies, proposal lifecycle, `Observe`/`Propose`/`AutoApply`, CLI and console usage, BYOK ChatOps with confirmation and audit, and GitOps. Removed claims for live Karpenter/eBPF actuation, DCGM GPU scraping, JVM `-Xmx` actuation, executed multi-cloud migration, and complete federated/model-registry rollout.
- Corrected canned assistant answers in `internal/api/chat.go`: the in-place path is SSA against Pod `resize` by the `reconcilyx-operator` field manager with no rolling-restart fallback; `carbonWeight` is 0–1 and does not schedule around grid windows; the RL answer no longer claims pprof-based rollback or a 60% cut ceiling; pricing fallback is described as bundled rather than monthly-refreshed; the forecast hint no longer points at a non-existent `#/forecast/<workload>` route.
- Updated `README.md` development order (`npm ci` and `npm run build` before `go build`, because the binary embeds `ui/dist`), `docs/DEPLOYMENT.md`, `ui/README.md`, `PROJECT.md`, `FEATURE_MATRIX.md`, and `GITHUB_ISSUES.md` to match the same reality.
- Evidence: `go build ./...` and `go vet ./internal/api/` passed after the assistant-text edits (2026-10-23). Gaps that still need a running control plane are listed in `GITHUB_ISSUES.md` Phase 10 rather than being described as done.

### 2026-10-23 — P0: console redesign and contract-correct UI

- The embedded console was rebuilt on an editorial design system (warm ink canvas, acid-lime accent, serif display type, hairline grids) across Overview, Proposals, Proposal detail, Savings, Forecast, Policies, and Settings.
- Every page was re-pointed at the real API contracts: proposal envelopes, `{proposal, events}` detail, raw `OptimizationPolicy` resources, `{status,time}` health, and audit `items`. Invented KPI endpoints and stale fields were removed.
- Proposals gained a responsive card queue below `lg` with selection, bulk approve/reject, rollback, and a detail drawer; desktop keeps the table.
- Policies now creates a real `optimization.reconcilyx.io/v1alpha1` policy with a live manifest preview. Settings tests the control plane, stores the assistant key and monthly goal in the browser, and documents server-side environment variables instead of faking saves.
- Savings now aggregates the real per-proposal series by creation date and labels it as a ledger, not a billing statement. Forecast and Savings charts use `ComposedChart` so the CO₂ line renders; animations are disabled for deterministic rendering.
- Accessibility fixes: labeled proposal checkboxes and actions, non-control form groups no longer wrap buttons in `<label>`, and the monthly goal input validates. Auto-refresh and the savings goal now persist in the browser.
- Removed the unused legacy `services/api.ts` client and unreferenced starter assets.
- Evidence: `npm run build` (tsc + Vite) passed. Browser interaction run at a 638 px viewport against a local mock control plane verified bulk approve, drawer approve, reject, rollback, policy creation and list refresh, health test (75 ms), assistant save and chat round-trip, shortcuts, and persisted preferences with zero console errors and no horizontal overflow (`scrollWidth` 623 ≤ 638). Desktop pages were separately checked at 1440 px. This validates UI behaviour against mock API shapes, not the operator against a live cluster.

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
