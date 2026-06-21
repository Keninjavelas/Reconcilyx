# Mutation Strategy Review

This document formally reviews the proposed actuation architecture for Reconcilyx prior to the implementation of workload mutation. It acts as an RFC detailing exactly how Kubernetes state will be altered and governed.

## 1. Server-Side Apply (SSA) Review
**Mechanism:** Reconcilyx will submit mutations using Server-Side Apply with `fieldManager: reconcilyx-optimizer`.

**Conditions for use:**
- Reconcilyx MUST only apply SSA patches containing the exact `resources` block for the target container (and `replicas` if scaling to zero).
- Reconcilyx MUST use `Force: false` by default to avoid fighting humans. If a human has explicitly seized ownership of the `resources` map via another field manager after Reconcilyx's last action, Reconcilyx must back off and log a conflict.
- Reconcilyx MAY use `Force: true` *only* during an emergency Rollback to ensure safety.

**Refusal Conditions:**
- Reconcilyx must refuse to mutate if the workload has been deleted.
- Reconcilyx must refuse to mutate if the proposal is stale (TTL exceeded).
- Reconcilyx must refuse to mutate if the `ActuationControl` kill-switch is active.

## 2. Resource Mutation Scope
Reconcilyx operates on a strict, whitelist-only mutation scope.

**ALLOWED Mutations:**
- `spec.template.spec.containers[*].resources.requests.cpu`
- `spec.template.spec.containers[*].resources.limits.cpu`
- `spec.template.spec.containers[*].resources.requests.memory`
- `spec.template.spec.containers[*].resources.limits.memory`
- `spec.replicas` (strictly for ScaleToZero operations)
- `metadata.annotations` (for tracking `reconcilyx.io/last-action`)

**FORBIDDEN Mutations:**
- Everything else. Reconcilyx MUST NEVER mutate: `images`, `labels`, `selectors`, `affinity`, `tolerations`, `securityContext`, `volumes`, `env`, or `ports`.

## 3. GitOps Compatibility Review
Reconcilyx will operate in environments managed by ArgoCD and Flux.

**Drift Scenarios:**
- If Reconcilyx optimizes a workload, ArgoCD will detect it as "OutOfSync". 
- **Auto-Sync Off:** Reconcilyx successfully manages resources.
- **Auto-Sync On (Self-Heal):** ArgoCD will instantly overwrite Reconcilyx. 
**Resolution:** Reconcilyx tracks its ownership via the `reconcilyx.io/last-action` annotation. During the `Verifying` state, if Reconcilyx detects that the applied `resources` map no longer matches its intended state, it transitions the proposal to `Failed: GitOpsOverride`. It must *never* re-apply the patch in an infinite loop.

## 4. Concurrency Review
**Execution Locking Strategy:**
Multiple proposals may target the same workload. If `Proposal A` and `Proposal B` both reach the `Scheduled` phase, they cannot execute simultaneously.
- **Mechanism:** The Reconciler uses an optimistic lock on the target workload via a specific annotation: `reconcilyx.io/active-proposal: <proposal-name>`.
- Before mutating, the Reconciler reads the workload. If the annotation exists and points to a *different* active proposal, it aborts execution and yields.
- Once a proposal reaches a terminal state (`Completed`, `Failed`, `RolledBack`), the annotation is removed, releasing the lock.

## 5. Blast Radius Review (Actuation Limits)
Actuation must be throttle-able.
- **Max Workload Mutations per Hour:** Hard limit of `1` per workload. Enforced by the `CooldownConstraint` in the Policy Engine.
- **Max Concurrent Executions:** Global limit of `10` simultaneous executions across the cluster to prevent control-plane API spiking.
- **Max Namespace Impact:** Enforced via `TargetSelectors` in the Policy.
- **Max Rollback Attempts:** `1` per execution. If a rollback fails to stabilize the workload, Reconcilyx transitions to `Failed: RollbackFailed` and requires human intervention. It must not loop.

## 6. Rollback Correctness Review
**When must rollback be refused?**
- **Snapshot Drift:** If `Status.ExecutionSnapshot` is empty, rollback is absolutely refused.
- **Workload Drift:** If the human operator manually patched the Deployment (e.g., changed the image tag) *after* Reconcilyx executed, rolling back via SSA with `Force: true` is safe because Reconcilyx only owns the `resources` map, leaving the new image tag intact. However, if the human *also* changed the `resources` map, Reconcilyx must refuse rollback and yield to the human (`Failed: HumanOverride`).
- **Target Deletion:** If the target no longer exists, rollback is refused.

## 7. Portfolio Review
Would a Staff Kubernetes Engineer sign off on this RFC? 
**Yes.** The combination of SSA's field-level granularity, the explicit whitelist of mutable fields, and the optimistic execution locking strategy provides a mathematically bounded blast radius. The GitOps yielding behavior prevents control-plane thrashing, making it safe for autonomous operation.
