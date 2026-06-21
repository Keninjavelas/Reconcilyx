# Reconciler Risk Review & Independent Assessment

## Executive Summary
This document serves as an independent, adversarial risk assessment of the Phase 1 Reconciler Design. As a Staff Platform Engineer, my objective is to break this design before it touches a production cluster.

**Recommendation: GO WITH CONDITIONS**

The decision to use Server-Side Apply (SSA) is a massive win for declarative stability, and the state machine correctly isolates the engine from the execution. However, there are significant race conditions and stale state hazards that must be remediated before implementation can begin.

---

## 1. Strengths
- **Server-Side Apply (SSA):** Correctly mitigates the notorious index-shifting bugs associated with JSON Patch.
- **Strict State Machine:** The terminal states (`Completed`, `Failed`, `RolledBack`) and forbidden transitions explicitly prevent infinite looping or "zombie" reconciliations.
- **Explicit Ownership:** The `ExecutionPlan` structure correctly captures the `BeforeState` required for a safe rollback.

---

## 2. Weaknesses & Critical Risks

### [CRITICAL RISK 1] Stale Telemetry vs. Execution Delay (The "Ghost Optimization" Hazard)
**Scenario:** 
1. Engine detects low usage and generates a RightSize proposal at T=0.
2. Proposal enters the `Scheduled` queue due to policy Cooldown, waiting 15 minutes.
3. At T=14, a massive traffic spike hits the workload. HPA scales it up.
4. At T=15, the Reconciler wakes up and executes the T=0 `ProposedResources` (downsizing the pod).
5. Result: Immediate OOMKills and production outage.
**Mitigation Required:** The Reconciler MUST re-fetch live telemetry (or require the Engine to re-certify) immediately before transitioning from `Scheduled` -> `Executing`. The 1-hour stale proposal timeout is too long.

### [CRITICAL RISK 2] Multi-Reconcile Collision
**Scenario:**
Multiple controllers run simultaneously. `OptimizationProposalReconciler` triggers a rollback via SSA, but an external CD pipeline (e.g., ArgoCD) immediately reverts the rollback because it detects drift. 
**Mitigation Required:** Reconcilyx must update an annotation on the target workload (e.g., `reconcilyx.io/last-action: rollback`) so that external systems or human operators can audit *why* drift occurred. We cannot rely solely on the Proposal's status field since it is disconnected from the workload object.

### [CRITICAL RISK 3] Unbounded Stabilization Loop
**Scenario:**
The Reconciler enters `Stabilizing` state and waits for 15 minutes. During this time, the target workload is deleted by a user. The Reconciler keeps querying Prometheus for the deleted workload, resulting in empty metrics, which it interprets as "no errors." The proposal falsely marks itself as `Completed`.
**Mitigation Required:** The stabilization loop must continuously verify the existence and `Ready` state of the target workload before passing the stabilization check.

### [CRITICAL RISK 4] Field Manager Collision
**Scenario:**
We use SSA with `fieldManager: reconcilyx-optimizer`. If the user has explicitly defined limits in their GitOps manifest, Reconcilyx applying limits will result in shared ownership or conflicts depending on how `force` is applied. 
**Mitigation Required:** The controller must use SSA with `Force: true` to seize ownership of the `resources` map, but MUST emit a specific Warning event so operators know Reconcilyx seized control from GitOps.

---

## 3. Required Safeguards (Conditions for GO)

Before beginning implementation of the Primary Reconcilers, the design must incorporate the following safeguards:

1. **Just-In-Time (JIT) Telemetry Validation:** Before executing a mutation, the Reconciler must perform a fast, JIT telemetry check. If current resource usage exceeds the `ProposedResources` by >80%, the execution must abort and transition to `Failed` (Reason: `StaleProposalHazard`).
2. **Workload Annotation Tracking:** Execution and Rollback patches must include adding an annotation (`reconcilyx.io/managed-by: true` and `reconcilyx.io/last-action: ...`) directly on the target workload to alert external GitOps controllers.
3. **Liveness Assertion in Stabilization:** The `Stabilizing` phase must fail immediately if the target workload is deleted or drops to 0 ready replicas.
4. **SSA Force Application:** Explicitly document and implement `Force: true` in the SSA patch client, accompanied by mandatory Event generation.

---

## Final Recommendation: GO WITH CONDITIONS

The core design is solid, but the asynchronous gap between Proposal Generation and Actuation creates a severe hazard for highly volatile workloads. Once the **JIT Telemetry Validation** and **Liveness Assertions** are formally added to the architecture plan, implementation of Task #6 may begin.
