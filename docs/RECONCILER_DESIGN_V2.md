# Reconciler Design V2 (Safe Actuation Architecture)

This document replaces the V1 design and introduces strict governance, Just-In-Time (JIT) validation, and GitOps conflict prevention to eliminate the "Ghost Optimization" hazard.

## 1. Just-In-Time (JIT) Validation Layer

The gap between Policy Approval and Actual Execution is the most dangerous window in Reconcilyx. To mitigate this, we introduce the **ExecutionGate**.

Before transitioning from `Scheduled` to `Executing`, the `OptimizationProposalReconciler` MUST:
1. **Refresh Telemetry:** Query Prometheus for the last 5 minutes of target metrics.
2. **Re-evaluate Policy:** Re-run the Policy `Evaluator` against the fresh telemetry.
3. **Verify Liveness:** Check that the target workload still exists and is healthy.
4. **Verify Freshness:** Ensure the proposal has not exceeded its TTL.

**Validation Failure Transitions:**
- If Telemetry or Policy fails (e.g., traffic spiked, confidence dropped): Transition to `ApprovalState: Rejected` and `ExecutionState: Failed` with reason `JITValidationFailed`.
- If Freshness TTL is exceeded: Transition to `ExecutionState: Expired`.
- If Workload is missing: Transition to `ExecutionState: Failed` with reason `TargetNotFound`.

---

## 2. Proposal Freshness Model (TTL)

Not all optimizations age identically. A proposal's validity decays over time. We introduce a `ProposalTTL` field to the `DecisionMetadata`.

- **CPU Optimization (RightSize):** Highly volatile. Short TTL (e.g., `15m`). If executed after 15m, the cluster state has likely drifted too far.
- **Memory Optimization (RightSize):** Less volatile. Medium TTL (e.g., `1h`). Memory leaks build slowly, so proposals remain valid longer.
- **ScaleToZero:** Extremely sensitive. Very short TTL (e.g., `5m`). A dormant workload can wake up in seconds; stale zero-scaling is fatal.

The Engine sets this TTL. The Reconciler enforces it during the `ExecutionGate` check.

---

## 3. Workload Ownership & GitOps Safety

To prevent multi-reconcile collisions with external tools (ArgoCD, Flux, Human Operators), Reconcilyx will inject tracking annotations directly onto the target Workload (e.g., `Deployment.metadata.annotations`).

**Annotations:**
- `reconcilyx.io/managed: "true"`
- `reconcilyx.io/last-execution-time: "2026-06-21T10:00:00Z"`
- `reconcilyx.io/last-proposal: "frontend-opt-abc"`
- `reconcilyx.io/last-action: "execute" | "rollback"`

**GitOps Conflict Resolution:**
1. If ArgoCD reverts the Reconcilyx optimization (because it detects drift from Git), the `resources` block will change.
2. The Reconciler detects this drift during its `Verification` / `Stabilizing` loop.
3. **Action:** The Reconciler MUST abort stabilization, assuming human/GitOps override. It transitions the Proposal to `ExecutionState: Failed` with reason `GitOpsOverrideDetected`. It must NEVER fight ArgoCD in an infinite loop.

---

## 4. Liveness Verification (WorkloadHealthGate)

Before and during execution, the Reconciler utilizes the **WorkloadHealthGate**.

**Pre-Execution (JIT):**
- Workload must exist.
- `status.replicas > 0` (Unless the recommendation is `ScaleToZero`).
- `status.readyReplicas == status.replicas` (No ongoing rollouts or crashing pods).
- The Deployment must have observed its latest generation and report no `ReplicaFailure`.

*Implementation status (2026-10-23): `WorkloadHealthGate` now enforces these checks against the live Deployment; it no longer returns an unconditional pass.*

**During Stabilization:**
- Continually monitor `readyReplicas`.
- If `status.replicas == 0` (and we didn't cause it), abort.
- If K8s surfaces a `ProgressDeadlineExceeded` condition on the Deployment, trigger `RollbackRequired`.

---

## 5. Stabilization & Rollback State Machine

The V1 `Stabilizing` model was too passive. V2 introduces explicit Verification and Rollback states.

**Success Path:**
`Executing` (SSA applied) → `Verifying` (Waiting for rollout to finish) → `Stabilizing` (Monitoring error rates/latency) → `Completed`

**Failure / Rollback Path:**
`Executing` → `Verifying` (Rollout fails) → `RollbackRequired` → `RollingBack` (SSA original state) → `RollbackVerification` → `Completed` (with `ApprovalState: Rejected` to indicate optimization failed safely).

*Note: Transitioning to `Completed` after a successful rollback ensures the controller stops processing it, while the metadata indicates it was a successful safe-abort.*

---

## 6. Auditability Enhancements

A mutated cluster requires a flawless paper trail.

Every execution attempt generates an `ExecutionAuditRecord` stored in a new ConfigMap (e.g., `reconcilyx-audit-log`), containing:
- `ProposalID`
- `JITTelemetrySnapshot` (JSON)
- `PolicySnapshot` (JSON)
- `BeforeWorkloadSpec`
- `AfterWorkloadSpec`
- `Decision` (Execute, Reject, Rollback)

This guarantees that if an outage occurs 3 days later, operators have the exact mathematical and state snapshot Reconcilyx used to justify the mutation.
