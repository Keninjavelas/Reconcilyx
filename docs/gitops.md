# GitOps Integration

Reconcilyx supports fully GitOps-driven optimizations via the `spec.gitOps` block on `OptimizationPolicy`. Rather than applying a mutation directly to a live Deployment, Reconcilyx opens a pull/merge request against your manifest repository, waits for CI/human review, and then (optionally) auto-merges. This keeps Argo CD / Flux as the source of truth and eliminates infinite reconciliation loops.

## Provider Configuration

Set the provider type and repository metadata declaratively on the policy:

```yaml
apiVersion: optimization.reconcilyx.io/v1alpha1
kind: OptimizationPolicy
metadata:
  name: cost-optimize
spec:
  mode: AutoApply
  gitOps:
    enabled: true
    provider: github     # mock | github | gitlab
    owner: your-org
    repo: manifests
    baseBranch: main
    manifestsPath: apps/production/
    autoMerge: true      # optional: let Reconcilyx merge when CI + min approvals pass
  targetSelectors:
    workloadSelector:
      matchLabels:
        app: nginx
```

Tokens are provided via environment variables on the controller manager:

| Provider | Env Variable | Optional |
| :--- | :--- | :--- |
| GitHub | `GITHUB_TOKEN` | Required for `provider=github` (classic or fine-grained with repo scope) |
| GitLab | `GITLAB_TOKEN` | Required for `provider=gitlab` (api scope) |
| GitLab | `GITLAB_BASE_URL` | Optional, default `https://gitlab.com/api/v4/` |

### Using the Mock Provider

`provider: mock` is a test double: it never touches a network and records PRs in memory. Its repository must be seeded through `MockProvider.Files`, which is only possible from Go tests, so a live cluster with `provider: mock` fails at manifest resolution rather than fabricating a document. Use GitHub/GitLab for a deployable GitOps policy, and use the mock provider in unit tests such as `internal/gitops/proposal_to_pr_test.go`.

> ⚠️ Provider configuration errors (missing owner/repo/token, unknown provider) are surfaced to the controller and logged; no PR is opened and the proposal does not silently fall back to the mock provider.

## PR / MR Contents

Reconcilyx patches the **existing** Deployment manifest rather than committing a partial document. A partial Deployment YAML in an auto-synced directory would be pruned to those fields by Argo CD/Flux, so a manifest that cannot be found is an error and no PR is opened.

- **Title**: `Optimize resources for <namespace>/<workload>`
- **Head branch**: `reconcilyx/<namespace>-<workload>` (reused idempotently on retry)
- **File**: the manifest resolved under `manifestsPath`. If `manifestsPath` names a `.yaml`/`.yml` file it is used as-is; otherwise the resolver tries `<path>/<workload>.yaml`, `.yml`, `<path>/<workload>/deployment.yaml`, and `.yml`. `Status.ExternalPRID` records the PR/MR number and the resolved path is returned in the PR result.
- **Patch**: the complete file is committed back with the proposed `resources.requests.{cpu,memory}` applied to the containers tracked by the proposal, limits raised when a new request would exceed them, and a `reconcilyx.io/last-proposal` annotation added to the Deployment. Multi-document files, unrelated Deployments, and other namespaces are preserved.
- **Body** (Markdown): workload, recommendation action, confidence (%), risk score (0–100), estimated monthly savings in USD, estimated carbon reduction when available, and rationale.
- **Metrics**: on successful creation, `reconcilyx_pr_created_total{provider,owner,repo}` is incremented.

## Approvals & Auto-Merge

Auto-merge is opt-in at two levels:

1. `spec.gitOps.autoMerge = true` on the policy, and
2. the proposal's own confidence and risk: confidence ≥ 80 and risk ≤ 20.

When both hold, the PR body contains the marker `<!-- reconcilyx-auto-merge: true -->`, the provider is asked to merge immediately, and the result is recorded as auto-merged. When the immediate merge fails (for example, CI is still running), the PR stays open and the webhook path below can merge it later. Auto-merge failures are logged and never fail proposal creation.

For the webhook path, Reconcilyx re-reads the PR/MR and only merges when **all** of the following are true: the state is not already merged/closed, the head branch starts with `reconcilyx/`, and the body carries the marker above. A human-authored PR in the same repository can therefore never be merged by the webhook. (GitHub approval counts are enforced by branch protection during the merge call; `MergeConditions.MinApprovalCount` is still honored when a provider exposes approval counts.)

## Proposal Lifecycle With GitOps

When a GitOps-enabled policy approves a proposal, the controller:

1. Resolves the provider and patches the manifest, opening the PR/MR.
2. Stores the PR/MR ID and moves the proposal to `AwaitingExternalApproval`; it requeues after one minute.
3. Polls the PR/MR on each reconcile: **merged** → `Completed` (the GitOps controller owns the rollout, so the operator does not also resize the live Deployment); **closed without merge** → `Rejected` with `ApprovalRejected`; still open → requeue.
4. Rejects the proposal if the applied policy no longer exists, and fails it if the phase has no recorded PR ID.

This is deliberate: the operator never races Argo CD/Flux for the same Deployment. In-place resize remains the non-GitOps path.

## Template Persistence

Live in-place rescaling changes running Pods and their owning ReplicaSet, not the Deployment template in Git. A rolling update, reschedule, or Git reconciliation will therefore restore the original template requests. Use GitOps mode when the optimization must survive rollouts; it is the supported persistence path. The local resize path is documented as ephemeral in [ARCHITECTURE.md](ARCHITECTURE.md).

## Configuring the Auto-Merge Webhook Server

For true CI-success-driven merging, Reconcilyx ships a standalone webhook listener in `internal/webhook/gitops_webhook.go` that reacts to GitHub status events and GitLab MR + pipeline events.

### Deployment

The controller manager starts the webhook listener itself when `AUTOMERGE_WEBHOOK_ADDR` is set; leave it unset to disable the endpoint.

```bash
export AUTOMERGE_WEBHOOK_ADDR=":8090"          # enables the in-process listener
export GITOPS_WEBHOOK_SECRET="$(openssl rand -hex 32)"
```

If you run the manager in-cluster, add those variables to the manager container and expose port 8090 through a Service/Ingress (the Helm chart does not publish this port by default).

Register the endpoints in your Git provider's Webhooks UI:

- **Payload URL**: `https://<your-public-endpoint>/webhook`
- **Content type**: `application/json`
- **Secret**: the value of `GITOPS_WEBHOOK_SECRET`
- **GitHub events**: `status`
- **GitLab events**: **Merge request events**, **Pipeline events**

### Signature Verification

- **GitHub**: computes HMAC-SHA256 over the payload and compares it to `X-Hub-Signature-256`.
- **GitLab**: compares `X-Gitlab-Token` header to the shared secret.

If `GITOPS_WEBHOOK_SECRET` is empty, verification is skipped (not recommended for production).

### Lifecycle

```
CI status=success ──▶ /webhook ──▶ lookup PR for commit SHA
                                   ├─ no-op if provider does not support lookup
                                   └─ AutoMerge() ──▶ squash-merge + delete branch

Pipeline=success + MR=can_be_merged ──▶ /webhook ──▶ AutoMerge()
```
