# Reconciler Design & Architecture

## 1. Reconciler Responsibilities

The Reconciliation subsystem is the execution layer of Reconcilyx. It strictly enforces the principle: **Reconcilers execute, they never invent.**

### OptimizationProposalReconciler
**Responsibilities:**
- Watches `OptimizationProposal` objects.
- Orchestrates the proposal lifecycle (Evaluation -> Execution -> Stabilization -> Finalization).
- Enacts K8s mutations on the target workload (`Deployment`, `StatefulSet`, etc.) when a proposal is Approved and ready for execution.
- Observes the stabilization period post-execution.
- Executes automated rollbacks if stabilization fails (e.g., elevated error rates, OOMKills, or latency spikes).
- Records comprehensive Kubernetes Events and updates the `Status` subresource.

**Boundaries:**
- Must NEVER alter the `Recommendation`, `ProposedResources`, or `DecisionMetadata` of a proposal.
- Must NEVER execute a proposal whose `ApprovalState` is not explicitly `Approved`.
- Must ONLY mutate the target workload specified in the proposal.

### OptimizationPolicyReconciler
**Responsibilities:**
- Watches `OptimizationPolicy` objects.
- Validates the policy syntax and structural integrity.
- Updates the policy `Status` with active targets and compliance metrics.
- Surfaces misconfigurations via Kubernetes Events.

**Boundaries:**
- Must NEVER mutate application workloads.
- Must NEVER mutate `OptimizationProposals` (it only configures the rules by which they are evaluated).

---

## 2. Proposal Lifecycle

The `OptimizationProposal` progresses through a strict state machine represented by its `Status.ApprovalState` and `Status.ExecutionState`.

**State Machine:**
- `Detected`: The proposal is newly created by the Engine.
- `Evaluating`: The `OptimizationProposalReconciler` triggers the Policy Engine to evaluate the proposal.
- `Approved`: The Policy Engine validates the proposal safely conforms to all constraints.
- `Rejected`: The Policy Engine denies the proposal (Terminal State).
- `Scheduled`: The proposal is approved and enters the queue to respect the `CooldownPeriod`.
- `Executing`: The Reconciler applies the mutation to the Kubernetes API.
- `Stabilizing`: The Reconciler waits for a defined period (e.g., 5-15m) to verify the workload remains healthy.
- `Completed`: The stabilization period passes without violating `RollbackConstraints`. (Terminal State).
- `Failed`: The execution fails due to K8s API errors or unrecoverable timeouts. (Terminal State).
- `RolledBack`: The workload violated rollback constraints during stabilization and was restored to its original state. (Terminal State).

**Forbidden Transitions:**
- `Approved` -> `Evaluating` (Once decided, re-evaluation requires a new proposal).
- `Completed` -> `Executing` (Terminal states are immutable).
- `Rejected` -> `Approved` (Requires a new proposal from the Engine).

---

## 3. Execution Model

When a proposal transitions to `Executing`, the Reconciler generates an `ExecutionPlan`.

**ExecutionPlan Schema (Internal):**
```go
type ExecutionPlan struct {
    TargetWorkload corev1.ObjectReference
    TargetContainer string
    BeforeState    corev1.ResourceRequirements
    DesiredState   corev1.ResourceRequirements
    RollbackState  corev1.ResourceRequirements // Identical to BeforeState initially
    ExecutionMetadata struct {
        FieldManager string // e.g., "reconcilyx-actuator"
        Timestamp    time.Time
    }
}
```

---

## 4. Mutation Strategy

**Selected Strategy: Server-Side Apply (SSA)**

**Rationale:**
- **Superior Conflict Resolution:** SSA allows Reconcilyx to claim ownership of *only* the `resources` map for the target container using a specific `fieldManager` (e.g., `reconcilyx-optimizer`).
- **Declarative:** We submit exactly what we want the resource section to look like, avoiding index out-of-bounds errors common in JSON Patch when container orders change.
- **Drift Detection:** SSA natively handles cases where another controller (e.g., VPA) or human overwrites our changes.
- **Deprecation:** Strategic Merge Patch is effectively deprecated in modern Kubernetes tooling.

**Rollback Implications:**
Rolling back is simple and deterministic: we re-submit a Server-Side Apply payload containing the `RollbackState` using the exact same `fieldManager`.

---

## 5. Rollback Design

Rollback is a first-class citizen designed to trigger automatically during the `Stabilizing` phase.

**Mechanism:**
- The Reconciler maintains a copy of the `BeforeState` in the `OptimizationProposalStatus.ObservedResources.CurrentResources`.
- During `Stabilizing`, the Reconciler periodically queries Prometheus for metrics violating the `RollbackConstraints` (e.g., `OOMKilledWindow`, `ErrorRateThresholdPercent`).
- If a violation is detected, an SSA payload is generated using the `BeforeState`.
- The `ExecutionState` transitions to `RolledBack`.

**Evidence Tracking:**
- The `Status.Conditions` array receives a new condition: `Type: RolledBack, Status: True, Reason: OOMSpike, Message: "OOM kills exceeded threshold of 5 in 10m window"`.

---

## 6. Eventing & Auditability

Every significant transition produces a Kubernetes Event on the `OptimizationProposal` AND the target workload.

**Event Taxonomy:**
- `Normal EvaluationTriggered`: "Policy evaluation started."
- `Normal ProposalApproved`: "Policy Engine approved proposal (Risk: 30, Confidence: 90)."
- `Warning ProposalRejected`: "Policy Engine rejected proposal. Reason: Risk exceeds tolerance."
- `Normal ExecutionStarted`: "Applying Server-Side Apply patch to Deployment/frontend."
- `Normal StabilizationStarted`: "Entering 10m stabilization window."
- `Warning RollbackTriggered`: "Rollback initiated. Reason: Error rate exceeded 5%."
- `Normal RollbackCompleted`: "Workload successfully restored to original resources."
- `Normal OptimizationCompleted`: "Optimization successfully verified."

---

## 7. Failure Handling

- **Workload Deleted:** If the `TargetWorkload` cannot be found, transition to `Failed` with reason `TargetNotFound`.
- **Deployment Changed Mid-Execution:** SSA handles this gracefully. If the pod spec was completely rewritten, Reconcilyx simply claims the new `resources` fields.
- **Stale Proposal:** If `time.Since(proposal.CreationTimestamp) > 1h`, reject execution and transition to `Failed` with reason `ProposalStale`.
- **Policy Changed During Execution:** Once in `Executing`, policy changes do not interrupt the current plan. The stabilization and rollback constraints are snapshotted.
- **Rollout Failure:** If the ReplicaSet fails to progress (K8s `ProgressDeadlineExceeded`), trigger Rollback immediately.

---

## 8. Concurrency Model

**Idempotency & Optimistic Locking:**
- The Reconciler relies entirely on `Status` conditions. If `ExecutionState == Executing`, subsequent reconcile loops will check K8s API state rather than re-patching blindly.
- Kubernetes native `ResourceVersion` checks prevent race conditions when updating the `OptimizationProposalStatus`.

**Duplicate Execution Prevention:**
- Once a proposal reaches `Completed`, `Failed`, or `RolledBack`, the controller ignores it.
- A finalizer (`reconcilyx.io/proposal-protection`) is added to ensure proposals aren't deleted mid-execution.

---

## 9. Testing Strategy

1. **Controller Tests (envtest):**
   - Spin up a real local APIServer/etcd using Kubebuilder `envtest`.
   - Apply an `OptimizationProposal`.
   - Verify the controller successfully patches a dummy `Deployment`.
   - Verify `Status` updates correctly.
2. **Rollback Tests:**
   - Simulate an execution.
   - Inject a mock telemetry response indicating high error rates.
   - Verify the Reconciler patches the dummy `Deployment` back to its original state.
3. **Failure Tests:**
   - Delete the target `Deployment` mid-execution and verify `ExecutionFailed` state.
