# Reconcilyx Repository Structure

The repository follows standard Go project layout conventions, optimized for a Kubernetes Operator and a modern web frontend.

## Final Repository Layout

```text
reconcilyx/
├── api/                        # Kubernetes CRD API definitions
│   └── v1alpha1/               # OptimizationPolicy, OptimizationProposal schemas
├── bin/                        # Compiled binaries (gitignored)
├── cmd/                        # Application entry points
│   └── operator/               # Main `main.go` for the Reconcilyx Operator
├── config/                     # Kubebuilder auto-generated manifests (CRDs, RBAC)
├── docs/                       # Architecture, design, and roadmap documentation
├── internal/                   # Private application code
│   ├── engine/                 # Optimization intelligence (PromQL queries, analysis)
│   ├── controller/             # Kubernetes reconciliation loops (The Operator logic)
│   ├── policy/                 # Governance evaluation logic
│   └── web/                    # Internal API server serving the frontend dashboard
├── pkg/                        # Public libraries (if any, e.g., external API clients)
├── simulators/                 # Workload generators to create cluster chaos
│   ├── cpu-burner/             
│   ├── idle-service/           
│   └── mem-leaker/             
├── ui/                         # React/TypeScript dashboard frontend
├── hack/                       # Scripts for development (e.g., kind-cluster-setup.sh)
├── Makefile                    # Core build and deploy automation
├── Dockerfile.operator         # Container build for the Operator
└── README.md                   # Project landing page and quickstart
```

## Ownership Boundaries & Service Responsibilities

To avoid distributed system complexity, **everything outside of the `ui/` and `simulators/` folders compiles into a single Go binary**.

* **The Operator Binary**: Responsible for intelligence, policy, reconciliation, and serving the API for the UI. It owns the logic.
* **The UI**: Purely a presentation layer. It does not contain optimization logic; it only reads/writes K8s Custom Resources via the Operator's API. It owns the UX.
* **The Simulators**: Entirely decoupled. They do not know Reconcilyx exists. They simply generate load and export metrics. They own the chaos.

## Infrastructure Boundaries

* **Metrics Engine**: Reconcilyx does *not* scrape its own metrics from pods. It delegates this entirely to Prometheus. Reconcilyx is a *consumer* of telemetry.
* **Storage**: Reconcilyx does *not* maintain a stateful database initially. The Kubernetes `etcd` cluster (via CRDs) is the source of truth for Proposals and Policies.

## Documentation Strategy
* **`/docs`**: High-level architecture, ADRs, and system design.
* **Code Comments**: Godoc standard for all exported functions.
* **README**: Focused entirely on the "Elevator Pitch", Architecture Diagram, and "How to run locally in 5 minutes".
