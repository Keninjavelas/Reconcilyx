# Reconcilyx Operator Guide (`v4.1.0-rc1`)

Reconcilyx evaluates Kubernetes workloads and turns cost or carbon opportunities into reviewable `OptimizationProposal` resources. This guide covers the verified workflows and explicitly documents which live external integrations remain optional Bring-Your-Own-Key (BYOK) or hardware-gated features.

For deployment prerequisites and security context, read [DEPLOYMENT.md](DEPLOYMENT.md) first.

---

## 1. Installation

### 1.1 Helm (bundled chart)

```bash
helm upgrade --install reconcilyx ./charts/reconcilyx \
  --namespace reconcilyx-system --create-namespace
```

CRDs are bundled in `charts/reconcilyx/crds/`. No remote chart repository is published; install from the checkout or a built chart archive. Set the image tag with `--set image.tag=<tag>` when not using the default.

### 1.2 CLI plugin

```bash
make build-cli
./bin/kubectl-reconcilyx --help
```

On Windows the binary is `bin/kubectl-reconcilyx.exe`. The plugin talks to the Hub API, not directly to etcd. Global flags:

```text
--server   Hub API endpoint (default http://localhost:8082)
--token    X-API-Key value when the API is protected
--namespace  Limit or scope a command to one namespace
```

Commands: `scan`, `proposals`, `explain`, `what-if`, `diff`, `approve`, `reject`, `rollback`, and `chat`.

---

## 2. Core workflow

1. Apply an `OptimizationPolicy` that selects namespaces, labels, or workloads.
2. The controller evaluates matching workloads and creates `OptimizationProposal` resources with the observed baseline, proposed request, confidence, risk, and expected cost/carbon impact.
3. A proposal in `Pending` can be approved or rejected from the console, the API, or the CLI.
4. Approved proposals pass through the execution gate, capture an execution snapshot, and are applied with in-place pod resizing.
5. The controller verifies and stabilizes the workload. Rollback uses the captured snapshot.

The proposal CRD accepts `RightSize`, `ScaleToZero`, `ReduceCPURequest`, `ReduceMemoryRequest`, and `MigrateCloud`. The v4 beta actuator executes the three resize types by Server-Side Apply against the Pod `resize` subresource with the `reconcilyx-operator` field manager; `ScaleToZero` and `MigrateCloud` can be evaluated and proposed, but the in-place actuator rejects them at execution time.

Each proposal carries `spec.proposedResourcesByContainer`, a map of proposed requests keyed by container name. The actuator and the GitOps manifest patcher change only the containers listed there, so sidecars such as `istio-proxy` keep their existing requests. When the map is absent, a proposal is actuated only if the execution snapshot has a single container; a multi-container workload fails closed with a `Failed` execution state rather than applying one request set to every container. `spec.proposedResources` is the aggregate summary used for display and single-container compatibility.

### Modes

| Mode | Behaviour |
|---|---|
| `Observe` | Record findings without proposals. |
| `Propose` | Queue proposals for human review. |
| `AutoApply` | Approve proposals that satisfy the policy's safety constraints. |

Guardrails in the policy cover minimum replicas, risk and confidence thresholds, carbon weight, cooldown, and rollback error rate. Workload protection can exclude sidecar containers. Evaluation uses live context: `cooldownPeriod` is measured from the newest executed change to the same workload, and protected-container checks read the target Deployment, StatefulSet, or DaemonSet pod template.

Three execution-time guards re-check state when a scheduled proposal is about to run:

- `safetyConstraints.cooldownPeriod` rejects a change that lands inside the cooldown window, even if the proposal was approved earlier. The policy CRD defaults it to `15m`.
- `changeBudget.maxChangesPerWindow` counts executed changes to the same workload inside a rolling `window` (default `24h`). A budget of `0` disables the check; a proposal is rejected when the count has already reached the budget.
- `blastRadius.maxReplicasPerChange` rejects changes to Deployments or StatefulSets above the replica cap. DaemonSets are rejected when the cap is enabled because their node count is unknown. `blastRadius.requireCanaryLabel` additionally requires `reconcilyx.io/canary=true` on the workload or its pod template.

The JIT gate fails closed: if an applied policy disappeared or the Hub cannot read the target workload, the proposal is marked `Failed` with the gate reason instead of executing.

Policy evaluations and execution outcomes are appended as newline-delimited JSON to the `reconcilyx-audit` ConfigMap in the proposal namespace, keeping the 500 newest entries. This is separate from the chat audit tail served at `GET /api/audit`.

### CLI examples

```bash
kubectl-reconcilyx proposals
kubectl-reconcilyx approve payment-service-rightsize-7f2c -n production --reason "P95 confirmed"
kubectl-reconcilyx reject search-indexer-scale-1d33 -n batch --reason "Cold-start tolerance is unknown"
kubectl-reconcilyx rollback ledger-worker-rightsize-2b8e -n payments --reason "Latency regression"

# Inspect the Hub's optimization opportunity list (no synthetic fallback data)
kubectl-reconcilyx scan -n production
kubectl-reconcilyx scan -o json > scan.json

# Reconcile predicted impact with the realized learning receipt
kubectl-reconcilyx explain payment-service-rightsize-7f2c -n production
kubectl-reconcilyx explain payment-service-rightsize-7f2c -n production -o json

# Simulate a resize and fail CI on an unsafe or cost-increasing change
kubectl-reconcilyx what-if \
  --current-cpu 500m --current-memory 256Mi \
  --proposed-cpu 250m --proposed-memory 128Mi \
  --fail-on not-allowed,cost-increase

# Compare manifests with a git base and enforce guardrails
kubectl-reconcilyx diff --manifests deploy/ --base origin/main \
  --fail-on cost-increase,guardrail
kubectl-reconcilyx diff --manifests deploy/app.yaml --base HEAD~1 -o json
```

Reject requires `--reason`. Approve and rollback default to a generic reason. The CLI prints the API error for non-2xx responses. `scan`, `explain`, `what-if`, and `diff` accept `-o json`; `what-if` and `diff` exit `2` when a `--fail-on` condition triggers and `1` for transport, HTTP, or validation errors.

---

## 3. Console

The console is embedded in the Hub and served at `/` on the API port. It provides:

- **Overview** — spend and carbon KPIs, savings trend, recent proposals, anomalies.
- **Proposals** — filters, search, bulk approve/reject, rollback, and a proposal inspector.
- **Savings** — a ledger of proposal-derived savings grouped by creation date, plus cluster and namespace breakdowns.
- **Forecast** — hourly forecast, observed versus proposed resources, and a what-if calculator.
- **Policies** — the policy catalogue and a create sheet whose preview is the exact `OptimizationPolicy` payload.
- **Settings** — control-plane reachability test, BYOK assistant key, monthly goal, and server-side environment reference.

Runtime connection settings and the assistant key are stored in the browser. They are not written to the cluster.

---

## 4. ChatOps

The assistant routes a question to server-side tools that read proposals, policies, savings, carbon, forecasts, anomalies, clusters, and workloads. Write actions return a confirmation token and require an explicit confirmation before execution; every request is written to the audit log.

```bash
kubectl-reconcilyx chat --provider ollama
kubectl-reconcilyx chat --provider openai --llm-key "sk-..."
```

The model provider must be reachable from the Hub. Passing a key through the CLI or the console keeps it out of the cluster, but it still transits the API request to the Hub.

---

## 5. GitOps

Policies can enable GitOps with the `mock`, `github`, or `gitlab` provider. When enabled, approved proposals patch the existing Deployment manifest and open a pull request instead of resizing pods directly. The proposal waits in `AwaitingExternalApproval` until the PR/MR merges (`Completed`) or closes (`Rejected`); the GitOps controller performs the rollout. Provider tokens are configured on the Hub (`GITHUB_TOKEN`, `GITLAB_TOKEN`), not in the browser. Auto-merge settings are policy fields; Reconcilyx also requires confidence ≥ 80 and risk ≤ 20 before it marks a PR auto-mergeable, and consult your repository protections before enabling it. See [gitops.md](gitops.md) for manifest resolution and webhook setup.

---

## 6. What the v4.0 beta does not do

These are known limits, not configuration mistakes:

- **eBPF telemetry** is deferred to v5.0 planning. Telemetry uses Prometheus; invoking the eBPF stub emits a warning.
- **Node consolidation and Karpenter actuation** are deferred to v5.0 planning.
- **Console & public Read-Only Evidence Replay:** Public zero-install inspection is served continuously at [`https://reconcilyx.vercel.app/`](https://reconcilyx.vercel.app/) (`HTTP 200`) in transparent **Read-Only Evidence Replay Mode** (with an interactive **Connect Live Hub** modal), while no paid mutating public VPS Hub is operated.
- **GPU/DCGM and JVM heap telemetry (contract-verified; hardware-gated):** `InspectGPU` and `ScrapeGPUMetrics` query NVIDIA DCGM Prometheus series (`DCGM_FI_DEV_GPU_UTIL`, `DCGM_FI_DEV_FB_USED`, `DCGM_FI_DEV_FB_TOTAL`, `DCGM_FI_DEV_GPU_TEMP`, `DCGM_FI_DEV_SM_CLOCK`, `DCGM_FI_DEV_POWER_USAGE`, `DCGM_FI_PROF_GR_ENGINE_ACTIVE`, plus `MIGProfile`) alongside JMX heap metrics and feed `DetermineGPUBucket` / `DetermineJVMBucket`. Ingestion and MIG/bucket sizing are contract-verified against Prometheus exposition series in CI; in production clusters, live GPU collection is hardware-gated on nodes equipped with physical NVIDIA GPUs running `dcgm-exporter`, and workloads without exporters cleanly yield zero values rather than synthetic data.
- **JVM `-Xmx` rewriting** exists as an engine helper and test path; it is not wired into the live actuator.
- **Multi-cloud migration** produces a proposal and a generated Velero backup/restore script. Reconcilyx does not execute the migration.
- **In-place resize is ephemeral:** the operator changes running Pods and the owning ReplicaSet, not the Deployment template. A rollout or reschedule restores the original requests; use a GitOps-enabled policy when the change must persist.
- **Autoscaler conflicts block execution:** proposals targeting a Deployment with an active HPA, VPA, or KEDA `ScaledObject` are rejected before mutation (VPAs in `updateMode: Off` are recommendation-only). Disable or retarget the autoscaler first.
- **Federated learning & managed cloud K8s target:** Two-cluster Hub-and-Spoke federation (`stable`/`canary` channels, trimmed-mean outlier rejection, 50 MiB FIFO spool) is verified in Kind CI (`validate-federation.yml`), and the 66-check lifecycle harness + 9-check TLS probe are verified on 3-node Kind TLS (`#20261026055631`) and GitHub-hosted CI (`validate-e2e.yml`). User-provisioned managed cloud clusters (`deploy/terraform/` for `aws-eks`, `gcp-gke`, `generic-kubeconfig`) and public Let's Encrypt DNS cutovers are supported user-supplied targets; live cloud billing clusters remain BYOK/user-provisioned infrastructure.
- **Cloud pricing (Azure live verified; AWS/GCP optional BYOK):** Azure uses the anonymous Retail Prices API (`https://prices.azure.com/api/retail/prices`) and is live-verified with zero credentials (`Standard_D2s_v5/eastus` `$0.096/h`, `Standard_F2s_v2/eastus` `$0.0846/h`, `Standard_B2s/eastus` `$0.0416/h`). Live AWS (`pricing:GetProducts` via `AWS_REGION` / IAM / IRSA STS) and GCP (`cloudbilling.googleapis.com` via `RECONCILYX_PRICING_GCP_API_KEY` / Workload Identity) are optional Bring-Your-Own-Key (BYOK) integrations whose SigV4 signing, SKU pagination/vCPU+RAM summation, and full-jitter retry/fallback parsers are contract-verified in CI; when credentials are absent, both default automatically to the bundled static regional catalog (`static-fallback`). Spot prices are estimates derived from on-demand rates.
- **Carbon intensity (optional BYOK live providers; static default):** Live grid carbon intensity lookups (`Electricity Maps` and `WattTime`) are optional and credential-gated via `RECONCILYX_CARBON_PROVIDER` plus provider API credentials; when credentials are absent or live requests fail, the carbon factory defaults to the deterministic static regional provider (`static`; offline `kubectl-reconcilyx diff` tags `static-regional-fallback`). Live external carbon APIs remain optional BYOK endpoints.
- **Security audit posture:** Internal control verification and DevSecOps CI gates (`govulncheck`, `gitleaks`, `trivy`, keyless `cosign`, Syft SPDX SBOMs) are documented in [SECURITY_AUDIT.md](SECURITY_AUDIT.md); regulated enterprise adopters should still pair deployments with an optional organization-specific penetration test.
- **Workload kinds**: execution supports Deployments. Other kinds may be evaluated but are not actuated by the in-place resize path.

See [FEATURE_MATRIX.md](../FEATURE_MATRIX.md), [STATUS.md](../STATUS.md), and [SECURITY_AUDIT.md](SECURITY_AUDIT.md) for current verification evidence and BYOK/hardware gate scopes.
