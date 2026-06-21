# Actuation Implementation (Phase 3A)

## Execution Flow

The `OptimizationProposalReconciler` implements the forward state machine using the Just-In-Time `ExecutionGate` and the Server-Side Apply `Actuator`. 

The flow is:
1. `Approved` → `Scheduled`: Proposals queue up waiting for the `ValidateJIT` check.
2. `ValidateJIT`: Verifies the global kill switch is disabled, the TTL is valid, the workload is healthy, and the recommendation is explicitly whitelisted for Phase 3A (`ReduceCPURequest` or `ReduceMemoryRequest`).
3. `Executing`: 
   - **Acquire Lock:** Optimistically acquires `reconcilyx.io/execution-lock` on the target deployment. If another proposal holds it, it requeues.
   - **Snapshot Capture:** DeepCopies the exact container resources, generation, and annotations to `ExecutionSnapshot`.
   - **Server-Side Apply:** Constructs an unstructured SSA payload mapping strictly to `spec.template.spec.containers[0].resources.requests`.
4. `Verifying`: Assumes the payload was accepted by the apiserver.
5. `Stabilizing`: Assumes the deployment is rolling out without crashing.
6. `Completed`: Terminal success state. The lock is released.

## Field Ownership

Reconcilyx uses Server-Side Apply with `FieldManager: reconcilyx-optimizer`.
- `Force` is set to `false`.
- The unstructured payload is strictly limited. It completely omits the `limits` sub-resource, the `env` array, and `replicas`.
- Kubernetes mathematically guarantees that Reconcilyx will only become the owner of the `requests.cpu` and `requests.memory` fields. Any other field modification attempt will be completely ignored or rejected.

## Lock Model

- **Key:** `reconcilyx.io/execution-lock` (Annotation on the target Workload)
- **Value:** `OptimizationProposal.Name`
- **Behavior:** Mutex locking to ensure only 1 proposal can modify a workload at a time. Released upon `PhaseCompleted` or `PhaseFailed`.

## Failure Handling

- If JIT validation fails, the proposal jumps to `Failed` or `Expired`.
- If SSA fails (e.g., due to a GitOps ownership conflict from another FieldManager), the proposal transitions to `Failed: Server-Side Apply Failed` and releases the lock.
- If the target workload is deleted mid-execution, the API errors handle it safely.
