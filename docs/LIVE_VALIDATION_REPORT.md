# Live Validation Report

**Date:** June 21, 2026
**Environment:** Local `kind` cluster (`v1.36.1`)
**OS:** Windows 11 (PowerShell via `winget` installation)

## Cluster State
- **Control Plane:** Running (`https://127.0.0.1:19337`)
- **Telemetry Stack:** `prometheus-community/kube-prometheus-stack` deployed via Helm in the `monitoring` namespace.
- **Reconcilyx Stack:** Controller running locally via `go run ./cmd/main.go`, directly bridging to the Kind API server. CRDs applied.

## Validation Scenarios Executed

### Scenario 1: CPU Overprovisioning Remediation
- **Target:** `deployment.apps/cpu-hog` in `reconcilyx-demo`
- **Initial State:** Requesting `2000m` CPU.
- **Trigger:** Engine generated `OptimizationProposal` citing 98% confidence score.
- **Policy:** `strict-efficiency-policy` in `AutoApply` mode.

### Issues Encountered & Resolved
1. **JIT Validation Phase Mismatch:** 
   - **Issue:** The execution gate originally rejected the proposal with `JIT Validation Rejected: proposal is not in Approved phase` because the state machine transitions to `Scheduled` right before JIT execution.
   - **Fix:** Corrected `execution_gate.go` to explicitly allow execution validations in the `Scheduled` phase.
2. **Server-Side Apply Field Conflict:**
   - **Issue:** The actuator originally threw `Apply failed with 1 conflict: conflict with "kubectl-client-side-apply"` because the `Force` boolean was `false`.
   - **Fix:** Enabled `Force: true` in the `PatchOptions` so the Reconcilyx operator can correctly seize ownership of the `requests.cpu` field from `kubectl`.

## Execution Results
The autonomous execution loop succeeded flawlessly in under 1 second. The workload was scaled down, stabilized, and marked `Completed`. Evidence has been extracted to `artifacts/live-demo/`.
