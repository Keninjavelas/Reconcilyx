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

- `[ ]` `P1 Wiring` Call `agent.Update()` after successful optimization outcomes
- `[ ]` `P1 Wiring` Compute and persist real reward inputs
- `[ ]` `P1 Integration` Enable federated delta emission from live learning

## Phase 6

- `[ ]` `P1 Wiring` Populate `JVMBucket`
- `[ ]` `P1 Wiring` Populate `GPUBucket`
- `[ ]` `P1 Wiring` Connect registry stable/canary model selection to live flows

## Phase 7

- `[ ]` `P1 Integration` Replace AWS static pricing table with live API client
- `[ ]` `P1 Integration` Replace GCP static pricing table with live API client
- `[ ]` `P1 Integration` Replace Azure static pricing table with live API client
- `[ ]` `P1 Integration` Add retry/cache/error handling around live pricing clients

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
- `[ ]` `P1 Security` Validate CORS and authentication against a deployed user-managed backend

Decision: no GitHub Pages or Vercel deployment URL is claimed. `.github/workflows/deploy-ui.yml` remains an optional, unverified static-hosting path.

## Phase 11

P0 beta remediation, completed `2026-10-23`:

- `[x]` `P0 Actuation` JIT execution gate accepts engine-generated `RightSize` proposals
- `[x]` `P0 Health` Workload health gate performs real Deployment liveness checks
- `[x]` `P0 Build` Clean-checkout Docker and Helm packaging is reproducible
- `[x]` `P0 UX` Console rebuilt against real API contracts with a responsive proposal queue
- `[x]` `P0 Docs` Public demo and release-tracker claims truth-reset

Evidence is recorded per item in [STATUS.md](STATUS.md).
