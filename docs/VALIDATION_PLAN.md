# Reconcilyx Validation Plan

This document defines the functional validation strategy and reproducible scenario testing required to prove Reconcilyx is production-ready. 

## 1. Functional Validation

| Component | Validation Goal | Success Criteria |
|-----------|-----------------|------------------|
| **Telemetry** | Verify accurate data ingestion | Engine receives P95 usage for CPU and Memory that matches raw PromQL queries. |
| **Analysis** | Verify scoring determinism | Given identical telemetry, the engine consistently produces identical confidence/risk scores. |
| **Policy** | Verify boundary enforcement | Proposals violating the Risk Tolerance or Cooldown Period are strictly rejected. |
| **Lifecycle** | Verify state progression | Proposals move linearly: Detected → Evaluating → Approved → Scheduled → Executing. |
| **Actuation** | Verify SSA field isolation | Mutating workload CPU requests via Reconcilyx does NOT alter limits, replicas, or image tags. |

---

## 2. Scenario Testing

### Scenario 1: Overprovisioned CPU Workload
- **Setup:** Deploy `nginx` with `requests.cpu=1000m`. Generate 50m of steady CPU load.
- **Expected Behavior:** Engine proposes `ReduceCPURequest`. Policy approves. Actuator applies patch.
- **Success Criteria:** Workload `requests.cpu` is lowered to ~100m. Status shows `Completed`.
- **Evidence:** `kubectl get deploy nginx -o yaml` before/after.

### Scenario 2: Overprovisioned Memory Workload
- **Setup:** Deploy `redis` with `requests.memory=2Gi`. Use ~100Mi consistently.
- **Expected Behavior:** Engine proposes `ReduceMemoryRequest`.
- **Success Criteria:** Workload `requests.memory` lowered to ~200Mi.
- **Evidence:** Proposal `ExpectedImpact` correctly calculates the MB delta.

### Scenario 3: Idle Workload
- **Setup:** Deploy generic pod with `0` traffic for 7 days.
- **Expected Behavior:** Engine proposes `ScaleToZero`. Policy rejects (Phase 3A limitation) or approves based on strict idle configuration.
- **Success Criteria:** Proper policy governance is logged.
- **Evidence:** `OptimizationProposal` Status Reason: "ScaleToZero forbidden by Policy".

### Scenario 4: Policy Rejection (Risk Tolerance)
- **Setup:** Set Policy `RiskTolerance: 10`. Generate highly volatile telemetry.
- **Expected Behavior:** Engine calculates Risk Score `85`. Policy rejects.
- **Success Criteria:** Proposal transitions to `Rejected`. No execution occurs.
- **Evidence:** Kubernetes Events: `Warning ProposalRejected Risk threshold exceeded`.

### Scenario 5: Cooldown Rejection
- **Setup:** Set Policy `CooldownPeriod: 24h`. Mutate a workload. Immediately generate a second proposal.
- **Expected Behavior:** Evaluator checks `last-action` annotation and rejects.
- **Success Criteria:** Proposal `Phase` = `Rejected`.
- **Evidence:** Controller logs: `Workload in cooldown period`.

### Scenario 6: Kill-Switch Activation
- **Setup:** Set `reconcilyx-config` ConfigMap `actuation_paused: "true"`. Approve a proposal.
- **Expected Behavior:** Proposal transitions to `Scheduled` but ValidateJIT returns `RetryLater`.
- **Success Criteria:** Workload remains untouched indefinitely until config is flipped.
- **Evidence:** Controller logs: `Execution globally paused`.

### Scenario 7: Stale Proposal Expiration
- **Setup:** Stop operator. Manually set Proposal `Phase=Approved` with an `ExpirationTime` 5 minutes in the past. Start operator.
- **Expected Behavior:** Reconciler immediately evaluates TTL.
- **Success Criteria:** Transitions to `Expired` without executing.
- **Evidence:** `Status.Phase == Expired`.

### Scenario 8: Execution Lock Conflict
- **Setup:** Inject `reconcilyx.io/execution-lock: some-other-proposal` onto the target Deployment.
- **Expected Behavior:** Actuator's `AcquireLock` yields.
- **Success Criteria:** Proposal remains in `Executing` but constantly requeues.
- **Evidence:** Controller logs: `Target workload locked by another proposal`.

### Scenario 9: GitOps Ownership Conflict
- **Setup:** Auto-Sync ArgoCD. Reconcilyx applies CPU reduction.
- **Expected Behavior:** ArgoCD instantly reverts the CPU reduction.
- **Success Criteria:** Reconcilyx `Verifying` state detects the mismatch and transitions to `Failed: GitOpsOverride`.
- **Evidence:** Metrics `reconcilyx_execution_failures_total` incremented.
