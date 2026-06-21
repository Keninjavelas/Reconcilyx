# Actuation Readiness Review

## Executive Summary
This document serves as the final, formal Actuation Readiness Review before Reconcilyx is granted the ability to mutate live Kubernetes workloads. 

**Final Recommendation: GO WITH CONDITIONS**

The current implementation provides a highly robust evaluation and policy governance boundary. However, critical gaps exist in the schema regarding Execution State tracking, Rollback State snapshotting, and Emergency Kill-Switches. A Staff Platform Engineer cannot confidently enable actuation at 2 AM until these specific data safety and blast-radius controls are implemented.

---

## 1. Execution Readiness
**Assessment: Incomplete**
Can a proposal move from `Approved` → `Scheduled` → `Executing` without ambiguity? **No.**
- **Missing State:** The `OptimizationProposalPhase` enum in `optimizationproposal_types.go` only defines `Detected`, `Evaluating`, `Approved`, `Rejected`, and `Expired`. 
- **Remediation Required:** The schema must be expanded to include `PhaseScheduled`, `PhaseExecuting`, `PhaseStabilizing`, `PhaseCompleted`, `PhaseFailed`, and `PhaseRolledBack`. Without these, the controller has no standardized way to track the actuation side of the lifecycle natively in the `Phase` field.

## 2. Rollback Readiness
**Assessment: Dangerously Incomplete**
Can rollback be implemented cleanly today? **No.**
- **Missing Before-State Capture:** The `OptimizationProposalSpec` contains `ObservedResources` (the baseline when the engine generated the proposal). However, if an operator manually changes the workload resources *during* the `Scheduled` cooldown gap, rolling back to `Spec.ObservedResources` will revert the operator's manual changes!
- **Remediation Required:** The `OptimizationProposalStatus` MUST include a new field: `ExecutionSnapshot` or `BeforeExecutionState`. The Reconciler must populate this field precisely 1 millisecond before executing the SSA patch. This guarantees the rollback state is perfectly accurate to the moment of actuation.

## 3. GitOps Readiness
**Assessment: Adequate, but requires explicit implementation**
Can Reconcilyx safely coexist with ArgoCD/Flux? **Yes, if implemented correctly.**
- **Strategy:** Using Server-Side Apply (SSA) with `fieldManager: reconcilyx-optimizer` is the correct approach.
- **Ownership Tracking:** Reconcilyx must inject the annotation `reconcilyx.io/last-action` into the workload. If ArgoCD detects drift and reverts the `resources` block, Reconcilyx will observe the workload state deviating from its SSA payload while its annotation remains.
- **Remediation Required:** The stabilization loop must strictly verify the `resources` map matches the SSA payload. If it drifts, Reconcilyx must abort stabilization, transition the Proposal to `ExecutionState: Failed` (Reason: `GitOpsOverride`), and never attempt to re-apply.

## 4. Actuation Safety
**Assessment: Missing Blast-Radius Controls**
Assume telemetry lies, engine lies, and policy is misconfigured. Can the controller avoid damaging workloads? **Partially.**
- **Blast-Radius Controls:** Workload Protection (sidecar exclusions) and Risk/Confidence thresholds provide strong perimeter defense.
- **Missing Kill-Switches:** There is no global mechanism to immediately halt all actuation. If a cluster-wide telemetry bug causes mass false-downscaling, deleting policies is too slow.
- **Remediation Required:** Introduce a global kill-switch. E.g., The Reconciler must check for a specific ConfigMap (`reconcilyx-system/reconcilyx-config`) or an environment variable `ACTUATION_PAUSED=true` before executing any patch.

## 5. Kubernetes Correctness
**Assessment: Strong**
- **Server-Side Apply:** Excellent choice. It prevents JSON Patch index shifting and naturally detects field ownership conflicts.
- **Optimistic Concurrency:** The controller correctly leverages `Status().Update()`. If an update fails due to a conflict (`ResourceVersion` mismatch), the controller-runtime will requeue, preventing silent state corruption.
- **Flaw:** Emitting events is not a substitute for durable auditing. The `ExecutionAuditRecord` ConfigMap logic designed in V2 is not yet present in the schema.

## 6. Production Simulation Review
**Scenario:** 100 workloads, 20 proposals, 5 concurrent reconciles, GitOps active.
**What breaks?**
1. **API Thrashing:** If 5 Reconcilers all trigger SSA at once on GitOps-managed workloads, ArgoCD will instantly fight back, causing an API Server spike. The GitOps Yield logic must be flawless.
2. **State Corruption:** Because `OptimizationProposalPhase` lacks execution states, the 20 proposals will be stuck in `Approved` phase while their `ExecutionState` says `Executing`, creating dashboard chaos and ambiguous logic branches.

---

## 7. Portfolio Review & Required Remediations
As a Staff Platform Engineer, I am assigning a **GO WITH CONDITIONS**. Implementation of actuation code remains blocked until the following schema and safety upgrades are committed:

### Required Remediations (The "Kill-Switch & Snapshot" Update)
1. **Expand `OptimizationProposalPhase`:** Add `PhaseScheduled`, `PhaseExecuting`, `PhaseStabilizing`, `PhaseCompleted`, `PhaseFailed`, and `PhaseRolledBack`.
2. **Add `ExecutionSnapshot` to Status:** Introduce `Status.BeforeExecutionResources` to guarantee accurate rollback, ignoring the engine's older `Spec.ObservedResources`.
3. **Add Global Kill-Switch:** Implement an `ACTUATION_PAUSED` environment variable or ConfigMap check in the `ExecutionGate`. If paused, proposals must remain `Scheduled` indefinitely.
4. **Define Audit Record Schema:** Create the exact Go struct for `ExecutionAuditRecord` so the `Auditor` interface can serialize it to a ConfigMap.

Once these 4 items are implemented in the API/Controller infrastructure, Phase 2 Actuation may safely begin.
