# ADR 003: Proposal-Based Optimization
* **Context**: Automated systems that mutate infrastructure are dangerous and often rejected by operations teams.
* **Decision**: The system will default to creating `OptimizationProposal` Custom Resources rather than immediately applying changes.
* **Rationale**: This enables a "human-in-the-loop" workflow. It provides auditability, allows for dry-runs, and builds trust. Even when auto-apply is enabled via policy, the intermediate Proposal CR serves as a historical record of *why* a change was made.
