# Portfolio Positioning

This document is designed to help articulate Reconcilyx's value, complexity, and engineering rigor to Staff/Principal-level Platform Engineers and technical hiring managers.

## 1. Elevator Pitch
"Reconcilyx is an autonomous Kubernetes right-sizing platform that safely closes the loop between observability and workload mutation. Unlike simple vertical pod autoscalers, it separates recommendation generation from execution using a strict, policy-driven state machine. By leveraging Just-In-Time validation, GitOps-aware Server-Side Apply, and deterministic execution snapshots, Reconcilyx guarantees safe, rollback-ready workload optimization without human intervention."

## 2. Architecture Summary
- **Data Plane (Telemetry):** Integrates with Prometheus to aggregate container-level CPU/Memory P95 usage over rolling windows.
- **Intelligence Plane (Engine):** Generates deterministic `OptimizationProposals` decoupled from the Kube-API, applying confidence and risk scores based on data volume and volatility.
- **Governance Plane (Policy):** Evaluates proposals against `OptimizationPolicies` to enforce strict Risk Tolerances, Cooldown Periods, and Workload Protections (e.g., sidecar exclusion).
- **Control Plane (Reconciler):** Drives a rigid 10-phase state machine utilizing an optimistic Execution Lock, JIT Validation Gates, and an unstructured Server-Side Apply patch strictly limited to `resources.requests`.

## 3. Engineering Challenges Solved

### A. The "GitOps Ping-Pong" Collision
**Problem:** Autonomous controllers mutating state will inherently fight ArgoCD/Flux self-healing loops, crashing the API server.
**Solution:** Designed a yielding Actuation strategy. Reconcilyx uses `client.Apply` with `Force: false`. If ArgoCD reverts the CPU limits, the controller detects the mismatch during its `Verifying` phase and immediately transitions to a `Failed: GitOpsOverride` state, breaking the loop permanently.

### B. The "Stale Snapshot" Rollback Hazard
**Problem:** Rolling back to the exact state observed when the telemetry was originally gathered will erase any manual `kubectl edit` modifications (like image tags) that an operator applied while the proposal was sitting in the approval queue.
**Solution:** Engineered a Just-In-Time `ExecutionSnapshot` captured precisely 1 millisecond before actuation. This captures the exact `ResourceVersion` and `Generation`, making rollbacks mathematically immune to stale data destruction.

## 4. Interview Deep-Dive Questions

**Q: "Why did you build your own Reconciler instead of using VPA?"**
A: "VPA is reactive and mutates pods aggressively on startup. Reconcilyx is preventative, policy-driven, and most importantly, provides a decoupled human-in-the-loop approval workflow when confidence scores are too low."

**Q: "How do you prevent a bad Prometheus query from destroying the cluster?"**
A: "Blast radius containment. First, the Policy Engine enforces a RiskTolerance ceiling. Second, the `Actuator` only has RBAC and schema permission to mutate `requests.cpu` and `requests.memory` via an unstructured SSA map. It physically cannot alter replica counts or limits in Phase 3A. Third, a global Kill-Switch ConfigMap exists to pause all actuations instantly."

**Q: "Explain how you handle concurrent Reconciler runs on the same Deployment."**
A: "I implemented optimistic locking using the `reconcilyx.io/execution-lock` annotation. When a proposal transitions to `Executing`, it attempts to acquire this lock. If another proposal's name is in that annotation, the Reconciler safely yields and requeues. This prevents two overlapping CPU reduction patches from stepping on each other's `ResourceVersion`."
