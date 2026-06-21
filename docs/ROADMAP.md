# Reconcilyx Implementation Roadmap

## MVP Scope (The "Core Thesis" Delivery)
**Goal:** Prove the core reconciliation loop (Observe -> Propose -> Reconcile) locally with zero unnecessary complexity.
* **Infrastructure**: Kind cluster, Prometheus stack.
* **Workload Simulators**: `idle-service` and `cpu-burner`.
* **Control Plane**: A single Go Operator capable of reading Prometheus metrics.
* **Custom Resources**: `OptimizationPolicy` and `OptimizationProposal`.
* **Action**: Detect overprovisioned CPU/Memory and auto-patch the Deployment based on a simple policy.
* **UI**: None. Interaction is purely via `kubectl get optimizationproposals`.

## Phase 1: Safety & Governance
**Goal:** Make the system robust and safe enough for a production environment.
* Implement Stabilization Windows (preventing flapping).
* Implement Dry-run mode (log changes without applying).
* Expand Policy Engine: Support namespace-level rules, max change percentages, and manual approval gating.
* Automated Rollbacks upon detecting OOMs or crash loops post-optimization.

## Phase 2: Visibility & Premium UX
**Goal:** Deliver the flagship portfolio experience.
* Build the React/TypeScript Reconcilyx Dashboard.
* Display the Timeline of Optimization Decisions (The Narrative).
* Provide a "Why this recommendation?" detail panel.
* Human-in-the-loop UI: "Approve" or "Reject" proposals directly from the dashboard.

## Phase 3: GitOps & Advanced Scenarios
**Goal:** Align with modern Platform Engineering practices.
* **GitOps PR Generation**: Instead of live-patching K8s, the Operator pushes a commit to a GitHub/GitLab repo and opens a Pull Request.
* Cost estimation: Translate resource reductions into estimated monthly savings ($).
* Advanced Workload Simulators (Burst traffic, memory leaks).

## Stretch Goals
* **Gamification**: Efficiency scoring per namespace/team.
* **ChatOps**: Slack/Discord integration for proposal approvals.

## Build Order Optimized for Learning & Delivery
1. **Repository Setup & CRD Scaffolding**: Setup Kubebuilder, define CRD schemas.
2. **Workload Simulators**: Write the dummy apps to have something to monitor.
3. **Local Env Automation**: A `make local-env` script to spin up Kind + Prometheus + Simulators.
4. **Metrics Integration**: Teach the Operator to query Prometheus.
5. **The Engine**: Write the logic to detect waste and create Proposals.
6. **The Reconciler**: Write the logic to patch K8s deployments.
7. **The Frontend**: Build the UI last, once the backend API and CRDs are perfectly stable.

## Intentionally Deferred Features (Non-Goals for now)
* **eBPF Telemetry**: Deferred. While extremely powerful, standard Prometheus metrics are sufficient to prove the thesis. Adding eBPF adds massive complexity to the MVP.
* **Multi-Cluster Support**: Deferred. Focus entirely on single-cluster perfection first.
* **Cloud Provider Billing API Integration**: Deferred. We will use static cost estimations (e.g., $X per CPU core) rather than complex AWS/GCP API integrations.
