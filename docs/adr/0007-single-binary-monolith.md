# ADR 007: Single Binary Monolith Control Plane
* **Context**: The original plan hinted at an Optimization Engine, a Policy Engine, and a Reconciliation Operator as potentially distinct components.
* **Decision**: All control plane components will be compiled into a single Golang binary.
* **Rationale**: Avoids distributed systems overhead (networking, complex deployments, separate logging). Go routines and modular package design provide enough separation of concerns without the operational tax of microservices.
