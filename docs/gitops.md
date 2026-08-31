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

`provider: mock` (the default) logs "PR created" without hitting any network, and is intended for E2E demos and development. Use it to verify the analysis → proposal → PR-handoff flow before provisioning real API credentials.

> ⚠️ All three providers degrade to Mock automatically if the configuration (owner/repo/token) is invalid, with the error surfaced via the proposal's `Conditions[*].message` and a manager log line.

## PR / MR Contents

- **Title**: `Reconcilyx: optimize <namespace>/<workload>`
- **Head branch**: `reconcilyx/<namespace>-<workload>` (forked from base)
- **Commits**: single file under `<manifestsPath>/reconcilyx-proposal-<ts>.yaml` containing a Deployment JSON-patch style YAML document with the new `resources.requests.{cpu,memory}` values.
- **Body** (Markdown): workload, recommendation action, confidence (%), risk score (0–100), estimated monthly savings in USD, and rationale.
- **Metrics**: on successful creation, `reconcilyx_pr_created_total{provider,owner,repo}` is incremented.

## Approvals & Auto-Merge

When `spec.gitOps.autoMerge = true`, the provider:

1. Fetches the PR/MR state.
2. (GitHub only) Counts `APPROVED` reviews and requires ≥ `MergeConditions.MinApprovalCount`.
3. Squash-merges the PR and, where supported, deletes the head branch.
4. Emits a counter increment on `reconcilyx_pr_automerge_total{provider,owner,repo}`.

## Configuring the Auto-Merge Webhook Server

For true CI-success-driven merging, Reconcilyx ships a standalone webhook listener in `internal/webhook/gitops_webhook.go` that reacts to GitHub status events and GitLab MR + pipeline events.

### Deployment

Start the webhook in a sidecar or a separate Deployment (sample manifest: `config/webhook/automerge-webhook.yaml`):

```bash
export AUTOMERGE_WEBHOOK_ADDR=":8090"
export GITOPS_WEBHOOK_SECRET="$(openssl rand -hex 32)"
# Manager binary does not yet start the hook automatically; run:
#   go run ./cmd/manager   # ...or a dedicated binary that calls:
#       webhook.NewAutoMergeWebhookServerFromEnv(logger, factory).Start()
```

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
