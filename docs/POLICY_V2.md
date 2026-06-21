# Policy Engine V2 (Hardened Governance)

The Policy Engine has been hardened to meet production-grade safety guarantees, ensuring that autonomous optimizations (AutoApply) cannot execute without strict boundaries.

## 1. Safety Governance
The `SafetyConstraints` now explicitly require defined bounds rather than hard-coded magic numbers:
- `RiskTolerance`: Rejects proposals if `RiskScore` exceeds this value.
- `ConfidenceRequirement`: Rejects proposals if `ConfidenceScore` is below this value.

## 2. Recommendation Governance
To prevent destructive operations, policies must explicitly declare permitted actions via `AllowedRecommendations`.
- If `ScaleToZero` is not listed, the engine will strictly block any zero-scaling proposals.
- If the list is empty, the policy defaults to safe-block, rejecting all optimizations.

## 3. Cooldown Enforcement
Oscillating optimizations (e.g. upscaling followed by immediate downscaling) are prevented by the `CooldownPeriod`.
- The `Evaluator` calculates the duration since the last optimization.
- If the cooldown has not elapsed, the proposal is strictly rejected.

## 4. Sidecar Protection (Workload Protection)
To prevent the engine from blindly optimizing and starving critical infrastructure sidecars (like `istio-proxy` or `linkerd-proxy`), `WorkloadProtection` allows operators to specify `ExcludedContainers`.
- The Evaluator inspects the live workload.
- If an excluded container is found in the target pod spec, the proposal is rejected.

## 5. Deterministic Matching
When multiple policies apply to the same workload, the `Matcher` selects the policy based on:
1. **Specificity**: Workload > Namespace > Global
2. **Mode Precedence**: Observe > Propose > AutoApply
3. **Alphabetical Tie-breaker**: Name ascending (guarantees absolute determinism)

The Engine is now ready to safely govern Controller Actuation.
