# Policy Engine Audit & Readiness Review

## Executive Summary
This document serves as an adversarial, independent platform engineering review of the `internal/policy` package (Task #5). The objective is to verify that the Policy Engine can mathematically and operationally prevent unsafe workload mutations. 

**Recommendation: GO WITH CONDITIONS**

The core structural abstractions are sound, but the MVP implementation is dangerously incomplete. It relies on hard-coded safety thresholds, completely ignores critical operational constraints (cooldowns), and has non-deterministic policy matching tie-breakers. The system cannot be trusted to mutate cluster state until the blocking remediation conditions are met.

---

## Architecture Assessment
**Grade: B**

**Strengths:**
- The Strategy Pattern (`Matcher`, `Constraint`, `Evaluator`) is well-defined and decoupling is strictly enforced.
- Dependency injection allows for clean, offline testing.
- The pipeline correctly prevents the Policy Engine from performing K8s API mutation operations.

**Weaknesses:**
- **Evidence Formatting:** The evidence is returned as a concatenated string (`strings.Join(rejections, " | ")`). This is human-readable but not machine-parseable, breaking the API standard for structured K8s status fields.
- **Missing Telemetry Context:** The `Evaluator` only takes the `OptimizationProposal` and `OptimizationPolicy`. It lacks access to cluster state (e.g., historical deployment events) necessary to enforce cooldowns or stabilization periods.

---

## Matching Assessment
**Grade: C-**

**Strengths:**
- Correctly parses `metav1.LabelSelector` via standard `k8s.io/apimachinery` utilities.
- Specificity scoring (Workload > Namespace > Global) is a robust heuristic.

**Weaknesses (Critical):**
- **Non-Determinism:** If two policies match a workload with identical specificity scores *and* identical `PolicyMode` (e.g., both are `ModeObserve`), the tie-breaker is purely based on the arbitrary order of the `policies` slice. 
- **Missing Exclusion Selectors:** There is no way to explicitly exclude a specific workload or namespace using standard `MatchExpressions` (e.g., `NotIn`).

---

## Constraint Assessment
**Grade: D**

**Strengths:**
- `ResourceConstraint` correctly handles K8s resource quantities (`MinCPU`, `MaxCPU`).

**Weaknesses (Critical):**
- **Hardcoded Safety Bounds:** The `SafetyConstraint` rejects proposals based on hard-coded thresholds (`RiskScore > 50`, `ConfidenceScore < 80`). These must be configurable via the `OptimizationPolicy` CRD. What if a user wants to accept high-risk scaling in a dev environment?
- **Ignored CRD Fields:** The engine completely ignores the `CooldownPeriod`, `MinReplicas`, `OOMKilledWindow`, and `ErrorRateThresholdPercent` fields defined in `OptimizationPolicySpec`.
- **Allowed Actions Missing:** The constraint framework assumes all recommendation types (RightSize, ScaleToZero) are allowed if limits are met. ScaleToZero is inherently destructive and must be explicitly toggled per policy.

---

## Governance Assessment
**Grade: C**

**Strengths:**
- Effectively catches and forces human review for `Observe` and `Propose` modes.
- Rejection is absolute.

**Weaknesses (Critical):**
- **Unsafe Recommendations CAN slip through:** If a deployment has just been restarted due to a previous optimization, the engine will still propose an `AutoApply` change because the `CooldownPeriod` constraint is not implemented. This risks an infinite restart loop.
- **No Dry-Run Mode:** There is no mechanism to record what *would* have happened without updating the `OptimizationProposal` to `Approved`.

---

## API Assessment
**Grade: C+**

**Strengths:**
- Clean separation between `ResourceConstraints` and `SafetyConstraints`.

**Weaknesses (Critical):**
- **Missing Allowed Recommendations Field:** Policies need an `AllowedRecommendations []RecommendationType` field.
- **Missing Container Exclusions:** Policies need to exclude sidecars (`istio-proxy`, `linkerd-proxy`).
- **Missing Risk/Confidence Thresholds:** Policies must explicitly define their own Risk and Confidence tolerances, replacing the hard-coded values in `constraints.go`.
- **Proposal Status Schema:** `OptimizationProposalStatus` needs a structured `[]DecisionEvidence` field, not just a string block.

---

## Risk Register
1. **[HIGH] Restart Loops:** Ignored `CooldownPeriod` allows continuous optimizations on oscillating workloads.
2. **[HIGH] Accidental ScaleToZero:** Lack of `AllowedRecommendations` governance could cause the engine to turn off critical but dormant pods.
3. **[MED] Non-Deterministic Policy Evaluation:** Tie-breaker failure causes unpredictable policy application in overlapping namespaces.
4. **[MED] Sidecar Starvation:** Without container exclusions, service-mesh proxies could be improperly downscaled, severing network traffic.

---

## Technical Debt Register
1. `constraints.go` uses hardcoded magic numbers for Risk and Confidence.
2. `metav1.LabelSelectorAsSelector` is used correctly, but sorting is unstable due to slice-order dependency.
3. `evaluator.go` generates unparseable string evidence.

---

## Top 10 Improvements (To Elevate to Exceptional)
1. Implement historical event tracking to enforce `CooldownPeriod`.
2. Add `AllowedRecommendations` to the Policy CRD.
3. Add `ExcludedContainers` to the Policy CRD.
4. Move Risk/Confidence bounds into the Policy CRD.
5. Create a deterministic string-comparison tie-breaker (e.g., alphabetical by Policy Name) for identical precedence.
6. Refactor the `Evaluator` API to return a structured `[]DecisionEvidence` object.
7. Integrate a `DryRun` boolean into the policy API.
8. Support namespace inheritance/overrides natively in the registry.
9. Support evaluating `Limits` constraints, not just `Requests`.
10. Wire the `Matcher` to use an informer-backed cache instead of a brute-force loop over all policies.

---

## Readiness Recommendation: GO WITH CONDITIONS

We cannot proceed to Controller actuation (Task #6) until the following blocking conditions are met within the Policy Engine.

### Required Remediation (Blockers for Task #6)

1. **API Update:** Add `RiskTolerance` and `ConfidenceRequirement` to `SafetyConstraints` in `OptimizationPolicy`.
2. **API Update:** Add `AllowedRecommendations []RecommendationType` to `OptimizationPolicy`.
3. **API Update:** Add `ExcludedContainers []string` to `OptimizationPolicy`.
4. **Logic Update:** Refactor `SafetyConstraint.Evaluate` to use the new CRD fields instead of hard-coded values.
5. **Logic Update:** Implement a `RecommendationConstraint` to enforce `AllowedRecommendations` (blocking `ScaleToZero` if not explicitly allowed).
6. **Logic Update:** Refactor `matcher.go` to use an alphabetical tie-breaker (e.g., `matched[i].Name < matched[j].Name`) when specificity and mode tie.
7. **Logic Update:** Enforce `CooldownPeriod` constraint (even if using a stubbed historical clock for the MVP).

Once these 7 conditions are implemented and tested, Task #6 may begin.
