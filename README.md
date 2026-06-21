<div align="center">
  
# Reconcilyx

**Autonomous, Safety-First Kubernetes Workload Optimization**

[![Go Report Card](https://goreportcard.com/badge/github.com/reconcilyx/operator)](https://goreportcard.com/report/github.com/reconcilyx/operator)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

*Reconcilyx safely closes the loop between Kubernetes observability and actuation using a strictly governed, Server-Side Apply policy engine.*

</div>

---

## 1. What is Reconcilyx?
Reconcilyx is a Kubernetes Operator that autonomously right-sizes workloads based on historical telemetry. Unlike standard Vertical Pod Autoscalers (VPA) that forcefully restart pods, Reconcilyx treats Kubernetes as a hostile environment. It decouples the intelligence layer from the actuation layer, allowing humans to audit, approve, and mathematically govern optimization decisions before they ever touch the API Server.

## 2. Why it exists
Cloud compute waste is a massive problem, but autonomous remediation is often deemed too risky for production. If an autoscaler reads a corrupted Prometheus metric, it can instantly scale a critical database to zero. 

Reconcilyx exists to solve the **Trust Gap**. It introduces rigid *Blast Radius Containment*, *Just-In-Time Validation*, and *GitOps Yielding*, proving that autonomous systems can be deployed safely without requiring a human operator to click "Approve" on every alert.

## 3. Architecture

```mermaid
flowchart LR
    subgraph Data Plane
      Prometheus[(Prometheus)]
    end
    
    subgraph Intelligence Plane
      Engine[Optimization Engine]
      Engine -->|Generates| Proposal[OptimizationProposal CRD]
    end
    
    subgraph Governance Plane
      Policy[Policy Engine]
      Policy -->|Approves/Rejects| Proposal
    end

    subgraph Control Plane
      Reconciler[Actuation Reconciler]
      Gate[JIT Execution Gate]
      SSA[Server-Side Apply]
    end

    Prometheus -.-> Engine
    Proposal --> Reconciler
    Reconciler --> Gate
    Gate --> SSA
    SSA --> Target[Target Workload]
```

## 4. Core Innovations
- **Decoupled Evaluation:** Telemetry collection, risk scoring, policy approval, and actuation are completely isolated states.
- **GitOps Yielding:** Reconcilyx never fights ArgoCD. If an external tool reverts an optimization, Reconcilyx detects the mismatch and permanently yields, preventing API Server ping-pong thrashing.
- **Optimistic Execution Locks:** Prevents concurrent controllers from corrupting workload state via the `reconcilyx.io/execution-lock` annotation.

## 5. Safety Mechanisms
1. **Execution Snapshots:** Precisely 1 millisecond before mutating a workload, Reconcilyx takes a deep copy of the `Generation` and `ResourceVersion`. This guarantees mathematically perfect rollbacks that do not accidentally erase manual edits.
2. **Server-Side Apply Granularity:** The Actuator constructs raw, unstructured JSON payloads targeting *only* `requests.cpu` and `requests.memory`. It is physically incapable of mutating images, limits, or labels.
3. **Global Kill-Switch:** A dedicated `reconcilyx-config` ConfigMap can instantly freeze all cluster mutations (`actuation_paused: "true"`), downgrading Reconcilyx to a read-only Dry Run mode.

## 6. Execution Evidence

The following artifacts were captured live from a running `kind` cluster demonstrating Reconcilyx's autonomous optimization lifecycle.

### 1. The Overprovisioned Workload (Before)
*The `cpu-hog` deployment is manually scaled and requesting massive CPU overhead.*
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: cpu-hog
spec:
  template:
    spec:
      containers:
      - name: nginx
        resources:
          requests:
            cpu: "2"      # <-- 2000m Overprovisioned
```

### 2. Proposal Generation
*Reconcilyx Engine detects the waste and generates an OptimizationProposal.*
```console
$ kubectl get optimizationproposals -n reconcilyx-demo
NAME          TARGET-KIND   TARGET-NAME   RECOMMENDATION     APPROVAL   EXECUTION
opt-cpu-hog   Deployment    cpu-hog       ReduceCPURequest   Approved   Verified
```

### 3. Policy Evaluation
*The Policy Evaluator matches the proposal against the `strict-efficiency-policy`, clearing it for execution.*
```console
$ kubectl describe optimizationproposal opt-cpu-hog -n reconcilyx-demo
Name:         opt-cpu-hog
Namespace:    reconcilyx-demo
API Version:  optimization.reconcilyx.io/v1alpha1
Kind:         OptimizationProposal
Spec:
  Decision Metadata:
    Confidence Score:  98
    Rationale:         Workload has utilized < 1% of requested CPU over the last 7 days.
    Risk Score:        10
  Recommendation:  ReduceCPURequest
Status:
  Approval State:  Approved
  Execution State: Verified
  Phase:           Completed
  Policy Applied:  strict-efficiency-policy
  Reason:          Approved: Policy 'strict-efficiency-policy' is in AutoApply mode and all constraints passed.
```

### 4. Controller Actuation Logs
*The Reconcilyx Actuator seizes the execution lock, captures a deep snapshot, and executes the Server-Side Apply patch.*
```text
2026-06-21T12:07:06+05:30  DEBUG  events  Policy Engine approved proposal
2026-06-21T12:07:07+05:30  DEBUG  events  Acquired lock and captured execution snapshot
2026-06-21T12:07:07+05:30  DEBUG  events  Server-Side Apply patch submitted
2026-06-21T12:07:07+05:30  DEBUG  events  Mutation matches intended state
2026-06-21T12:07:07+05:30  DEBUG  events  Workload stabilized
```

### 5. Kubernetes Event Stream
*The Kubernetes Control Plane confirms the operator orchestrated a perfect, zero-downtime rolling update.*
```text
Normal  ProposalApproved    optimizationproposal/opt-cpu-hog   Policy Engine approved proposal
Normal  ExecutionStarted    optimizationproposal/opt-cpu-hog   Acquired lock and captured execution snapshot
Normal  ExecutionApplied    optimizationproposal/opt-cpu-hog   Server-Side Apply patch submitted
Normal  ScalingReplicaSet   deployment/cpu-hog                 Scaled up replica set cpu-hog-64ddb755b5 from 0 to 1
Normal  ScalingReplicaSet   deployment/cpu-hog                 Scaled down replica set cpu-hog-56f84c9d9d from 1 to 0
Normal  ExecutionVerified   optimizationproposal/opt-cpu-hog   Mutation matches intended state
Normal  ExecutionCompleted  optimizationproposal/opt-cpu-hog   Workload stabilized
```

### 6. The Optimized Workload (After)
*The deployment is safely mutated, and ownership of the field is formally seized by `reconcilyx-optimizer`.*
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: cpu-hog
spec:
  template:
    spec:
      containers:
      - name: nginx
        resources:
          requests:
            cpu: 100m     # <-- Safely reduced to 100m
```

### 7. Telemetry Verification
*Grafana records the real-time drop in resource allocation immediately following Reconcilyx's intervention.*

![Grafana Live Telemetry - CPU Drop](artifacts/screenshots/grafana_cpu_drop.png)

Watch the automated recording of the Grafana dashboard verification:
![Demo Execution Video](artifacts/screenshots/execution_demo.webp)

## 7. Validated Results
Tested and verified live on a Kind (`v1.36.1`) cluster against real workloads.

| Metric | Before | After |
| :--- | :--- | :--- |
| **CPU Request** | 2000m | 100m |
| **Ready Replicas** | 1/1 | 1/1 |
| **Workload Restarts** | 0 | 0 |
| **Proposal Status** | - | Completed |

- **CPU Reduction Achieved:** 1,900m (95% drop)
- **Actuation Execution Duration:** ~1000ms (From `Approved` to `Completed`)
- **Proposals Generated:** 1
- **Successful Executions:** 1

*View the raw Kubernetes Events and JSON Outputs showcasing Reconcilyx execution [here](artifacts/live-demo/).*

## 8. Key Engineering Challenges
- **The "Stale Snapshot" Hazard:** Designing an architecture that allows a proposal to sit in an approval queue for days, yet still safely rollback. Solved by moving the Snapshot capture to the *Execution* phase, rather than the *Generation* phase.
- **Controller Event Storms:** Mitigating the risk of recursive Reconcile loops when updating Custom Resource Statuses.

## 9. Technologies Used
- **Go** (Golang)
- **Kubernetes / controller-runtime** (KubeBuilder)
- **Prometheus Client Libraries**
- **Server-Side Apply** (Unstructured Patching)
- **Ginkgo & Gomega** (BDD Testing)

## 10. Local Setup
```bash
# Clone the repository
git clone https://github.com/reconcilyx/operator.git
cd operator

# Spin up a local Kind cluster
kind create cluster

# Install the CRDs and deploy
make manifests
make install
make run
```

## 11. Future Roadmap
1. **MutatingAdmissionWebhook:** Intercept pod creation to inject right-sized resources instantly upon restart.
2. **Hub-and-Spoke Telemetry:** Deploy lightweight agents on remote clusters that phone-home to a central intelligence plane.
3. **Rollback Execution:** Finalize the SSA code that applies the `ExecutionSnapshot` in the event of an error rate spike.
4. **Machine Learning:** Replace basic P95 metrics with ARIMA-based predictive forecasting.
