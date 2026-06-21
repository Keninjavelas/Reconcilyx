# Policy Design (Phase 1)

## 1. Policy Responsibilities
The Policy Engine strictly enforces governance over the optimizations proposed by the Engine.

**What policy owns:**
- Evaluating the safety of an `OptimizationProposal`.
- Enforcing resource boundaries (`MinCPU`, `MaxMemory`).
- Rejecting high-risk optimizations based on Engine scores.
- Determining if an optimization requires human review (`ModeObserve`, `ModePropose`) or can be automated (`ModeAutoApply`).

**What policy must never own:**
- Calculating resource usage or suggesting limits.
- Patching actual workloads.

## 2. Evaluation Pipeline
1. **Proposal Input**: The engine submits an `OptimizationProposal` and a target `corev1.ObjectReference`.
2. **Policy Selection**: `Matcher` selects the most applicable `OptimizationPolicy` from the cluster.
3. **Constraint Evaluation**: The `Evaluator` runs Resource and Safety constraints.
4. **Approval Decision**: If constraints pass, the policy `Mode` dictates the final state.
5. **Execution Eligibility**: The `OptimizationProposal.Status` is updated with `ApprovalState` and `DecisionEvidence`.

## 3. Policy Matching
The `Matcher` determines which policy applies to a workload.
- **Specificity Precedence**: A policy matching a workload label > namespace label > global policy.
- **Conflict Resolution**: If two policies tie, the most conservative mode wins (`Observe` > `Propose` > `AutoApply`).

## 4. Constraint Framework
- **Resource Constraints**: Reject proposals where requested resources fall below `MinCPU`/`MinMemory` or above `MaxCPU`/`MaxMemory`.
- **Safety Constraints**: Reject proposals where the Engine's `ConfidenceScore` < 80 or `RiskScore` > 50.

## 5. Decision Model
- **Rejected**: Constraints violated. Execution is blocked.
- **RequiresReview**: Constraints passed, but Policy is `Observe` or `Propose`. Execution is paused until a human changes status to `Approved`.
- **Approved**: Constraints passed, Policy is `AutoApply`. Controller may actuate.

## 6. Auditability
Every evaluation produces a `DecisionEvidence` struct added to the `OptimizationProposal` status. It explicitly logs the matched policy name and a human-readable string explaining the rejection or approval reasoning.
