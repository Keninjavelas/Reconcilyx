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
- `go vet ./...`: passed locally on 2026-10-23 after the CLI and policy-guard changes
- `go test ./cmd/kubectl-reconcilyx/cmd ./internal/api`: passed on 2026-10-23; CLI command tests use `httptest` Hub responses and temp git repositories, and the API test covers signed what-if cost deltas
- `go test ./internal/controller` (envtest): 9 of 9 specs passed on 2026-10-23; focused policy-guard and execution-gate tests also passed
- `npm run build` in `ui/`: clean build with TypeScript and Vite on 2026-10-23
- Controller Manager on Kind: verified running, leader election acquired, and API (`:8082`) serving live proposals (2026-09-01)
- Live Kind validation: manager container served the embedded console and authenticated API, rejected an HPA-managed resize, resized pods in place after the HPA was removed, and rejected unsigned/wrong webhook signatures (2026-10-23; details in the Remediation Log)

## Public-claim Rule

Use `v4.0.0-beta` in documentation and releases. Do not describe Reconcilyx as production-ready, fully verified, 95% complete, CNCF-approved, or commercially hosted without evidence for each specific claim.

## Remediation Log

### 2026-10-23 — P1: real CLI scan, explain/what-if/diff, and execution-time policy guardrails

- `kubectl-reconcilyx scan` now reads `GET /api/proposals` and reports the Hub's items, approval/phase state, estimated monthly savings, and carbon. The synthetic fallback report was deleted. Transport failures, non-2xx responses (with a token hint on 401/403), malformed JSON, and malformed `estimatedSavings` values all fail the command; an empty Hub response prints an empty state, not demo workloads.
- `explain <proposal>` fetches the proposal and its events, prints the policy/approval/execution path, resource deltas, predicted savings/carbon, and the finalized `status.learning` receipt with per-container cost/carbon/reward. It computes realized-minus-predicted variance and supports `-o json`. A missing namespace is resolved from the proposal list; ambiguous names are rejected.
- `what-if` posts to `/api/what-if` and supports `-o json` and `--fail-on` (`not-allowed`, `risk>=N`, `confidence<N`, `savings-below:N`, `cost-increase`). The API now returns a signed `costDeltaUSDPerMonth` (positive means increased spend) while keeping the legacy clamped `savingsUSDPerMonth`.
- `diff --manifests` reads Deployment/StatefulSet/DaemonSet manifests from the working tree and the same paths at a git base ref, prices them with the engine's cost/carbon helpers, and reports changed/added/removed workloads, guardrail violations (missing requests, request above limit, replica bounds, request-increase caps), and DaemonSet one-pod warnings. It supports `-o json` and `--fail-on cost-increase,carbon-increase,guardrail`. Git discovery was corrected to list base files from the repository root with `--full-tree`; listing from a manifests subdirectory silently produced zero base workloads.
- `OptimizationPolicy` now has `spec.changeBudget` (`maxChangesPerWindow`, `window`, default 24h) and `spec.blastRadius` (`maxReplicasPerChange`, `requireCanaryLabel`). The JIT gate resolves the applied policy, fails closed when it is missing, counts other proposals that actually executed on the same workload inside the rolling window, rejects workloads above the replica cap, and fails closed for DaemonSets when a cap is configured. CRDs, deepcopy, RBAC, and Helm CRDs were regenerated.
- Verification on 2026-10-23: `go build ./...` and `go vet ./...` passed; `go test ./cmd/kubectl-reconcilyx/cmd` passed (scan, explain, what-if, diff, fail-on, and output tests); `go test ./internal/api` passed (including the signed what-if cost delta); the full controller envtest suite passed 9 of 9 specs. `make test` now includes the CLI package instead of excluding `/cmd`.
- Not exercised: `scan`, `explain`, and `what-if` were tested against `httptest` Hub responses, not a live Hub; `diff` was tested against temporary git repositories, not a release branch. The live GitHub/GitLab PR flow remains the open validation item in `GITHUB_ISSUES.md`.

### 2026-10-23 — P0: live Kind validation of API auth, autoscaler gate, and webhook signatures

- Environment: Kind cluster `reconcilyx-live` on node image `kindest/node:v1.36.1` (kubelet `v1.36.1`), manager binary built from the current tree (HEAD `5d12d8c`, a docs-only commit over the validated code) and run in a container on the kind network with `KUBECONFIG`, `RECONCILYX_API_KEY=live-secret`, `RECONCILYX_CORS_ORIGINS=https://console.example.test/`, `AUTOMERGE_WEBHOOK_ADDR=:8090`, `GITOPS_WEBHOOK_SECRET=hook-secret`, and `ENABLE_WEBHOOKS=false` (admission webhooks were out of scope; the auto-merge listener was enabled). Ports 8082 and 8090 were published to `127.0.0.1`.
- Console and auth over HTTP: `GET /` returned 200 with the `Reconcilyx · Operator Console` document; `/healthz` returned 200 without credentials; `/api/proposals` returned 401 with no key and with a wrong key, and 200 with `X-API-Key: live-secret`. The 200 body used the real proposal envelope with live `name`/`phase`/`executionState` values. `/api/policies` returned 200 with the same key.
- Embedded console against the live backend: with `reconcilyx_api_key` set in browser storage and the API URL left same-origin, Overview rendered 3 tracked proposals, 1 policy, 1 AutoApply policy, and fleet `hub` READY; Proposals rendered 3 rows including `NO-HPA` (Stabilizing) and `HPA-CONFLICT-2` (Failed); Policies rendered `demo/autoscaler-test` in AutoApply mode. The page reported `API online` and produced no console messages. The in-app browser had no visible surface, so this was a structural DOM check (snapshot and script), not a visual screenshot; visual layout had been checked previously against a mock control plane.
- CORS: a request with `Origin: https://console.example.test` received `Access-Control-Allow-Origin: https://console.example.test`, proving the trailing slash in the environment value is normalized. A request with `Origin: https://evil.example` returned no `Access-Control-Allow-Origin` (the browser-side denial; the middleware does not turn it into a 4xx). An `OPTIONS` preflight without an API key returned 204 with the allow headers and origin.
- Autoscaler gate: with HPA `demo/web` (min 1, max 3, CPU 70%) present, an `AutoApply` policy approved a `RightSize` proposal and the JIT gate set `phase=Failed`, `executionState=Failed`, `reason="JIT Validation Rejected: autoscaler conflict: HorizontalPodAutoscaler demo/web targets Deployment/web"`. No pod mutation occurred.
- Execution after removing the HPA: the same policy approved a second proposal. With the VPA CRD absent, the gate still passed, confirming missing-CRD tolerance. The actuator acquired the lock, captured a snapshot, resized both `web` pods in place via the `resize` subresource (container requests `250m/128Mi`, limits `1/512Mi`; `status.containerStatuses[].allocatedResources` matched), and the proposal reached `Stabilizing` with `ExecutionVerified`. The Deployment template remained at `500m/256Mi`, confirming the resize is ephemeral.
- Stabilization boundary: the proposal did not reach `Completed`. After the five-minute window the rollback check calls Prometheus, which was not installed; `CheckMonitoringWindow` errors and the controller extends stabilization and requeues, by design. Terminal outcome remains unverified without a monitoring backend.
- Webhook: `POST /webhook` without a provider header returned 400 (`unknown provider`). With `X-GitHub-Event`, unsigned and wrong `X-Hub-Signature-256` requests returned 403 and a valid HMAC-SHA256 of the body with `GITOPS_WEBHOOK_SECRET` returned 200. With `X-Gitlab-Event`, a missing or wrong `X-Gitlab-Token` returned 403 and the configured token returned 200. `/healthz` on 8090 returned 200.
- Not exercised: a real GitHub/GitLab repository and token, so no live PR/MR was opened, polled, or merged; no TLS-terminating reverse proxy or VPS deployment; no Prometheus-backed stabilization completion. `GITHUB_ISSUES.md` keeps the live-repository item open and scopes the CORS item accordingly.

### 2026-10-23 — P0/P1: autoscaler gate, GitOps completion, API auth rehearsal

- Added `AutoscalerConflictGate` to the JIT execution gate. It lists HPAs (`autoscaling/v2`) and VPAs (`autoscaling.k8s.io/v1`, unstructured) in the workload namespace and rejects execution when an active autoscaler targets the Deployment. VPAs with `updateMode: Off` are recommendation-only and do not block; a missing VPA CRD or RBAC is tolerated (the rule is skipped) instead of blocking every optimization. The gate is wired in `SetupWithManager` and has RBAC markers plus Helm chart rules for both API groups.
- Replaced the placeholder GitOps diff with real manifest patching. `ResolveManifest` locates the existing Deployment YAML under the policy's `manifestsPath`, applies proposed requests to the tracked containers, raises limits when a request would exceed them, annotates `reconcilyx.io/last-proposal`, and commits the complete file. A missing manifest is an error, because a partial document in an auto-synced directory would prune live fields. Container names, unrelated documents, namespaces, and replicas are preserved.
- Completed the external-approval path: proposals entering `AwaitingExternalApproval` now record the PR ID, requeue every minute, and transition to `Completed` when the PR/MR merges, `Rejected`/`ApprovalRejected` when it is closed, or fail when no PR ID is recorded. The applied policy is re-resolved by namespace first and cluster-wide second.
- Auto-merge is gated by `spec.gitOps.autoMerge` **and** confidence ≥ 80 / risk ≤ 20. The PR body carries `<!-- reconcilyx-auto-merge: true -->`; the webhook re-reads PR state and merges only when the marker is present, the head branch starts with `reconcilyx/`, and the PR is not already merged/closed. GitHub/GitLab webhook signatures are verified. The manager starts the listener in-process when `AUTOMERGE_WEBHOOK_ADDR` is set.
- GitHub/GitLab providers are now idempotent (branch/MR/PR already exists → reuse), read repository files, report PR status, and honor the resolved `FilePath`.
- CORS/API-key behavior is exercised in `internal/api/security_test.go` over a real `httptest` listener on the production middleware chain: missing/wrong/header/cookie keys, public routes, dev mode, CORS allow/deny, trailing-slash normalization, and preflight without credentials.
- Evidence: `go build ./...` passed; `go test ./internal/gitops/... ./internal/webhook/... ./internal/api -count=1` passed; `go test ./internal/controller -run 'TestReconcileExternalApproval|TestCreateGitOpsPR|TestAutoscalerConflictGate|TestExecutionGate' -count=1` passed (2026-10-23). Superseded by the live Kind entry above, which exercised the cluster, API auth/CORS, autoscaler gate, and webhook signatures later the same day; live-repository validation remains open.

### 2026-10-23 — P1: live cloud pricing clients with retry and cache

- AWS now uses the authenticated Price List Query API (`AWSPriceListService.GetProducts`) with in-process SigV4 signing, Linux/Shared/Used filters, and optional `AWS_SESSION_TOKEN`; credentials are read from `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`, and the bundled rate table is used when they are absent.
- GCP pages through the Cloud Billing Catalog API when `RECONCILYX_PRICING_GCP_API_KEY` (or `GCP_BILLING_API_KEY`) is configured, totaling the per-vCPU and per-GB component SKUs for the predefined e2/n2 machine types. Azure uses the anonymous Retail Prices API and no longer requires a subscription ID.
- All live calls go through `doWithRetry`: network failures, 429, 5xx responses, and AWS 400 `ThrottlingException` responses retry up to three attempts with exponential backoff and `Retry-After` support. Live failures log a warning and fall back to the bundled catalog; an unknown instance without a static rate returns an error instead of a hardcoded price.
- `CachedPricingProvider` caches on-demand, spot, and instance-list results for the configured TTL (default one hour) and exposes `Live()` so startup logging reports whether each provider is using a live API.
- Removed obsolete Helm pricing values (`region`, Azure `subscriptionId`) and wired `pricing.providers.aws.sessionToken`; added a deployment section covering credentials, cache TTL, retries, fallbacks, and the fact that spot prices are estimates.
- Evidence: `go build ./...` passed; `go test ./internal/cloud/...` passed against mock AWS/GCP/Azure endpoints on 2026-10-23, covering SigV4 headers, GCP component and pagination parsing, 429/500/throttle retries, static gating, fallback, and cache hits. Calls against real cloud accounts and live spot prices remain unverified.

### 2026-10-23 — P1: JVM/GPU buckets and federated channel rollout

- `InspectJVM` and `InspectGPU` now query JMX (`jvm_memory_max_bytes`/`bytes_committed`/`bytes_used` for heap) and NVIDIA DCGM (`DCGM_FI_DEV_GPU_UTIL`, `DCGM_FI_DEV_FB_USED`, `DCGM_FI_DEV_FB_TOTAL`, `DCGM_FI_DEV_GPU_TEMP`) series from Prometheus. Exporters that omit a container label are applied at workload level; containers with no series are skipped instead of receiving synthetic zeros.
- The telemetry provider merges JVM and GPU metrics into each container, so `status.learning.pre/post` now carries real `JVMBucket` and `GPUBucket` values. `ScrapeGPUMetrics` no longer fabricates utilization or device counts; it errors without a Prometheus client and returns zeroes when no DCGM exporter exists.
- The model registry now keeps stable and canary versions separate. Registering a canary never promotes it, a stable release supersedes an older canary, and `Promote` moves an existing release to stable. The Hub auto-publishes converged snapshots to stable and non-converged ones to canary on every compaction.
- `GET /api/federated/global-model` resolves `channel` (`stable`/`canary`) and `cluster`; `GET /api/federated/releases` reports the active channels, and `DELETE /api/federated/releases/canary` demotes a canary. Spokes request a channel through `RECONCILYX_MODEL_CHANNEL` (default `stable`) with `FetchGlobalModelForChannel`, falling back to the live snapshot when a channel is empty.
- Evidence: `go build ./...` passed; `go test ./internal/telemetry/... ./internal/federated/... ./internal/api/... ./hack/e2e/...` passed on 2026-10-23. The updated e2e integration test feeds DCGM series through a mock client instead of relying on the removed synthetic values. Live exporter and multi-cluster rollout validation remain open.

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
