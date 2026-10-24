# Reconcilyx Phase Tracker

This file stands in for a GitHub Project and is the current planning tracker.

Legend:

- `P0`: release-blocking for beta
- `P1`: required to move from beta toward stable
- `Stub`: placeholder or simulated behavior
- `Wiring`: code exists but is not connected end-to-end
- `Integration`: external system or runtime coupling
- `Docs`: public claim cleanup

## Phase 0

- `[x]` `P0 Docs` Truth reset in `README.md`
- `[x]` `P0 Docs` Truth reset in `docs/ARCHITECTURE.md`
- `[x]` `P0 Docs` Downgrade chart metadata to `v4.0.0-beta`
- `[x]` `P0 Docs` Add `STATUS.md`
- `[x]` `P0 Docs` Add `FEATURE_MATRIX.md`
- `[x]` `P0 Docs` Add `GITHUB_ISSUES.md`

## Phase 1

- `[x]` `P0 Wiring` Create a real Go `ui` package for embedded frontend assets
- `[x]` `P0 Wiring` Update API server dependencies to use embedded UI FS cleanly
- `[x]` `P0 Wiring` Fix manager import/build path for UI package
- `[x]` `P0 Build` Fix kubectl plugin compile issues
- `[x]` `P0 Build` Install envtest binaries
- `[x]` `P0 Build` Get `go build ./...` green
- `[x]` `P0 Build` Get `go test ./...` green

## Phase 2

- `[x]` `P0 Contract` Align proposal list response shape
- `[x]` `P0 Contract` Align policy list response shape
- `[x]` `P0 Contract` Align forecast response shape
- `[x]` `P0 Contract` Remove premature frontend mock fallback behavior

Evidence: `ui/src/lib/api.ts` maps the proposal `{items,total}` envelope and raw policy CRDs, reads forecast `{series}`, and contains no fabricated endpoint responses; unmapped fields fail visibly instead of being invented (2026-10-23).

## Phase 3

- `[x]` `P0 Contract` Require valid JSON bodies for approve/reject/rollback clients
- `[x]` `P0 CLI` Surface real request failures in kubectl plugin
- `[x]` `P0 API` Return consistent mutation endpoint status behavior

Evidence: the console and `kubectl-reconcilyx` send `{reason}` and the namespace query; the CLI returns non-2xx responses with the server message; approve/reject/rollback return JSON action envelopes with 404/409/422 error statuses (2026-10-23).

## Phase 4

- `[x]` `P0 Actuation` Replace merge patch mutation path
- `[x]` `P0 Actuation` Use `Apply` with field ownership
- `[x]` `P0 Actuation` Target Pod `/resize` subresource
- `[x]` `P0 Actuation` Update rollback to the same mutation strategy

Evidence: `applyPodResizeSSA` owns the `reconcilyx-operator` field manager and patches `pods/resize` with `client.Apply`; both `ApplyMutation` and `ExecuteRollback` call it (2026-10-23).

## Phase 5

- `[x]` `P1 Wiring` Call `agent.Update()` after successful optimization outcomes
- `[x]` `P1 Wiring` Compute and persist real reward inputs
- `[x]` `P1 Integration` Enable federated delta emission from live learning

Evidence: pre-actuation telemetry is captured into `status.learning.pre` before the resize, and the completed/rolled-back transitions call `applyRLLearning` in the same reconcile. Reward inputs (pre/post usage, throttle, OOM, cost and carbon deltas, snapped action percentages) are persisted under `status.learning`, with `finalized` providing per-proposal idempotency. The shared `SimpleQAgent` emits a federated delta from each `Update`. Unit tests cover completion, rollback, idempotency, legacy proposals, bucket capture, and action snapping (2026-10-23).

## Phase 6

- `[x]` `P1 Wiring` Populate `JVMBucket`
- `[x]` `P1 Wiring` Populate `GPUBucket`
- `[x]` `P1 Wiring` Connect registry stable/canary model selection to live flows

Evidence: `InspectJVM`/`InspectGPU` query JMX and DCGM series from Prometheus with workload-level fallbacks; the telemetry provider merges them into container metrics, and `learningStateFromMetrics` feeds `DetermineJVMBucket`/`DetermineGPUBucket`. `ScrapeGPUMetrics` now returns real DCGM data or zeroes when no exporter exists, never synthetic values (2026-10-23). The Hub publishes converged snapshots to the stable channel and non-converged ones to canary; `GET /api/federated/global-model` selects by `channel`/`cluster`, `GET /api/federated/releases` reports channels, and `DELETE /api/federated/releases/canary` demotes a canary. Spokes bootstrap with `RECONCILYX_MODEL_CHANNEL` (default `stable`). Unit tests cover channel selection, canary rollback, bucket capture, and JVM/GPU merging; live multi-cluster rollout remains unverified.

## Phase 7

- `[x]` `P1 Integration` Replace AWS static pricing table with live API client
- `[x]` `P1 Integration` Replace GCP static pricing table with live API client
- `[x]` `P1 Integration` Replace Azure static pricing table with live API client
- `[x]` `P1 Integration` Add retry/cache/error handling around live pricing clients

Evidence: AWS uses the authenticated Price List Query API (`AWSPriceListService.GetProducts`) with SigV4 signing and optional session tokens; GCP pages through the Cloud Billing Catalog API when an API key is configured and totals the per-vCPU and per-GB component SKUs; Azure uses the anonymous Retail Prices API, so no subscription is required. Each provider keeps its bundled catalog as a fallback and logs when a live refresh fails. `doWithRetry` retries network errors, 429, 5xx responses, and AWS 400 `ThrottlingException` responses up to three times with exponential backoff and honors `Retry-After`; `CachedPricingProvider` caches on-demand, spot, and list results for `RECONCILYX_PRICING_CACHE_TTL_HOURS` (default 1 hour). Unknown instances with no static rate now return an error instead of a fabricated price. Spot prices remain on-demand multipliers (35% AWS, 30% GCP, 32% Azure), not live spot feeds. Evidence: `go build ./...` and `go test ./internal/cloud/...` passed against mock AWS/GCP/Azure endpoints on 2026-10-23, including SigV4 headers, GCP component/pagination parsing, 429/500/throttle retries, fallback, static-gating, and cache tests; calls against real cloud accounts remain unverified.

## Phase 8

- `[x]` `P1 Wiring` Replace canned chat answers with real tool-backed responses
- `[x]` `P1 Wiring` Add explicit confirmation flow for write actions
- `[x]` `P1 Wiring` Ensure all chat-driven writes land in audit log

Evidence: `internal/api/chat.go` routes messages through the agent tool dispatcher, validates `confirm:<fn>:<actionId>` tokens before write actions, and appends every read and pending-confirm action to the audit log (2026-10-23).

## Phase 9

- `[x]` `P1 Stub` Decide whether eBPF remains beta-scope
- `[x]` `P1 Stub` Decide whether node consolidation remains beta-scope
- `[x]` `P1 Integration` Implement or defer these pillars explicitly to `v5.0`

Decision: eBPF telemetry and node consolidation/Karpenter actuation are deferred to v5.0 with no committed date; v4 uses Prometheus and warns when the stubs are invoked. Documented in `README.md` and `FEATURE_MATRIX.md` (2026-10-23).

## Phase 10

- `[x]` `P1 Docs` Add an evidence-based beta deployment guide
- `[x]` `P1 Integration` Add an optional static-UI workflow (GitHub Pages)
- `[x]` `P1 Wiring` Allow a hosted UI user to configure a backend API URL locally
- `[x]` `P1 Security` Allow a hosted UI user to configure the control-plane API key locally
- `[x]` `P1 Docs` Document Kind + ngrok as an evaluation-only backend option
- `[x]` `P1 Docs` Remove dead Vercel/Pages demo claims and name the embedded console as the supported path
- `[x]` `P1 Security` Validate CORS and authentication against a deployed user-managed backend

Local pre-deploy rehearsal complete (2026-10-23): `internal/api/security_test.go` starts a real `httptest` listener on the production middleware chain and verifies missing/wrong/header/cookie API keys, public routes, dev mode, allowed/denied CORS origins, trailing-slash normalization, and preflight behavior without credentials.

Live check complete (2026-10-23): the release manager binary ran in a container against a Kind cluster with `RECONCILYX_API_KEY` and `RECONCILYX_CORS_ORIGINS=https://console.example.test/`. Over the published port, `/healthz` returned 200 without credentials, `/api/proposals` returned 401 without a key and 401 with a wrong key, 200 with `X-API-Key`, no `Access-Control-Allow-Origin` for a disallowed origin, and `Access-Control-Allow-Origin: https://console.example.test` for the trailing-slash-normalized configured origin. A preflight `OPTIONS` without an API key returned 204 with CORS headers. The embedded console at `/` returned the `Reconcilyx · Operator Console` document. Scope note: the manager spoke plain HTTP behind Docker's published port; a TLS-terminating reverse proxy (Caddy/Nginx) was not part of this check, so the VPS packaging path still needs its own run.

Decision: no GitHub Pages or Vercel deployment URL is claimed. `.github/workflows/deploy-ui.yml` remains an optional, unverified static-hosting path.

## Phase 11

P0 beta remediation, completed `2026-10-23`:

- `[x]` `P0 Actuation` JIT execution gate accepts engine-generated `RightSize` proposals
- `[x]` `P0 Health` Workload health gate performs real Deployment liveness checks
- `[x]` `P0 Build` Clean-checkout Docker and Helm packaging is reproducible
- `[x]` `P0 UX` Console rebuilt against real API contracts with a responsive proposal queue
- `[x]` `P0 Docs` Public demo and release-tracker claims truth-reset

Evidence is recorded per item in [STATUS.md](STATUS.md).

## Phase 12 — CLI Product Layer and Execution-Time Policy Guardrails (2026-10-23)

- `[x]` `P1 CLI` Replace `scan` fallback data with the real Hub proposal list and propagate failures
- `[x]` `P1 CLI` Add `explain` with predicted-versus-realized savings/carbon receipts
- `[x]` `P1 CLI` Add `what-if` with a signed cost delta, JSON output, and CI `--fail-on` exit codes
- `[x]` `P1 Shift-Left` Add `diff --manifests` with cost, carbon, guardrails, JSON, and `--fail-on`
- `[x]` `P1 Safety` Enforce `changeBudget` and `blastRadius` in the JIT execution gate
- `[x]` `P1 Docs` Document CLI commands, CI usage, and the new policy fields

Evidence: `scan` now calls `GET /api/proposals` and has no synthetic fallback; it exits non-zero on unreachable/error/malformed responses, and an empty response prints an empty state. `explain` joins the proposal detail with `status.learning` per-container receipts and computes variance. `what-if` returns `costDeltaUSDPerMonth` (signed) alongside the legacy clamped savings field. `diff` compares the working tree with a git base ref with `git show`, prices workloads with the engine cost/carbon helpers, and flags guardrails; the base file listing was fixed to run from the repository root with `--full-tree`. `OptimizationPolicy` gained `spec.changeBudget` and `spec.blastRadius`; the controller resolves the applied policy before execution, fails closed when it is missing, and rejects proposals that exceed the rolling change budget, replica cap, or canary-label requirement. Command tests use `httptest` and temporary git repositories; the full controller envtest suite passed 9/9 on 2026-10-23. Live Hub and live GitHub/GitLab validation remain open.

## Phase 13 — Live Policy Context and Controller Audit (2026-10-24)

- `[x]` `P1 Safety` Feed the newest execution timestamp and live container list into policy evaluation
- `[x]` `P1 Safety` Re-check `cooldownPeriod` in the JIT gate before change budget and blast radius
- `[x]` `P1 Observability` Replace the no-op controller auditor with a persistent ConfigMap audit trail
- `[x]` `P1 Docs` Rewrite the stale policy audit and replace the empty proposal sample

Evidence: `lastExecutionForWorkload` and `workloadContainerNames` supply `cooldownPeriod` and `workloadProtection.excludedContainers` with real state, with a proposal-namespace fallback when the workload reference omits it. `checkCooldown` runs in the policy guard. `reconcilyx-audit` stores newline-delimited JSON (500-entry cap, conflict retry) and the controller records evaluation, gate allow/reject, resize outcome, rollback, and completion. Focused unit tests and the full controller envtest suite (11/11) passed on 2026-10-24. Live Kind validation later the same day rejected `storefront-exclusion` with `[WorkloadProtection]: Workload contains protected sidecar/container: 'istio-proxy'`, rejected `storefront-cooldown` with `Cooldown active: Only 18s elapsed since last optimization, requires 30m0s`, and wrote both `policy-evaluation` and `execution-attempt` JSON lines to `live-demo/reconcilyx-audit` (`audit.jsonl`).

## Phase 14 — Live Per-Container Actuation and Config Safety (2026-10-24)

- `[x]` `P0 Safety` Add `spec.proposedResourcesByContainer` and fail closed when a multi-container proposal lacks it
- `[x]` `P0 Safety` Apply only the listed containers in the actuator, GitOps manifest patcher, and RL learning receipt
- `[x]` `P0 Build` Accept `ReduceCPURequest`, `ReduceMemoryRequest`, and `MigrateCloud` in the CRD recommendation enum
- `[x]` `P1 Config` Honor the chart's `RECONCILYX_PROMETHEUS_URL` in the flag default and the reconciler
- `[x]` `P1 CLI` Live-validate `scan`, `explain`, and `what-if` against the Kind Hub, including quoted `--fail-on` thresholds

Evidence: the first live approval (`storefront-approve-4`) applied the app's aggregate request to the `istio-proxy` sidecar. After the fix, `checkout-approve-1` resized both pods' `app` to `250m/192Mi` while `istio-proxy` stayed at `100m/64Mi`; `legacy-approve-1` (two containers, no map) failed with `proposal has no proposedResourcesByContainer and the workload has 2 containers; refusing to apply one request set to every container`, and its pods stayed at their original requests. `kubectl-reconcilyx what-if` exited 2 for `cost-increase` (+$51.50/mo) and for `risk>=70` at risk 70, and exited 0 for a $25.75/mo reduction and for `risk>=80` at risk 70. The live manager logged `"prometheus": "http://prometheus-k8s.monitoring.svc:9090"`, proving the chart environment value is now used. Prometheus-backed stabilization completion was validated later in Phase 15; a real provider PR remains open.

## Phase 15 — Prometheus-Backed Stabilization and Receipt Integrity (2026-10-24)

- `[x]` `P1 Observability` Install `kube-prometheus-stack` and expose the `prometheus-k8s` Service alias the operator expects
- `[x]` `P0 Validation` Complete a proposal through the five-minute stabilization window with a finalized learning receipt
- `[x]` `P0 Safety` Capture live pod resources in execution snapshots and fail closed on missing or divergent pods
- `[x]` `P0 Safety` Include every pod container in the resize SSA patch so previously owned requests are not removed
- `[ ]` `P0 Validation` Re-run actuation on a production-like multi-node cluster with TLS
- `[ ]` `P0 Validation` Exercise the GitOps PR flow against a real GitHub or GitLab repository

Evidence: `kube-prometheus-stack` `69.4.1` ran in `monitoring` with kubelet and kube-state-metrics `up=1`; `live-demo` exposed 24 CPU-usage series and 8 limit series. Because the chart names the Prometheus Service `prometheus-kube-prometheus-prometheus` while the operator expects `prometheus-k8s`, `scripts/install-observability.sh` now creates a selecting alias and `scripts/uninstall-observability.sh` removes it. `checkout-learning-1` completed stabilization at 13:39:00Z with a finalized receipt, but its snapshot used the Deployment template (`500m/256Mi` + `100m/64Mi`) instead of the live pods (`250m/192Mi`), overstating savings (`23269m` versus `4106m` for the actual change). `CaptureSnapshot` now reads active pods and refuses ambiguous snapshots. A second defect surfaced when `storefront-learning-1` failed with `resource requests cannot be removed`: the app-only SSA patch omitted the operator-owned sidecar. `desiredResizeResources` now patches every pod container. After the 30-minute cooldown, `storefront-learning-2` captured the live `250m/192Mi` values for both containers, resized `app` to `200m/160Mi`, preserved `istio-proxy` at `250m/192Mi`, kept all containers at 0 restarts, and completed at 14:23:31Z with `learning.finalized=true`, `costSavedUSD=4106m`, `carbonSavedGramsCO2=11097`, and total `reward=1.0205197183327863`. Unit tests and the full non-e2e suite passed.

## Phase 16 — Executable End-to-End Harness (2026-10-24)

- `[x]` `P0 Validation` Add `scripts/validation/e2e.sh` and `make validate-e2e` with isolated, self-cleaning fixtures
- `[x]` `P0 Validation` Assert live-pod snapshot, sidecar and limit preservation, unchanged pod UIDs, and zero restarts
- `[x]` `P0 Validation` Assert the finalized `4106m` learning receipt, audit chain, and Kubernetes events after `Completed`
- `[x]` `P0 Wiring` Resolve namespace and workload labels before policy matching
- `[x]` `P0 RBAC` Grant the manager `namespaces` get/list/watch
- `[x]` `P1 CI` Run the harness from CI against a disposable Kind cluster (runs locally today; no CI job yet)
- `[ ]` `P0 Validation` Run the harness on a production-like multi-node cluster with TLS

Evidence: run `20261024155951` passed 66 checks with 0 failures from 15:59:51Z to 16:05:10Z. Its snapshot recorded live `250m/192Mi` for both containers against a `500m/256Mi` + `100m/64Mi` template; the resize touched only `app` (`200m/160Mi`); sidecar requests, all limits, pod UIDs, readiness, and 0 restart counts were preserved; the proposal reached `Completed` with a finalized receipt (`costSavedUSD=4106m`, `carbonSavedGramsCO2=11097`, total `reward=1.0205197183327863`); and the audit JSONL plus events recorded evaluation, JIT allow, resize, and completion. Building the harness exposed two product defects: the controller passed empty label maps to `Matcher.Match` (selector-scoped policies could never match), and the manager lacked `namespaces` RBAC (`namespaces is forbidden`). Both are fixed, with an envtest covering selector matching; the controller suite reported `Ran 12 of 12 Specs`, all passing.

## Cross-Cutting Remediation (2026-10-23)

- `[x]` `P0 Safety` Detect HPA/VPA conflicts before in-place execution
- `[x]` `P1 Wiring` Patch real manifests in GitOps PRs and complete `AwaitingExternalApproval` proposals
- `[x]` `P1 Security` Gate auto-merge webhooks on the Reconcilyx marker and branch prefix
- `[x]` `P1 Wiring` Raise limits when a proposed request exceeds them during manifest patching
- `[x]` `P0 Validation` Exercise the autoscaler gate and CORS/API-key path against a running Kind cluster
- `[ ]` `P0 Validation` Exercise the GitOps PR flow against a real GitHub or GitLab repository

Live evidence (2026-10-23, Kind v1.36.1, manager container with `ENABLE_WEBHOOKS=false`): with an HPA targeting `demo/web`, an AutoApply `RightSize` proposal reached `PhaseFailed` with `JIT Validation Rejected: autoscaler conflict: HorizontalPodAutoscaler demo/web targets Deployment/web`. After deleting the HPA, a second proposal passed the gate (the VPA CRD is absent, confirming missing-CRD tolerance), resized both pods in place to `250m/128Mi` requests through the `resize` subresource, and reached `Stabilizing`. The Deployment template still showed `500m/256Mi`, confirming the resize is ephemeral. Terminal `Completed` was not reached because the rollback check needs Prometheus, which was not installed; the proposal was left in `Stabilizing` by design. (Superseded by Phase 15, where Prometheus was installed and completion was validated.)

Live evidence (2026-10-24): a Propose-mode proposal was approved through the API and progressed `Pending → Approved → Scheduled → Executing → Stabilizing`; the audit ConfigMap captured the allow/apply lines, and a manual rollback restored the captured snapshot (event `Manual rollback restored the captured snapshot`). A two-container proposal with `spec.proposedResourcesByContainer` resized only `app` and left `istio-proxy` at `100m/64Mi`; the equivalent proposal without the map failed closed with `refusing to apply one request set to every container` and left both pods unchanged. Stabilization completion still required Prometheus at that point; Phase 15 installed it and validated completion and the learning receipt.

Webhook checks on `:8090`: unsigned and wrong `X-Hub-Signature-256` GitHub events returned 403; a valid HMAC returned 200. GitLab events without `X-Gitlab-Token` returned 403; the correct token returned 200. The GitOps PR flow itself remains unexercised because no repository or provider token was available.

Unit evidence: HPA/VPA lookups, Off-mode exemption, and missing-CRD tolerance are covered in `internal/controller/autoscaler_gate_test.go`; manifest patching and missing-manifest refusal in `internal/gitops/manifest_test.go` and `proposal_to_pr_test.go`; external-approval transitions in `internal/controller/gitops_approval_test.go`; webhook authorization in `internal/webhook/gitops_webhook_test.go`; CORS/API-key behavior in `internal/api/security_test.go`. `go build ./...` and the focused package tests passed on 2026-10-23.
