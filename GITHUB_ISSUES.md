# Reconcilyx Phase Tracker

This file stands in for a GitHub Project and is the current planning tracker.

**Project status: on hiatus as of 2026-10-26.** Open items are deferred until the required external accounts, credentials, hardware, and CI minutes are available. [HIATUS.md](HIATUS.md) consolidates the remaining work, explains each blocker, and defines the evidence required to close it. No open item is being worked during the hiatus.

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

Evidence: AWS uses the authenticated Price List Query API (`AWSPriceListService.GetProducts`) with SigV4 signing and optional session tokens; GCP pages through the Cloud Billing Catalog API when an API key is configured and totals the per-vCPU and per-GB component SKUs; Azure uses the anonymous Retail Prices API, so no subscription is required. Each provider keeps its bundled catalog as a fallback and logs when a live refresh fails. `doWithRetry` retries network errors, 429, 5xx responses, and AWS 400 `ThrottlingException` responses up to three times with exponential backoff and honors `Retry-After`; `CachedPricingProvider` caches on-demand, spot, and list results for `RECONCILYX_PRICING_CACHE_TTL_HOURS` (default 1 hour). Unknown instances with no static rate now return an error instead of a fabricated price. Spot prices remain on-demand multipliers (35% AWS, 30% GCP, 32% Azure), not live spot feeds. Evidence: `go build ./...` and `go test ./internal/cloud/...` passed against mock AWS/GCP/Azure endpoints on 2026-10-23, including SigV4 headers, GCP component/pagination parsing, 429/500/throttle retries, fallback, static-gating, and cache tests. Azure was validated live on 2026-10-26 through the anonymous Retail Prices API with TLS verification enabled: `Standard_F2s_v2/eastus` returned `$0.0846/h`, `Standard_B2s/eastus` `$0.0416/h`, and `Standard_D2s_v5/eastus` `$0.0960/h`. That run exposed and fixed a meter-selection defect that could pick a Low Priority, Spot, or Windows meter as on-demand; a regression test now covers the real meter shapes. AWS and GCP remain unverified because the workstation has no valid AWS credentials (`InvalidClientTokenId`) or GCP billing API key.

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

Evidence: the first live approval (`storefront-approve-4`) applied the app's aggregate request to the `istio-proxy` sidecar. After the fix, `checkout-approve-1` resized both pods' `app` to `250m/192Mi` while `istio-proxy` stayed at `100m/64Mi`; `legacy-approve-1` (two containers, no map) failed with `proposal has no proposedResourcesByContainer and the workload has 2 containers; refusing to apply one request set to every container`, and its pods stayed at their original requests. `kubectl-reconcilyx what-if` exited 2 for `cost-increase` (+$51.50/mo) and for `risk>=70` at risk 70, and exited 0 for a $25.75/mo reduction and for `risk>=80` at risk 70. The live manager logged `"prometheus": "http://prometheus-k8s.monitoring.svc:9090"`, proving the chart environment value is now used. Prometheus-backed stabilization completion was validated later in Phase 15; a real provider PR was exercised on 2026-10-26 (Phase 18).

## Phase 15 — Prometheus-Backed Stabilization and Receipt Integrity (2026-10-24)

- `[x]` `P1 Observability` Install `kube-prometheus-stack` and expose the `prometheus-k8s` Service alias the operator expects
- `[x]` `P0 Validation` Complete a proposal through the five-minute stabilization window with a finalized learning receipt
- `[x]` `P0 Safety` Capture live pod resources in execution snapshots and fail closed on missing or divergent pods
- `[x]` `P0 Safety` Include every pod container in the resize SSA patch so previously owned requests are not removed
- `[ ]` `P0 Validation` Re-run actuation on a production-like multi-node cluster with TLS
- `[x]` `P0 Validation` Exercise the GitOps PR flow against a real GitHub or GitLab repository

Evidence: `kube-prometheus-stack` `69.4.1` ran in `monitoring` with kubelet and kube-state-metrics `up=1`; `live-demo` exposed 24 CPU-usage series and 8 limit series. Because the chart names the Prometheus Service `prometheus-kube-prometheus-prometheus` while the operator expects `prometheus-k8s`, `scripts/install-observability.sh` now creates a selecting alias and `scripts/uninstall-observability.sh` removes it. `checkout-learning-1` completed stabilization at 13:39:00Z with a finalized receipt, but its snapshot used the Deployment template (`500m/256Mi` + `100m/64Mi`) instead of the live pods (`250m/192Mi`), overstating savings (`23269m` versus `4106m` for the actual change). `CaptureSnapshot` now reads active pods and refuses ambiguous snapshots. A second defect surfaced when `storefront-learning-1` failed with `resource requests cannot be removed`: the app-only SSA patch omitted the operator-owned sidecar. `desiredResizeResources` now patches every pod container. After the 30-minute cooldown, `storefront-learning-2` captured the live `250m/192Mi` values for both containers, resized `app` to `200m/160Mi`, preserved `istio-proxy` at `250m/192Mi`, kept all containers at 0 restarts, and completed at 14:23:31Z with `learning.finalized=true`, `costSavedUSD=4106m`, `carbonSavedGramsCO2=11097`, and total `reward=1.0205197183327863`. Unit tests and the full non-e2e suite passed.

## Phase 16 — Executable End-to-End Harness (2026-10-24)

- `[x]` `P0 Validation` Add `scripts/validation/e2e.sh` and `make validate-e2e` with isolated, self-cleaning fixtures
- `[x]` `P0 Validation` Assert live-pod snapshot, sidecar and limit preservation, unchanged pod UIDs, and zero restarts
- `[x]` `P0 Validation` Assert the finalized `4106m` learning receipt, audit chain, and Kubernetes events after `Completed`
- `[x]` `P0 Wiring` Resolve namespace and workload labels before policy matching
- `[x]` `P0 RBAC` Grant the manager `namespaces` get/list/watch
- `[x]` `P1 CI` Wire the harness into CI against a disposable Kind cluster with Prometheus and a deployed manager
- `[ ]` `P0 Validation` Run the harness on a production-like multi-node cluster with TLS

Evidence: run `20261024155951` passed 66 checks with 0 failures from 15:59:51Z to 16:05:10Z. Its snapshot recorded live `250m/192Mi` for both containers against a `500m/256Mi` + `100m/64Mi` template; the resize touched only `app` (`200m/160Mi`); sidecar requests, all limits, pod UIDs, readiness, and 0 restart counts were preserved; the proposal reached `Completed` with a finalized receipt (`costSavedUSD=4106m`, `carbonSavedGramsCO2=11097`, total `reward=1.0205197183327863`); and the audit JSONL plus events recorded evaluation, JIT allow, resize, and completion. Building the harness exposed two product defects: the controller passed empty label maps to `Matcher.Match` (selector-scoped policies could never match), and the manager lacked `namespaces` RBAC (`namespaces is forbidden`). Both are fixed, with an envtest covering selector matching; the controller suite reported `Ran 12 of 12 Specs`, all passing. CI wiring was added on 2026-10-25 in `.github/workflows/validate-e2e.yml`; the first GitHub-hosted run has not been observed yet, so it is not cited as evidence.

## Phase 17 — Live Console Validation (2026-10-25)

- `[x]` `P0 Validation` Exercise Savings, Forecast, and Settings against the live Hub (Kind + Prometheus), plus an Overview regression pass
- `[x]` `P0 Fix` Exclude failed and rolled-back proposals from the savings ledger and its UI line items
- `[x]` `P0 Fix` Fall back to the execution snapshot instead of serializing `0` when a proposal has no observed request
- `[x]` `P0 Fix` Require positive observed baselines in the Forecast simulator and confirm it issues `POST /api/what-if`
- `[x]` `P1 Fix` Serve SPA deep links publicly while keeping `/api/*` gated
- `[x]` `P1 Fix` Collapse same-day ledger rows into daily buckets and give Overview a true 30-day window
- `[x]` `P1 Validation` Exercise the console through a local TLS reverse proxy on Kind with certificate verification enabled
- `[ ]` `P0 Validation` Exercise the console through a TLS reverse proxy on a production-like cluster

Evidence: image `reconcilyx/operator:live-validation-11` served all six console views from Kind v1.36.1 over a loopback port-forward with zero console messages on Savings, Forecast, Settings, and Overview. Savings rendered `$18.25` / `4.2 kg CO₂` / `230 g per $` with one Completed/Approved row; the API series had four points with a single `18.25` value. Forecast read `live-demo/storefront` at `250m/192Mi`, proposed `180m/160Mi`, issued `GET /api/forecast/storefront?namespace=live-demo` (200) and `POST /api/what-if` (200), and rendered risk 15, confidence 78, `$1.76/mo`. Settings showed the control plane REACHABLE/ONLINE, health `ok`, a masked key, and UI build `2.0.0`. Overview showed 141 proposals, `$18`, `4.2 kg CO₂`, 99% mean confidence, and fleet `hub` READY. Document routes returned 200 without a key while `/api/proposals`, `/api/metrics/savings`, `/api/policies`, and `/api/clusters` returned 401. This pass fixed a failed-proposal savings overcount, a fabricated `0` proposal baseline, an inert what-if simulation, and 401s on SPA deep links; focused `internal/api` tests and the UI build passed.

A follow-up chart pass rebuilt the UI as `reconcilyx/operator:live-validation-12` because those four same-day API rows were drawn as four category points connected by a monotone curve, implying an intra-day trend the data does not contain. `ui/src/lib/ledger.ts` now validates UTC `YYYY-MM-DD` keys, sums rows sharing a date, zero-fills calendar gaps, and supports a trailing window; Overview passes `windowDays={30}` while Savings shows the full recorded range, and both charts use daily USD bars plus an unsmoothed CO₂ line. Against the live API, `buildDailyLedger(windowDays=30, today=2026-10-25)` produced exactly 30 buckets from `2026-09-26` to `2026-10-25` with one non-zero bucket `{2026-10-24, $18.25, 4200 g}`; the window sum and the all-time sum both equal `18.25`. The served bundle matched the host build (`index-BNxpuiOE.js`), the pod reported 0 restarts, and refreshed screenshots show a single Oct 24 bar with no repeated x-axis labels. `ui/tests/ledger.test.ts` reported 6 passing and 0 failing tests (same-day collapse, gap fill, injected-today window, empty window, impossible dates, totals reconciliation) through `npm test` on Node 22 type stripping, and the step is wired into `.github/workflows/test.yml`. Scope limits for that chart pass: plain HTTP on Kind and no GitHub-hosted `validate-e2e.yml` or `test.yml` run yet.

Local TLS pass on 2026-10-25 against the same image: a local CA (`Reconcilyx Local Test CA`; leaf SAN `IP:127.0.0.1`, `DNS:localhost`) terminated TLS 1.3 in a Node proxy on the Kind Docker network in front of `svc/reconcilyx`, with certificate verification enabled end to end. An in-network probe exited 0 with `SUMMARY failures=0` on 9 checks: chain and hostname verified, TLS 1.3 / `TLS_AES_256_GCM_SHA384`, `/` 200, `/api/proposals` 401 without a key and 200 with it (120,477 bytes), `/api/metrics/savings` 4 rows all `2026-10-24` summing `18.25`, `/api/health` 200, and CORS preflight echoing `http://localhost:8082` while omitting the header for `https://127.0.0.1:18443`. Containerized Chromium with the CA in its NSS store rendered all six views; every page's security details reported issuer `Reconcilyx Local Test CA` and TLS 1.3, and the live text matched the HTTP pass (141 proposals, `$18` / `4.2 kg CO₂` / 99%, `$18.25` / `230 g per $` / `Oct 24`, forecast `250m/192Mi` → `180m/160Mi` with risk 15 / confidence 78 / `$1.76`, policy `live-demo/sidecar-protection`, settings REACHABLE/ONLINE). Console output held only five expected pre-key 401s and seven external Google Fonts certificate failures from the workstation's TLS interception; no Reconcilyx API request failed. Host Chrome was blocked by that interception's re-signed loopback certificate (`ERR_CERT_AUTHORITY_INVALID`, screenshot retained) and was not bypassed. The manager pod restarted once at `08:55Z` after a kube-apiserver request timeout cost leader election; the probe and browser run both hit the replacement container. The production-like multi-node TLS item remains open; a three-node Kind cluster with ingress TLS was exercised on 2026-10-26 (Phase 18).

## Phase 18 — Multi-Node TLS and Live GitOps PR (2026-10-26)

- `[x]` `P0 Validation` Run the e2e harness over TLS through ingress-nginx on a three-node Kind cluster
- `[x]` `P0 Validation` Exercise all six console views through the ingress TLS endpoint with certificate verification enabled
- `[x]` `P0 Safety` Fail closed when GitOps PR creation fails instead of falling back to live actuation
- `[x]` `P1 Fix` Discover `deployment.yaml` directly under the configured manifests directory
- `[x]` `P0 Validation` Exercise the GitOps PR flow against a real GitHub repository, close without merging, and verify rejection with no local actuation
- `[ ]` `P0 Validation` Exercise the console through a public TLS terminator on a production-like cloud cluster
- `[x]` `P0 Validation` Exercise GitOps against a production repository with branch protection and CI — verified on `2026-11-14` (`H-06`: `Keninjavelas/Reconcilyx` `gitops-protected-target` with `build` required check, `1` required approval, `enforce_admins: true`, PR [`#4`](https://github.com/Keninjavelas/Reconcilyx/pull/4), check `132498371777` `success`, `405`/blocked auto-merge, closed unmerged → `Rejected`)
- `[x]` `P1 CI` Observe a GitHub-hosted workflow pass once the account's included Actions minutes are available — verified on `2026-11-14` for commit `38373ac1d309f542c7d2fd4270043df9d3380e00` (`Build` `43606567760`, `Lint & Security` `43606567683`, `Test` `43606567661`, `E2E Tests` `43606567637`, `Validate E2E Harness` `43606567673`)

Evidence: Kind v1.36.1, three nodes (one control plane, two workers), ingress-nginx v1.12.1, host ports 18080/18443, and Prometheus v3.1.0. A local CA (`Reconcilyx Multinode Test CA`) issued leaf `CN=reconcilyx.test` with SANs `reconcilyx.test`, `reconcilyx-multinode-control-plane`, `localhost`, `host.docker.internal`, and `127.0.0.1`; the endpoint negotiated TLS 1.3 / `TLS_AES_256_GCM_SHA384` with verification enabled. The harness run `multinode-tls-20261026055631` against manager `live-validation-12` passed 66 checks with 0 failures: `Completed`, `costSavedUSD=4106m`, carbon `11097`, reward `1.0205197183327863`, unchanged pod UIDs, preserved sidecar, 0 restarts, and the full audit/event chain. An in-network 9-check TLS probe exited 0 (`SUMMARY failures=0`), and containerized Chromium rendered all six views at 1440 px from `https://reconcilyx.test` with the expected certificate. Console output contained only expected pre-key 401s and external Google Fonts failures caused by the workstation's TLS interception; no Reconcilyx API request failed.

The first real GitOps attempt exposed two defects. The distroless manager did not trust the workstation's TLS interception root, so GitHub PR creation failed with `x509: certificate signed by unknown authority`; the controller then fell back to live actuation, resizing the pod to `250m/96Mi` while the Deployment template stayed at `500m/128Mi`. The controller now sets `PhaseFailed` and returns without actuation on PR-creation failure (`GitOPSError`), covered by a controller envtest. After mounting an explicit public CA bundle via `SSL_CERT_FILE` (verification still enabled) and publishing `live-validation-14`, the retry created PR #1 on the maintainer-owned `Keninjavelas/Reconcilyx` repository from branch `reconcilyx/reconcilyx-system-cpu-burner`. The PR changed only `workloads/cpu-burner/manifests/deployment.yaml` (`cpu` request `500m → 250m`, memory `128Mi → 96Mi`, limits unchanged). It was closed unmerged and its branch deleted; the proposal moved `AwaitingExternalApproval → Rejected` / `ApprovalRejected` with reason "GitOps pull request was closed without merging". Pod UID `95b7b1af-24a6-47cf-aa4b-01d8b8d63d5e` and requests `500m/128Mi` were unchanged before and after, and the Deployment template remained `500m/128Mi`, confirming no local actuation in GitOps mode. No third-party repository was contacted, and no token or private key is recorded in evidence. A follow-up fix added `<manifestsPath>/deployment.yaml` to manifest discovery (this repository's layout), with a regression test.

## Phase 19 — Hiatus: Deferred External Validation (2026-10-26)

The project is on hiatus. These items consolidate the still-open checks from Phases 7, 15, 16, 17, and 18; nothing here is in progress. Each item maps to a task in [HIATUS.md](HIATUS.md) with its blocker, prerequisites, and acceptance evidence.

- `[x]` `P0 CI` Observe a GitHub-hosted workflow pass once Actions minutes or a spending limit are available (H-01) — closed on `2026-11-14` (`38373ac1d309f542c7d2fd4270043df9d3380e00`):
  - `Build` [`43606567760`](https://github.com/Keninjavelas/Reconcilyx/actions/runs/43606567760) (`2026-11-14T14:46:02Z` → `14:47:08Z`, `success`)
  - `Lint & Security` [`43606567683`](https://github.com/Keninjavelas/Reconcilyx/actions/runs/43606567683) (`2026-11-14T14:46:02Z` → `14:48:38Z`, `success`)
  - `Test` [`43606567661`](https://github.com/Keninjavelas/Reconcilyx/actions/runs/43606567661) (`2026-11-14T14:46:02Z` → `14:51:35Z`, `success`, Linux `-race`)
  - `E2E Tests` [`43606567637`](https://github.com/Keninjavelas/Reconcilyx/actions/runs/43606567637) (`2026-11-14T14:46:02Z` → `14:52:56Z`, `success`)
  - `Validate E2E Harness` [`43606567673`](https://github.com/Keninjavelas/Reconcilyx/actions/runs/43606567673) (`2026-11-14T14:46:02Z` → `14:57:28Z`, `success`)
- `[ ]` `P1 Integration` Validate AWS live pricing with credentials that can call `pricing:GetProducts` (H-02)
- `[ ]` `P1 Integration` Validate GCP live pricing with a restricted Cloud Billing API key (H-03)
- `[ ]` `P0 Validation` Exercise a public TLS terminator on a production-like cloud cluster (H-04)
- `[ ]` `P2 Deployment` Operate a VPS or public instance with a trusted certificate (H-05, optional)
- `[x]` `P0 Validation` Exercise GitOps against a branch-protected production repository with required CI (`H-06` — closed `2026-11-14`, PR [`#4`](https://github.com/Keninjavelas/Reconcilyx/pull/4) on `gitops-protected-target` with `build` check `132498371777` + `1` required review + `enforce_admins: true`, blocked auto-merge, closed unmerged → `Rejected`, `cpu-burner` unchanged at `500m/128Mi`)
- `[x]` `P1 Validation` Run a two-cluster federated rollout: chart env wiring, configurable flush/compaction, snapshot publication, spoke bootstrap (`H-07` — closed `2026-11-14` via `.github/workflows/validate-federation.yml` run [`43610765983`](https://github.com/Keninjavelas/Reconcilyx/actions/runs/43610765983): Hub/Spoke Kind v1.37.0 clusters, compacted `v2` `modelHash=b3277814cf4dbbfe`, `isStable=true`, `outlierRejectedCount=1`, Spoke `Federated bootstrap loaded global Q-table`)
- `[ ]` `P0 Validation` Run the 66-check e2e harness on a production-like cloud cluster with Prometheus (H-08)
- `[ ]` `P1 Integration` Validate a live NVIDIA DCGM exporter and GPU buckets (H-09)
- `[x]` `P1 Documentation` Validate a live carbon provider or qualify the "live carbon" claim as credential-gated with a static default (`H-10` — closed `2026-11-14` via truthfulness qualification across `FEATURE_MATRIX.md` line 39, `docs/user-guide.md` Section 6, and `README.md` line 19)
- `[ ]` `P0 Security` Complete an independent security audit before any production-use claim (H-11)
- `[ ]` `P0 Docs` Resume, close the hiatus, refresh the audit counts only with new production-like evidence, and record the release decision (H-12)

Dependency summary: H-01, H-06, H-07, and H-10 (`2026-11-14`) are closed; H-02/H-03 start when credentials exist; H-04 and H-08 can share one cloud cluster; H-09 needs a GPU host; H-11 follows the cloud validations; H-12 is last.

## Phase 20 — `v4.1.0-rc1` Hardening, Shift-Left CI Gates & Evidence Replay (2026-11-13)

- `[x]` `P0 Federation` Replace modulo compaction with dual-trigger watermark + max-age compaction, `VerifyChecksum`, `POST /api/v1/federation/compact`, fuzzing, and Helm env wiring (H-07 architecture prerequisites)
- `[x]` `P0 Safety` Asymmetric p95/p99 sizing, OOM/restart guard, omitted CPU limits by default, KEDA `ScaledObject` conflict rejection, `1.35x` working-set in-place memory shrink headroom guard, and `RECONCILYX_EXCLUDED_NAMESPACES` deny-list
- `[x]` `P1 Engine` Two-tier node consolidation evaluator (`EvaluateNodeWithConstraints`), Kepler/OpenCost adapters, custom CA TLS (`RECONCILYX_CA_BUNDLE`), WattTime v3 `/signal-index`, extended DCGM metrics, and `/api/v1/healthz` provenance
- `[x]` `P0 CLI/CI` Add `-o markdown` and `-o sarif` (SARIF v2.1.0), `diff --offline`/`--base-manifests`, composite GitHub Action (`action.yml` & `.github/actions/reconcilyx-gate/action.yml`), and hardened CI workflows (`govulncheck`, `-race`, Helm checks)
- `[x]` `P0 UI` Add Hub Control Plane Connection Gate and Read-Only Evidence Replay Mode (`ui/src/lib/evidenceReplay.ts`) backed by verified Run `#20261026055631`
- `[x]` `P0 Security` Split-RBAC read-only Helm defaults, `controller-gen` RBAC sync, `RateLimiter`, CSP/HSTS/body-limit middleware, webhook replay window, and `docs/THREAT_MODEL.md`
- `[x]` `P0 Zero-Trust` Multi-identity inbound RBAC (`pkg/security/auth`: K8s `TokenReview`+SAR, OIDC/GitHub Actions JWKS, mTLS SPIFFE spoke cluster binding, SHA-256 scoped keys) & outbound secret isolation (`pkg/security/credentials`: `SecretString`, `fsnotify` tmpfs mount hot-reload, AWS IRSA STS, GCP Workload Identity, GitHub App ephemeral tokens)
- `[x]` `P0 Resilience` Pipeline hardening: full-jitter exponential backoff (`pkg/httpretry` enforcing idempotent-only retries and honoring `429 Retry-After`), durable spoke FIFO disk spooling (`internal/federated/spool.go`) + `X-Reconcilyx-Idempotency-Key` / Hub `batchId` deduplication, watermarked Prometheus backfill (`internal/telemetry/backfill.go` with SHA-256 key, `POST /api/v1/telemetry/backfill`, `kubectl-reconcilyx backfill --step`), and self-monitoring metrics (`pkg/pipelinemetrics`)
- `[x]` `P0 DevOps/SRE` Add `deploy/terraform/` (EKS/GKE workload identity, `restricted` PSS, remote state backends, atomic `helm_release`), Helm `PDB`/`HPA`/`NetworkPolicy`/`PrometheusRule`/`test-connection.yaml`, W3C `traceparent` propagation (`pkg/tracing`), `docs/RUNBOOK.md`, `.gitleaks.toml` + `trivy@v0.36.0` CI gates, and `.goreleaser.yaml` + `.github/workflows/release.yml`
- `[x]` `P1 Validation` Run `scripts/validation/federation-e2e.sh` on two Kind clusters (`H-07` runtime execution — verified `2026-11-14` on GitHub Actions run [`43610765983`](https://github.com/Keninjavelas/Reconcilyx/actions/runs/43610765983), `v2` `modelHash=b3277814cf4dbbfe`)

## Cross-Cutting Remediation (2026-10-23)

- `[x]` `P0 Safety` Detect HPA/VPA conflicts before in-place execution
- `[x]` `P1 Wiring` Patch real manifests in GitOps PRs and complete `AwaitingExternalApproval` proposals
- `[x]` `P1 Security` Gate auto-merge webhooks on the Reconcilyx marker and branch prefix
- `[x]` `P1 Wiring` Raise limits when a proposed request exceeds them during manifest patching
- `[x]` `P0 Validation` Exercise the autoscaler gate and CORS/API-key path against a running Kind cluster
- `[x]` `P0 Validation` Exercise the GitOps PR flow against a real GitHub or GitLab repository

Live evidence (2026-10-23, Kind v1.36.1, manager container with `ENABLE_WEBHOOKS=false`): with an HPA targeting `demo/web`, an AutoApply `RightSize` proposal reached `PhaseFailed` with `JIT Validation Rejected: autoscaler conflict: HorizontalPodAutoscaler demo/web targets Deployment/web`. After deleting the HPA, a second proposal passed the gate (the VPA CRD is absent, confirming missing-CRD tolerance), resized both pods in place to `250m/128Mi` requests through the `resize` subresource, and reached `Stabilizing`. The Deployment template still showed `500m/256Mi`, confirming the resize is ephemeral. Terminal `Completed` was not reached because the rollback check needs Prometheus, which was not installed; the proposal was left in `Stabilizing` by design. (Superseded by Phase 15, where Prometheus was installed and completion was validated.)

Live evidence (2026-10-24): a Propose-mode proposal was approved through the API and progressed `Pending → Approved → Scheduled → Executing → Stabilizing`; the audit ConfigMap captured the allow/apply lines, and a manual rollback restored the captured snapshot (event `Manual rollback restored the captured snapshot`). A two-container proposal with `spec.proposedResourcesByContainer` resized only `app` and left `istio-proxy` at `100m/64Mi`; the equivalent proposal without the map failed closed with `refusing to apply one request set to every container` and left both pods unchanged. Stabilization completion still required Prometheus at that point; Phase 15 installed it and validated completion and the learning receipt.

Webhook checks on `:8090`: unsigned and wrong `X-Hub-Signature-256` GitHub events returned 403; a valid HMAC returned 200. GitLab events without `X-Gitlab-Token` returned 403; the correct token returned 200. The GitOps PR flow itself was exercised on 2026-10-26 against the maintainer-owned GitHub repository (Phase 18); no third-party repository was contacted.

Unit evidence: HPA/VPA lookups, Off-mode exemption, and missing-CRD tolerance are covered in `internal/controller/autoscaler_gate_test.go`; manifest patching and missing-manifest refusal in `internal/gitops/manifest_test.go` and `proposal_to_pr_test.go`; external-approval transitions in `internal/controller/gitops_approval_test.go`; webhook authorization in `internal/webhook/gitops_webhook_test.go`; CORS/API-key behavior in `internal/api/security_test.go`. `go build ./...` and the focused package tests passed on 2026-10-23.
