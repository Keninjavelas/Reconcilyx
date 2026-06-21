# Reconcilyx Architecture

## Refined Project Vision
**Reconcilyx is an autonomous Kubernetes optimization platform that observes workload behavior, detects waste and regressions, predicts the impact of changes, and safely reconciles resources toward policy-driven efficiency goals.**

Unlike traditional dashboards that only visualize metrics, Reconcilyx acts as a **control plane for optimization**. It closes the loop between observability and action, operating with a FinOps intelligence mindset and GitOps-aware safety.

## Problem Statement
Real-world Kubernetes environments consistently suffer from resource waste and instability due to:
* Overprovisioned CPU and memory limits based on guesswork.
* Idle services consuming resources unnecessarily.
* Deployment regressions that silently increase infrastructure costs.
* Alert fatigue from noisy metrics without actionable, automated remediation.
* A lack of safe, policy-gated automation to right-size workloads dynamically.

## Design Principles
1. **Safety Through Proposals**: Never blindly mutate state. Actions must be proposed, scored for confidence, checked against policy, and optionally approved before execution.
2. **Observability-Driven**: Decisions are only as good as the telemetry. Rely on established metrics (Prometheus) with an extensible design for kernel-level signals (eBPF) later.
3. **Policy-Gated**: Business rules (e.g., "no automated changes in prod") dictate behavior, cleanly separating optimization logic from governance.
4. **Local-First**: The platform must be fully deployable on a local machine (Kind/K3d) for rapid iteration, zero-cost development, and impressive portfolio demos.
5. **Engineering Depth over Feature Count**: A single, robust, well-tested reconciliation loop is more valuable than ten shallow features.

## Non-Goals
* **Replacing HPA/VPA entirely**: Reconcilyx is an intelligent complement, focusing on right-sizing baselines and detecting waste, rather than simply reacting to short-term CPU spikes.
* **Generic APM**: It is not a replacement for Datadog or New Relic. It focuses strictly on metrics relevant to resource efficiency and cost.
* **Multi-Cluster Federation (MVP)**: Kept out of scope initially to focus on the core single-cluster reconciliation loop.

## System Boundaries
* **Input (Telemetry)**: Ingests metrics from Prometheus and K8s API (events, state).
* **Control Plane (Operator)**: Processes metrics, generates `OptimizationProposals`, evaluates `OptimizationPolicies`.
* **Actuation (Reconciliation)**: Applies patches to Deployments/StatefulSets or generates GitOps PRs.
* **Output (UX)**: Exposes a timeline of decisions, savings, and proposals to users.

## Core Reconciliation Loop
The heart of Reconcilyx is not a static script, but a continuous, state-aware loop:
`Observe → Analyze → Predict → Propose → Approve → Reconcile → Verify`

## Local-First Architecture
The entire stack runs in a local Kubernetes cluster (Kind/K3d).
* **Dependencies**: Prometheus for metrics, basic K8s metrics-server.
* **Simulators**: Lightweight pods (CPU burner, memory leak simulator, idle service) generate realistic telemetry for the platform to analyze.
* **Platform**: The Reconcilyx Operator runs in-cluster, watching the simulators.

## Cloud-Ready Architecture
While designed locally, the architecture seamlessly transitions to the cloud.
* Uses standard K8s API machinery (CRDs, controllers).
* Can persist decision history to an external database (e.g., Postgres) for long-term retention.
* Supports standard Ingress controllers for dashboard access.

## Component Interaction Flow
1. **Prometheus** scrapes metrics from the K8s cluster and Workload Simulators.
2. The **Optimization Engine** (inside the Operator) queries Prometheus periodically.
3. The Engine detects waste and generates an `OptimizationProposal` Custom Resource.
4. The **Policy Engine** evaluates the proposal against `OptimizationPolicy` rules.
5. If auto-apply is enabled and safe, the **Reconciliation Engine** patches the target workload.
6. The **Stabilization Window** logic prevents further changes until the workload proves stable.

## Technology Decisions with Rationale
* **Go (Golang)**: The undisputed standard for Kubernetes Operators (using Kubebuilder / controller-runtime). Ensures low footprint and native K8s API integration.
* **Prometheus**: The industry standard for K8s observability. Leveraging PromQL allows for complex time-series analysis without building a custom metrics store.
* **React/TypeScript (Frontend)**: For a premium, interactive dashboard capable of displaying complex timelines and Mermaid graphs.
* **CRDs (Custom Resource Definitions)**: For state management (`OptimizationPolicy`, `OptimizationProposal`). Native to K8s, highly extensible.

## Risks and Mitigation Strategies
* **Risk: Flapping (Continuous scaling up and down)**
  * **Mitigation**: Mandatory stabilization windows after any mutation.
* **Risk: Aggressive scale-down causing OOM/Throttling**
  * **Mitigation**: Dry-run modes, confidence scoring, and strict policy limits (e.g., max 10% reduction per window).
* **Risk: Architectural Bloat (Too many microservices)**
  * **Mitigation**: Consolidate the Optimization Engine, Policy Engine, and Operator into a **single binary** (The Reconcilyx Operator) for the MVP.
