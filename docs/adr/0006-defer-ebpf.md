# ADR 006: eBPF is Deferred from MVP
* **Context**: eBPF provides incredible kernel-level observability (network flows, precise latency), which would make the intelligence engine extremely powerful.
* **Decision**: We will intentionally defer eBPF integration and rely strictly on Prometheus/cAdvisor metrics for the MVP.
* **Rationale**: Adding eBPF (e.g., Cilium or Pixie) drastically increases the complexity of the local setup and the learning curve. We can prove the core thesis (Observe -> Propose -> Reconcile) with standard metrics first. eBPF remains a strong Phase 4 roadmap item.
