# Troubleshooting

## Build / Scaffold

- **`make manifests` / `make generate` fails**: ensure `controller-gen` is installed and on `PATH`:
  ```bash
  go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest
  ```
- **`go build ./...` fails on missing `go-github/v63/github`**: run `go mod tidy` to pull the direct dependency added by `internal/gitops/github.go`.
- **SSAConflictsTotal redeclared**: only one copy of the metric variable exists; the labeled `CounterVec` in `internal/telemetry/metrics.go` is the canonical one. If you see this error after merging branches, delete the `var SSAConflictsTotal` declaration in `internal/telemetry/constants.go`.

## Operator Installation

- **Operator crashes with "Failed to build analyzer engine"**: the configured Prometheus URL (`--prometheus-url` / `PROMETHEUS_URL`) is unreachable at startup. The policy controller still runs but only in Observe mode until connectivity is restored. Fix the URL or install Prometheus into the cluster (see `config/prometheus/`).
- **No proposals appear after applying an `OptimizationPolicy`**:
  1. Check `kubectl get optimizationpolicy -o yaml` and verify `status.conditions[*].type == Validated` reports `True`.
  2. Ensure the deployment's pod labels match `spec.targetSelectors.workloadSelector.matchLabels`.
  3. Check controller-manager logs for `Analyzer error` entries.
- **Webhook server refuses to start**: verify ENABLE_WEBHOOKS env var is not `"false"`, and that the cert-manager manifests or `webhook-cert-path` points at valid TLS assets (`tls.crt`, `tls.key`). For local development, use `ENABLE_WEBHOOKS=false make run`.

## Analysis / Recommendations

- **Recommendations always look conservative / no RL / forecast used**: check `AnomalySeverity{workload,namespace,metric}` in `/metrics`. If CPU or memory severity > **0.7** in the last analysis window, the safety gate intentionally bypasses RL and forecast to avoid acting during anomalous workload behaviour.
- **`ProposalStaleness` keeps growing**: reconciler concurrency is saturated. Bump `--concurrency` and/or `--batch-size`. Check that proposals have not all been TTL-gated by the cooldown period.
- **Confidence is always 0.3**: the RL agent is still in the initial exploration phase (epsilon is high). After 50–100 `Update()` rewards with positive feedback, epsilon decays and exploit-mode confidence (≈0.7+) kicks in.
- **Savings USD always 0**: the analyzer uses estimated cloud-cost rates from a built-in defaults table. To customize, extend `engine/savings.go` with per-node pricing (on roadmap for v2.1).

## Execution / Rollback

- **OptimizationProposal stuck in `PhaseExecuting`**:
  - `kubectl describe optimizationproposal <name>` and check Events.
  - If the error is `"forbidden recommendation type"`, ensure you have the RightSize fix in `internal/controller/actuator.go`.
- **OptimizationProposal keeps rolling back (3-strike)**: the post-execution monitoring window is flagging either OOMKilled restarts or sustained CPU-throttling. Inspect `kubectl get pods -o yaml` for container restart reasons; temporarily raise `spec.safety.minConfidence` before re-running.
- **Server-Side Apply conflicts**: the controller emits `SSAConflictsTotal{workload,namespace}`. Common causes:
  - Another operator / Flux / Argo CD also owns the same field. Reconcilyx detects Argo/Flux annotations and `GitOpsYieldsTotal` is incremented – if both values are climbing, disable in-place mode and switch fully to GitOps PRs.
  - Stale field manager name. Set a consistent field manager (`kubectl apply --field-manager=reconcilyx`) or delete the Deployment and re-apply.
- **InPlacePodVerticalScaling not applied**: confirm the feature gate is enabled on your cluster (`kind create cluster --config kind-config.yaml` with `featureGates.InPlacePodVerticalScaling=true`). On vanilla Kubernetes 1.31+, the feature is beta and enabled by default. For older versions, the actuator falls back to a Deployment spec edit (rolling restart).

## GitOps PRs

- **PRs not created**: run `kubectl get optimizationproposal -o yaml`. Under `status.conditions[]` look for `GitOpsPRCreated` or an error message such as `"github provider requires gitOps.owner and gitOps.repo"`.
- **PRs created but empty diff (comment-only)**: the analyzer produced a `RightSize` recommendation but the deployment had no matching workload labels. Re-check `spec.targetWorkload` matches your manifest's Deployment name.
- **Provider calls fail with 401 / 403**:
  - GitHub: ensure `GITHUB_TOKEN` has `repo` (or fine-grained: pull-requests read/write, contents read/write) scope.
  - GitLab: `GITLAB_TOKEN` needs `api` scope; also check `GITLAB_BASE_URL` ends with `/api/v4/`.
- **Auto-merge never triggers**:
  1. Check the webhook listener's health on `/healthz`.
  2. Confirm CI status events (GitHub) or Merge request + Pipeline events (GitLab) are actually delivered – most providers list recent deliveries under Settings → Webhooks.
  3. If `MinApprovalCount` is non-zero, ensure at least that many reviewers marked the PR as `APPROVED` before the success status arrived.

## Metrics / Grafana

- **Custom metrics missing from `/metrics`**: scrape the manager's `--metrics-bind-address` (NOT the kubelet). Check `config/monitoring/` for ServiceMonitor references.
- **Grafana shows "No data" for RL Rewards / Savings**: these gauges are emitted lazily – they appear only after at least one proposal has been approved. Generate a demo workload (see `demo/run.sh`) to warm the panels.

## Support / Debugging Switches

| Flag | Env | Purpose |
| :--- | :--- | :--- |
| `--zap-devel` | — | Verbose development logs |
| `--batch-size` | `RECONCILYX_BATCH_SIZE` | Max deployments analyzed per policy reconcile pass (default 10) |
| `--concurrency` | `RECONCILYX_CONCURRENCY` | Max concurrent reconciles per controller (default 4) |
| `--pprof-bind-address` | `PPROF_ADDR` | pprof debug endpoints, disabled if empty (default `:6060`) |
| `--prometheus-url` | `PROMETHEUS_URL` | Prometheus URL used by the retriable telemetry client |
| — | `ENABLE_WEBHOOKS` | Set to `"false"` to skip admission webhook startup |
