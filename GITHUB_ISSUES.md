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

- `[ ]` `P0 Contract` Align proposal list response shape
- `[ ]` `P0 Contract` Align policy list response shape
- `[ ]` `P0 Contract` Align forecast response shape
- `[ ]` `P0 Contract` Remove premature frontend mock fallback behavior

## Phase 3

- `[ ]` `P0 Contract` Require valid JSON bodies for approve/reject/rollback clients
- `[ ]` `P0 CLI` Surface real request failures in kubectl plugin
- `[ ]` `P0 API` Return consistent mutation endpoint status behavior

## Phase 4

- `[ ]` `P0 Actuation` Replace merge patch mutation path
- `[ ]` `P0 Actuation` Use `Apply` with field ownership
- `[ ]` `P0 Actuation` Target Pod `/resize` subresource
- `[ ]` `P0 Actuation` Update rollback to the same mutation strategy

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

- `[ ]` `P1 Wiring` Replace canned chat answers with real tool-backed responses
- `[ ]` `P1 Wiring` Add explicit confirmation flow for write actions
- `[ ]` `P1 Wiring` Ensure all chat-driven writes land in audit log

## Phase 9

- `[ ]` `P1 Stub` Decide whether eBPF remains beta-scope
- `[ ]` `P1 Stub` Decide whether node consolidation remains beta-scope
- `[ ]` `P1 Integration` Implement or defer these pillars explicitly to `v5.0`

## Phase 10

- `[x]` `P1 Docs` Add an evidence-based beta deployment guide
- `[x]` `P1 Integration` Add a GitHub Pages workflow for the static UI
- `[x]` `P1 Wiring` Allow a hosted UI user to configure a backend API URL locally
- `[x]` `P1 Security` Allow a hosted UI user to configure the control-plane API key locally
- `[x]` `P1 Docs` Document Kind + ngrok as an evaluation-only backend option
- `[ ]` `P1 Integration` Enable GitHub Pages and verify a real deployment URL
- `[ ]` `P1 Security` Validate CORS and authentication against a deployed user-managed backend
