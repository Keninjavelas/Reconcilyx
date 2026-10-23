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

## Cross-Cutting Remediation (2026-10-23)

- `[x]` `P0 Safety` Detect HPA/VPA conflicts before in-place execution
- `[x]` `P1 Wiring` Patch real manifests in GitOps PRs and complete `AwaitingExternalApproval` proposals
- `[x]` `P1 Security` Gate auto-merge webhooks on the Reconcilyx marker and branch prefix
- `[x]` `P1 Wiring` Raise limits when a proposed request exceeds them during manifest patching
- `[x]` `P0 Validation` Exercise the autoscaler gate and CORS/API-key path against a running Kind cluster
- `[ ]` `P0 Validation` Exercise the GitOps PR flow against a real GitHub or GitLab repository

Live evidence (2026-10-23, Kind v1.36.1, manager container with `ENABLE_WEBHOOKS=false`): with an HPA targeting `demo/web`, an AutoApply `RightSize` proposal reached `PhaseFailed` with `JIT Validation Rejected: autoscaler conflict: HorizontalPodAutoscaler demo/web targets Deployment/web`. After deleting the HPA, a second proposal passed the gate (the VPA CRD is absent, confirming missing-CRD tolerance), resized both pods in place to `250m/128Mi` requests through the `resize` subresource, and reached `Stabilizing`. The Deployment template still showed `500m/256Mi`, confirming the resize is ephemeral. Terminal `Completed` was not reached because the rollback check needs Prometheus, which was not installed; the proposal was left in `Stabilizing` by design.

Webhook checks on `:8090`: unsigned and wrong `X-Hub-Signature-256` GitHub events returned 403; a valid HMAC returned 200. GitLab events without `X-Gitlab-Token` returned 403; the correct token returned 200. The GitOps PR flow itself remains unexercised because no repository or provider token was available.

Unit evidence: HPA/VPA lookups, Off-mode exemption, and missing-CRD tolerance are covered in `internal/controller/autoscaler_gate_test.go`; manifest patching and missing-manifest refusal in `internal/gitops/manifest_test.go` and `proposal_to_pr_test.go`; external-approval transitions in `internal/controller/gitops_approval_test.go`; webhook authorization in `internal/webhook/gitops_webhook_test.go`; CORS/API-key behavior in `internal/api/security_test.go`. `go build ./...` and the focused package tests passed on 2026-10-23.
