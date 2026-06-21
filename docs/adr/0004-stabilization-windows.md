# ADR 004: Stabilization Windows
* **Context**: If a workload is scaled down and immediately spikes in CPU, a naive system might immediately scale it back up, causing thrashing (flapping).
* **Decision**: Every applied Proposal triggers a strict stabilization window (e.g., 15 minutes) for the target workload.
* **Rationale**: Safety first. Workloads need time to settle after a resource change. During this window, no new optimization proposals will be generated for that workload unless it is a critical rollback.
