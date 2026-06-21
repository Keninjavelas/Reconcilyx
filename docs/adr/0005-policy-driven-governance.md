# ADR 005: Policy-Driven Governance
* **Context**: Not all namespaces or workloads should be treated equally. Production might require manual approval, while Dev can be fully autonomous.
* **Decision**: We introduce an `OptimizationPolicy` CRD that governs how Proposals are handled based on namespace or label selectors.
* **Rationale**: This separates the "Intelligence" (what should we do?) from the "Governance" (are we allowed to do it?). It makes the platform feel like a mature enterprise tool.
