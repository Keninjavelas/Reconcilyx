# Reconcilyx Policy

The `policy` package evaluates `OptimizationProposal` objects against
`OptimizationPolicy` rules and selects which policy governs a workload.

## Components

- `Matcher` — linear selector matcher with deterministic precedence.
- `Index` — informer-fed candidate index used by the controller; falls back to
  the linear matcher when unavailable.
- `Evaluator` — runs the constraint chain and returns a decision, per-check
  evidence, and the hypothetical result for dry-run policies.

## Constraints

`ResourceBoundaries`, `SafetyScores`, `RecommendationGovernance`,
`CooldownEnforcement`, and `WorkloadProtection`. Every constraint is
fail-closed: an empty recommendation allow-list rejects everything, and a
missing bound disables only that bound.

## Precedence

1. Selector specificity (workload +10, namespace +5).
2. Namespace-local affinity (+6), giving a local policy override over a
   foreign policy of equal specificity.
3. Conservative mode precedence: `Observe` > `Propose` > `DryRun` >
   `AutoApply`.
4. Fully qualified `namespace/name` ordering.

## Modes

`Observe` and `Propose` hold for review; `AutoApply` approves; `DryRun`
evaluates and records `status.dryRun.wouldBe` without advancing the proposal.
Dry-run proposals cannot be approved or rejected.

See [docs/POLICY_REVIEW.md](../../docs/POLICY_REVIEW.md) for the current review,
remaining limitations, and verification evidence.
