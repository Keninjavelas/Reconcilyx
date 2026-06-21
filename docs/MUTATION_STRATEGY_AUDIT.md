# Mutation Strategy Audit

## Executive Summary
This document serves as the adversarial audit of the Reconcilyx Mutation Strategy prior to Phase 3 Actuation implementation. The objective is to rigorously test the proposed Server-Side Apply (SSA), GitOps yielding, and Execution Locking architectures against edge-case failures.

**Final Recommendation: GO**

The strategy effectively neutralizes API thrashing, bounds the blast radius of any single mutation, and correctly isolates field ownership to prevent colliding with GitOps tools or human operators.

---

## 1. Attack Scenarios & Mitigations

### Scenario A: The GitOps "Ping Pong" Thrash
**Attack:** Reconcilyx scales memory up. ArgoCD (with Auto-Sync enabled) detects drift and scales it back down. Reconcilyx, seeing its optimization removed, immediately re-applies the patch. The two controllers fight at infinite speed, crashing the Kubernetes API Server.
**Defense:** *Pass.* The strategy dictates that during the `Verifying` state, if Reconcilyx's intended payload does not match the actual workload, it yields immediately (`Failed: GitOpsOverride`). There is no retry loop.

### Scenario B: Concurrent Execution Collision
**Attack:** Two separate OptimizationPolicies generate two different Proposals for the exact same Deployment. Both reach the `Scheduled` state simultaneously. The Reconciler wakes up and executes both at the same millisecond, corrupting the workload state.
**Defense:** *Pass.* The optimistic `Execution Locking Strategy` using the `reconcilyx.io/active-proposal` annotation guarantees mutual exclusion. Only the first proposal to acquire the lock via SSA can proceed. The second proposal will see the lock belongs to someone else and abort.

### Scenario C: Unintended Field Destruction
**Attack:** A Reconciler bug causes it to submit an SSA patch that accidentally omits the `env` block or the `image` field. Because SSA can be declarative, Kubernetes deletes the `env` variables, taking down the application.
**Defense:** *Pass.* SSA explicitly maps ownership field-by-field. Because Reconcilyx's patch payload *only* includes the `resources` map (and `replicas`), Kubernetes assigns `fieldManager: reconcilyx-optimizer` exclusively to those fields. The `env` block remains owned by `kubectl` or `argocd` and is safely ignored by the patch.

### Scenario D: The Orphaned Rollback
**Attack:** Reconcilyx scales CPU down. The application begins crashing (OOMKilled). Before Reconcilyx can trigger an automatic rollback, a human operator panic-edits the Deployment to change the image to a stable tag, but leaves the CPU limits untouched. Reconcilyx then triggers its Rollback. Does the Rollback erase the human's image fix?
**Defense:** *Pass.* Because Reconcilyx uses SSA for its rollback (submitting only the `resources` map from its `ExecutionSnapshot`), it exclusively restores the CPU limits. The human's updated image tag is preserved perfectly.

---

## 2. Strengths
- **Field-Level Granularity:** SSA is the only safe way to mutate workloads automatically. JSON Patch is too brittle.
- **Yield-First Bias:** The explicit rule to yield to humans/GitOps in the event of conflict (`Force: false`) ensures the operator never goes rogue.
- **Mutual Exclusion:** The execution locking annotation solves the highly complex multi-proposal race condition elegantly.

## 3. Weaknesses & Remaining Risks
- **Rollback `Force: true` Hazard:** The strategy permits `Force: true` during a Rollback. While this guarantees the rollback succeeds, it will forcefully overwrite a human operator if they *specifically* modified the `resources` block during the incident. This is a calculated risk to prioritize stabilization over human drift.
- **Lock Leaking:** If the Reconciler crashes and never transitions a proposal to a terminal state, the `reconcilyx.io/active-proposal` annotation might become stuck on the workload, permanently blocking future optimizations. 

## 4. Required Mitigations (Technical Debt)
1. **Lock Garbage Collection:** A future enhancement must implement a TTL on the `active-proposal` lock annotation. If the annotation is older than 1 hour, the controller should forcefully break the lock.
2. **Metrics Emission:** Every time Reconcilyx yields to GitOps, it must increment a Prometheus metric `reconcilyx_gitops_yields_total`. If an operator sees this spiking, they know Reconcilyx and ArgoCD are misaligned.

---

## Final Recommendation: GO

The Mutation Strategy represents a highly defensive, production-grade approach to cluster actuation. By tightly coupling Server-Side Apply with a strict whitelist of mutable fields and an optimistic locking mechanism, the blast radius is mathematically constrained. 

**Implementation of Task #6 Phase 3 (Actuation) is officially justified.**
