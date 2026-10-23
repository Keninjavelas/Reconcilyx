# Reconcilyx Operator Guide (v4.0.0-beta)

Reconcilyx evaluates Kubernetes workloads and turns cost or carbon opportunities into reviewable `OptimizationProposal` resources. This guide covers the workflows that exist in the beta. It deliberately does not describe deferred or stubbed capabilities as available.

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

Commands: `proposals`, `approve`, `reject`, `rollback`, and `chat`.

---

## 2. Core workflow

1. Apply an `OptimizationPolicy` that selects namespaces, labels, or workloads.
2. The controller evaluates matching workloads and creates `OptimizationProposal` resources with the observed baseline, proposed request, confidence, risk, and expected cost/carbon impact.
3. A proposal in `Pending` can be approved or rejected from the console, the API, or the CLI.
4. Approved proposals pass through the execution gate, capture an execution snapshot, and are applied with in-place pod resizing.
5. The controller verifies and stabilizes the workload. Rollback uses the captured snapshot.

The v4 beta actuator executes `RightSize`, `ReduceCPURequest`, and `ReduceMemoryRequest` by Server-Side Apply against the Pod `resize` subresource with the `reconcilyx-operator` field manager. `ScaleToZero` and `MigrateCloud` can be evaluated and proposed, but the in-place resize actuator rejects them at execution time.

### Modes

| Mode | Behaviour |
|---|---|
| `Observe` | Record findings without proposals. |
| `Propose` | Queue proposals for human review. |
| `AutoApply` | Approve proposals that satisfy the policy's safety constraints. |

Guardrails in the policy cover minimum replicas, risk and confidence thresholds, carbon weight, cooldown, and rollback error rate. Workload protection can exclude sidecar containers.

### CLI examples

```bash
kubectl-reconcilyx proposals
kubectl-reconcilyx approve payment-service-rightsize-7f2c -n production --reason "P95 confirmed"
kubectl-reconcilyx reject search-indexer-scale-1d33 -n batch --reason "Cold-start tolerance is unknown"
kubectl-reconcilyx rollback ledger-worker-rightsize-2b8e -n payments --reason "Latency regression"
```

Reject requires `--reason`. Approve and rollback default to a generic reason. The CLI prints the API error for non-2xx responses.

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

Policies can enable GitOps with the `mock`, `github`, or `gitlab` provider. When enabled, approved proposals open a pull request instead of patching pods directly. Provider tokens are configured on the Hub (`GITHUB_TOKEN`, `GITLAB_TOKEN`), not in the browser. Auto-merge settings are policy fields; consult your repository protections before enabling them.

---

## 6. What the v4.0 beta does not do

These are known limits, not configuration mistakes:

- **eBPF telemetry** is deferred to v5.0 planning. Telemetry uses Prometheus; invoking the eBPF stub emits a warning.
- **Node consolidation and Karpenter actuation** are deferred to v5.0 planning.
- **GPU/DCGM telemetry** returns stub values; it is not a live DCGM scrape.
- **JVM `-Xmx` rewriting** exists as an engine helper and test path; it is not wired into the live actuator.
- **Multi-cloud migration** produces a proposal and a generated Velero backup/restore script. Reconcilyx does not execute the migration.
- **Federated learning** and **model registry rollout** are partially implemented; live production rollout wiring is incomplete.
- **Workload kinds**: execution supports Deployments. Other kinds may be evaluated but are not actuated by the in-place resize path.

See [FEATURE_MATRIX.md](../FEATURE_MATRIX.md) for the conservative inventory and [STATUS.md](../STATUS.md) for evidence.
