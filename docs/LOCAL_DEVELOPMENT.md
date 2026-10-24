# Reconcilyx Local Development Guide

This document describes how to boot, manage, and tear down the local Kubernetes platform environment for Reconcilyx. 

## Architecture Overview

The local environment is entirely self-contained within a `Kind` (Kubernetes IN Docker) cluster. 

```mermaid
graph TD
    subgraph "Local Workstation (Docker)"
        subgraph "reconcilyx-dev (Kind Cluster)"
            M[Metrics Server]
            PROM[Prometheus Stack]
            REC[Reconcilyx Operator]
            
            subgraph "Workload Sandbox"
                W1[CPU Burner]
                W2[Memory Leaker]
                W3[Idle Service]
            end
        end
    end
    
    W1 --> PROM
    W2 --> PROM
    W3 --> PROM
    REC --> PROM
```

## Prerequisites

> [!WARNING]
> **Windows Users:** The local environment scripts and `Makefile` rely heavily on `bash` and standard Unix tools. You MUST execute all local platform commands (e.g., `make dev-up`) from within a WSL (Windows Subsystem for Linux) distribution (like Ubuntu) or a native Linux/macOS environment. Running these commands directly in native Windows PowerShell will fail.

Ensure the following tools are installed on your workstation (inside WSL if on Windows):
- **Docker** (or Docker Desktop)
- **Kind** (`go install sigs.k8s.io/kind@latest`)
- **Helm** (`brew install helm` or equivalent)
- **Kubectl**
- **Make**

## Startup Flow

To build the operator, compile the workloads, start the cluster, and install all observability components, run:

```bash
make dev-up
```

This command will:
1. Create a Kind cluster named `reconcilyx-dev`.
2. Install `metrics-server` (with insecure TLS for local dev).
3. Install the `kube-prometheus-stack` via Helm into the `monitoring` namespace.
4. Install Reconcilyx CRDs (`OptimizationPolicy` and `OptimizationProposal`).
5. Build the Operator Docker image and load it into the Kind cluster.
6. Deploy the Reconcilyx Operator.
7. Deploy the dummy workload sandbox applications (cpu-burner, memory-leaker, idle-service).

## Expected Cluster State

After a successful `make dev-up`, you should see:
- Operator pod running in `reconcilyx-system` namespace.
- Prometheus stack pods running in `monitoring` namespace.
- Workload sandbox pods running in `reconcilyx-system` namespace.

```bash
kubectl get pods -A
```

## Accessing Observability (Grafana & Prometheus)

To view metrics, port-forward the Prometheus or Grafana services:

**Grafana:**
```bash
kubectl port-forward svc/prometheus-grafana -n monitoring 3000:80
```
- Access at `http://localhost:3000`
- Default login: `admin` / `prom-operator`

**Prometheus:**
```bash
kubectl port-forward svc/prometheus-kube-prometheus-prometheus -n monitoring 9090:9090
```
- Access at `http://localhost:9090`

## Scripted End-to-End Validation

`scripts/validation/e2e.sh` (also `make validate-e2e`) exercises the full
in-place resize lifecycle against a deployed operator. It is intentionally
self-contained: it creates a uniquely named namespace and fixture, and removes
them on exit unless `E2E_KEEP=1`.

Prerequisites:
- A manager Deployment running in `reconcilyx-system` with the resize
  subresource enabled.
- Prometheus reachable through the manager's `RECONCILYX_PROMETHEUS_URL`
  (`make install-observability` installs the required `prometheus-k8s` alias).
- Hub API reachable. The harness port-forwards `svc/reconcilyx` on port
  `18082` by default, or uses `E2E_API_URL`.
- An API key. The harness reads `E2E_API_KEY`, then the manager's
  `RECONCILYX_API_KEY` environment variable or mounted secret.

Run it with:
```bash
make validate-e2e
# or, when the Hub API is already reachable:
E2E_API_URL=http://127.0.0.1:8082 E2E_API_KEY=... scripts/validation/e2e.sh
```

The harness pre-seeds live pod requests (`250m/192Mi`) that differ from the
Deployment template (`500m/256Mi` and the `100m/64Mi` sidecar), submits a
`RightSize` proposal through the Hub API, approves it, and waits for
`Completed`. It fails unless all of the following hold:

- the execution snapshot came from live pods, not the Deployment template;
- only `app` was resized to `200m/160Mi`, while `istio-proxy` kept its live
  requests and every limit stayed unchanged;
- pod UIDs are unchanged and no container restarted;
- the learning receipt is finalized with `costSavedUSD=4106m` and a positive
  carbon saving;
- the policy evaluation, JIT allow decision, resize, and completion are in the
  JSONL audit log, with the expected Kubernetes events.

Each run writes `assertions.tsv`, `summary.txt`, the proposal, execution
snapshot, receipt, pod and Deployment dumps, events, audit extracts, the Hub
API response, manager logs, and the resolved configuration under
`artifacts/e2e/<run-id>/` (gitignored). The script exits non-zero if any check
fails.

Configuration is available through `E2E_*` variables documented in the script
header. `E2E_TIMEOUT` (default 600 seconds) bounds the post-approval wait;
`E2E_POLL_INTERVAL` defaults to 10 seconds.

## Teardown Flow

To safely tear down the environment and clean up all resources:

```bash
make dev-down
```

## Troubleshooting

- **Cluster fails to create**: Ensure Docker Desktop is running and has sufficient resources allocated (minimum 4GB RAM).
- **Metrics not showing in Prometheus**: Check if the `ServiceMonitor` resources are applied and have the `release: prometheus` label.
- **ImagePullBackOff for Operator**: Ensure `make kind-push` executed successfully before deploying.
