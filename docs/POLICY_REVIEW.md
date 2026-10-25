# Policy Engine Review

Last reviewed: 2026-10-25. Scope: `internal/policy`, the controller evaluation
path, the JIT policy guard, and the approval API.

## Verdict

**GO for beta.** The policy engine is the safety boundary between analyzer
recommendations and cluster mutation. It is deterministic, fail-closed for
unknown recommendations, and every decision now carries structured evidence.
Production readiness is still gated by the live validations tracked in
[STATUS.md](../STATUS.md) and [FEATURE_MATRIX.md](../FEATURE_MATRIX.md), not by
the policy package itself.

## Evaluation Model

| Policy mode | Approval state | Proposal lifecycle |
|---|---|---|
| `Observe` | `Pending` | Held for review when constraints pass. |
| `Propose` | `Pending` | Held for review when constraints pass. |
| `AutoApply` | `Approved` | Scheduled and executed, subject to the JIT gate. |
| `DryRun` | `DryRun` | Terminal. `status.dryRun.wouldBe` records what `spec.dryRunMode` would have decided. Actuation never starts, and the approval API rejects both approve and reject requests with HTTP 409. |

`spec.dryRunMode` accepts `Observe`, `Propose`, or `AutoApply` and defaults to
`Propose`. Constraint violations under a dry-run policy are reported as
`status.dryRun.wouldBe: Rejected` instead of transitioning the proposal to
`Rejected`; a dry run must not change the lifecycle even when the
hypothetical outcome is a rejection.

The evaluator runs five constraints in a fixed order and then the mode
decision:

| Constraint | Behavior |
|---|---|
| `ResourceBoundaries` | Applies `minCPU`/`maxCPU`/`minMemory`/`maxMemory` to aggregate and per-container **requests and limits**. Per-container entries are evaluated in sorted order for deterministic messages. |
| `SafetyScores` | Rejects `riskScore > riskTolerance` (default 50) and `confidenceScore < confidenceRequirement` (default 80). |
| `RecommendationGovernance` | Rejects any type not explicitly listed; an empty `allowedRecommendations` rejects everything. |
| `CooldownEnforcement` | Rejects when the newest execution for the same workload is younger than `cooldownPeriod` (default 15m). The controller supplies the timestamp from `status.executionSnapshot.timestamp`; the JIT gate repeats the check at execution time. |
| `WorkloadProtection` | Rejects the workload when any live container is in `excludedContainers`. Container names come from the Deployment, StatefulSet, or DaemonSet pod template. |

## Structured Evidence

Every evaluation writes one `status.decisionEvidence` entry per constraint plus
a `PolicyMode` entry:

```yaml
decisionEvidence:
  - constraint: ResourceBoundaries
    allowed: true
    rationale: ""
  - constraint: WorkloadProtection
    allowed: false
    rationale: "Workload contains protected sidecar/container: 'istio-proxy'"
  - constraint: PolicyMode
    allowed: true
    rationale: "mode AutoApply produced Approved"
```

`status.reason` remains the human-readable summary. The evidence list is the
machine-readable record: consumers no longer need substring matching to learn
which check failed. A proposal with no matching policy records a
`PolicyMatch` / `allowed: false` entry.

## Matching and Precedence

Policies are matched with `metav1.LabelSelectorAsSelector`, so both
`matchLabels` and set-based `matchExpressions` (`In`, `NotIn`, `Exists`,
`DoesNotExist`) are honored.

Precedence is deterministic:

1. **Selector specificity** — workload selector +10, namespace selector +5.
2. **Namespace-local affinity** — a policy created in the target workload's
   namespace scores +6. This is the namespace override: a local default beats
   a foreign or cluster-wide policy of equal specificity. An explicit workload
   selector on a global policy (10) still outranks a local no-selector default
   (6).
3. **Conservative mode precedence** — `Observe` > `Propose` > `DryRun` >
   `AutoApply`.
4. **Fully qualified name** — `namespace/name` lexicographic order, so two
   same-named policies in different namespaces no longer depend on slice
   order.

Namespace inheritance is implemented as precedence, not field merging: a
namespace-local policy selects the behavior for that namespace; there is no
deep merge of constraint fields across policies. That is intentional and is
called out as a limitation below.

## Policy Index

`policy.Index` narrows the candidate set before specificity scoring. It buckets
policies under each `namespaceSelector.matchLabels` `key=value` pair, keeps
global policies and expression-only selectors in separate buckets, and verifies
both selectors against live labels before scoring. Invalid selectors are
skipped rather than failing a reconciliation.

The controller refreshes the index:

- lazily on the first evaluation (`ensurePolicyIndex`), and
- after every `OptimizationPolicy` create/update/delete through a
  `Watches(...)` mapping (`mapPolicyChangeToProposals`).

The map function also re-queues proposals still in `Detected` (or with no
phase), so a newly created policy is applied without waiting for a resync.
Already-evaluated proposals are not silently re-decided. If an index refresh
fails, matching falls back to a live `List` with the linear matcher.

## Fail-Closed Behavior Outside the Evaluator

- A scheduled proposal with no recorded policy, or whose policy was deleted,
  fails before execution.
- The JIT gate re-checks cooldown, change budget, and blast radius, so a stale
  evaluation cannot bypass them.
- A multi-container proposal without `proposedResourcesByContainer` fails
  closed before any pod is resized.
- DaemonSets fail closed when a blast-radius replica cap is configured.

## Remaining Limitations

1. **Namespace inheritance is precedence-only.** There is no merge of
   constraint fields from a global base policy into a namespace-local one; the
   selected policy is used whole.
2. **Index refresh is event-driven, not a shared informer.** Each policy
   event performs a `List`; this is cheap at beta policy counts but is not the
   same as a long-lived indexed informer.
3. **Dry run is evaluation-only.** It does not simulate telemetry, JIT gates,
   or a resize against the live API server.
4. **`rollbackConstraints` fields are not wired.** The local rollback manager
   rolls back on any OOM reported by telemetry and does not honor
   `oomKilledWindow` or `errorRateThresholdPercent`. The spoke agent has a
   separate OOM check. Do not treat these policy fields as effective
   guardrails yet.
5. **No PDB or disruption-budget constraint.** Availability protection is
   limited to replica checks in the JIT gate and workload health probes.
6. **Pod-local limits are not bounded by the namespace `LimitRange`.** The
   policy engine compares against policy fields only.

## Verification (2026-10-25)

| Check | Result |
|---|---|
| `go build ./...` | Passed. |
| `go test ./internal/policy` | Passed, including evidence, dry-run, limit bounds, namespace precedence, FQN tie-break, and index refresh. |
| `go test ./internal/controller` (envtest) | `Ran 13 of 13 Specs` — `13 Passed | 0 Failed`, including the new dry-run lifecycle spec. |
| `go test ./internal/api -run TestDryRun` | Passed; both approve and reject return HTTP 409 and leave the dry-run status unchanged. |
| `go test ./internal/api` | Passed. |
| `make generate manifests` | Regenerated deepcopy and CRD schemas for `dryRunMode`, `mode: DryRun`, `decisionEvidence`, and `dryRun`. |

The last recorded policy/gate live evidence (2026-10-24) is in
[POLICY_AUDIT.md](POLICY_AUDIT.md): selector matching against live labels,
cooldown rejection at evaluation and JIT time, audit records, per-container
resize, and the scripted harness run `20261024155951` with 66 checks and 0
failures.
