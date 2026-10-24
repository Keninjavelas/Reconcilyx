# Policy Engine Audit — Current Status

Last reviewed: 2026-10-24. Scope: `internal/policy` plus the controller and JIT
gate paths that consume its decisions.

## Executive Summary

The original audit (2026-07) blocked controller actuation on seven missing
policy controls. All seven are now implemented and covered by tests. The
remaining findings are structural improvements rather than safety blockers, so
the policy engine is suitable for beta use; production readiness still depends
on the end-to-end validations tracked in [STATUS.md](../STATUS.md) and
[FEATURE_MATRIX.md](../FEATURE_MATRIX.md).

## Resolved Findings

| Original finding | Current implementation |
|---|---|
| Hard-coded risk/confidence thresholds | `SafetyConstraint` reads `spec.safetyConstraints.riskTolerance` and `confidenceRequirement`; defaults remain 50/80 when unset (`internal/policy/constraints.go`). |
| `AllowedRecommendations` missing | `RecommendationConstraint` rejects any recommendation not in the list, and an empty list rejects everything (fail closed). |
| `ExcludedContainers` missing | `WorkloadProtectionConstraint` rejects a workload when a protected container is present. `workloadContainerNames` now supplies the live container list from Deployment, StatefulSet, or DaemonSet pod templates. |
| `CooldownPeriod` ignored | `CooldownConstraint` receives the newest execution timestamp for the same workload (`lastExecutionForWorkload`), and the JIT gate repeats the check in `checkCooldown` so a stale evaluation cannot bypass a cooldown. The CRD defaults the field to 15m. |
| Non-deterministic policy matching | `Matcher` sorts by specificity, then conservative mode precedence (Observe > Propose > AutoApply), then policy name. |
| No allowed-action governance | Covered by `RecommendationConstraint`; `ScaleToZero` must be explicitly whitelisted. |
| Cooldown cannot see cluster history | The controller now lists executed proposals and passes the latest `status.executionSnapshot.timestamp` into the evaluator. |

Additional guardrails added after the original audit:

- `spec.changeBudget` enforces a rolling per-workload change limit in the JIT gate (`maxChangesPerWindow`, default 24h window).
- `spec.blastRadius` enforces a replica cap and optional canary-label requirement before actuation; replica caps fail closed for DaemonSets.
- `spec.proposedResourcesByContainer` scopes actuation and GitOps manifest patching to the named containers; a multi-container proposal without it fails closed before any pod is resized.
- A scheduled proposal with no recorded policy, or whose policy no longer exists, fails instead of executing unguarded.
- Policy matching now receives live labels. `workloadMatchLabels` resolves the target `Namespace` and the Deployment, StatefulSet, or DaemonSet labels before `Matcher.Match` runs; previously the controller passed empty maps, so any policy with a namespace or workload selector could never match. Missing targets yield empty labels and are rejected by matching rather than blocking reconciliation. The manager also has `namespaces` get/list/watch RBAC, without which reconciliation logged `namespaces is forbidden`.
- Policy evaluations and execution decisions are appended to the `reconcilyx-audit` ConfigMap in the proposal namespace (newline-delimited JSON, capped at the 500 newest entries).

## Remaining Gaps

These do not block beta actuation but should be addressed before a stable
release:

1. **Unstructured evidence.** Rejections are persisted as a joined string in
   `status.reason`. A `[]DecisionEvidence` field would make gate decisions
   machine-readable instead of relying on substring matching.
2. **No dry-run.** Evaluation always lands on `Approved`, `Rejected`, or
   `Pending`; there is no mode that records what would have happened without
   transitioning the proposal.
3. **Brute-force matching.** Every evaluation lists and scores all policies.
   An informer-backed index would matter at large policy counts, not at beta
   scale.
4. **Requests only.** `ResourceConstraint` evaluates requests; it does not
   apply min/max bounds to limits.
5. **Namespace inheritance.** Policies do not natively inherit or override
   per namespace; specificity plus name ordering is the only precedence.
6. **Name collisions.** Two policies with the same name in different
   namespaces and equal specificity still tie on name, leaving slice order as
   the final discriminator.

## Verification

- `go test ./internal/policy` covers thresholds, recommendation governance,
  cooldown, exclusions, and matching.
- `internal/controller/policy_context_test.go` covers execution lookup,
  namespace fallback, container discovery, selector label resolution, and
  end-to-end evaluator wiring for cooldown and excluded containers.
- `internal/controller/policy_guard_test.go` covers cooldown, change budget,
  replica caps, canary scoping, and DaemonSet fail-closed behavior.
- `internal/controller/audit_test.go` covers audit append, JSON shape, and the
  500-entry cap.
- `internal/controller/actuator_test.go` covers live-pod snapshots, missing and
  divergent pod refusal, per-container selection, and complete resize patches.
- The controller envtest suite reported `Ran 12 of 12 Specs` and all passing on
  2026-10-24, including the selector-scoped policy match; the full non-e2e
  suite exited 0 after the snapshot and SSA fixes the same day.

## Live Validation (2026-10-24)

The policy and gate paths were exercised against a Kind cluster (`reconcilyx-live`, node `v1.36.1`) with the manager installed from the Helm chart:

- `WorkloadProtectionConstraint` rejected `storefront-exclusion` with `[WorkloadProtection]: Workload contains protected sidecar/container: 'istio-proxy'`.
- `CooldownConstraint` rejected `storefront-cooldown` with `[CooldownEnforcement]: Cooldown active: Only 18s elapsed since last optimization, requires 30m0s`. A later approved proposal was independently rejected by the JIT gate with `cooldown active: 20s since the last change to live-demo/storefront, requires 30m0s`, confirming the execution-time recheck.
- The `reconcilyx-audit` ConfigMap in the proposal namespace contained newline-delimited `policy-evaluation` and `execution-attempt` records for allowed and applied decisions.
- A multi-container proposal without `proposedResourcesByContainer` failed with the fail-closed reason and left both pods unchanged; a per-container proposal resized only `app` and preserved `istio-proxy` at its original requests.
- `kube-prometheus-stack` `69.4.1` was installed in `monitoring`; kubelet and kube-state-metrics reported `up=1`, and `live-demo` exposed 24 CPU-usage series and 8 limit series.
- `checkout-learning-1` passed the full five-minute stabilization window and completed at 13:39:00Z with a finalized receipt. Its template-based snapshot revealed the savings defect (`23269m` reported versus `4106m` for the live delta); after `CaptureSnapshot` was corrected, `storefront-learning-2` completed at 14:23:31Z with a live `250m/192Mi` snapshot, `app` resized to `200m/160Mi`, the operator-owned `istio-proxy` preserved at `250m/192Mi`, `costSavedUSD=4106m`, and `learning.finalized=true`.
- The scripted harness (`make validate-e2e`) reproduced the full lifecycle in an isolated namespace: a selector-scoped policy matched using the live namespace and workload labels, the audit log recorded `policy-evaluation`, `allowed: JIT validation passed`, `applied: in-place pod resize`, and `completed: workload stabilized`, and 66 checks passed with 0 failures on run `20261024155951`.

Not exercised on the live cluster: a production-like multi-node cluster, a real GitHub/GitLab PR/MR (no provider token), and TLS termination. These remain tracked in [GITHUB_ISSUES.md](../GITHUB_ISSUES.md).
