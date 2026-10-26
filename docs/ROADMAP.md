# Reconcilyx Product & Engineering Roadmap

> **Project status: on hiatus (2026-10-26).** The completed milestones below describe prior work. Remaining work and resume criteria are tracked in [HIATUS.md](../HIATUS.md); this roadmap is not being updated during the hiatus.

## Released Milestone: v3.0.0 "Intelligence Fabric" (COMPLETED)

All four novel pillars and core platform features are fully implemented, integrated, and verified against the automated production test suite:

### ✅ Phase 0: Core Engine, In-Place Resizing & Distribution
- **In-Place Pod Resizing (`InPlacePodVerticalScaling`)**: Zero-downtime CPU/Memory resizing without pod restarts.
- **Q-Learning RL Agent**: Multi-objective reinforcement learning agent with sigmoid-normalized cost & carbon reward signals.
- **React FinOps Dashboard & `kubectl-reconcilyx` CLI**: Web UI and Go CLI plugin for cluster scanning and proposal approvals.
- **Helm v3 Package Installer**: Production-grade `charts/reconcilyx` package.

### ✅ Phase 1: Byzantine-Resistant Federated Learning (Pillar 1)
- **Trimmed Mean Aggregation**: Discards outlier Q-deltas ($|\Delta Q| > 5.0$) and trims top/bottom 10% across reporting clusters to prevent rogue cluster poisoning.
- **Model Hashes & Convergence Metrics**: SHA256 model hashing, `VersionStr`, and `IsStable` flag enabling zero-history spoke clusters to reach 100% efficiency in <1 minute.

### ✅ Phase 2: Multi-Cloud Arbitrage (Pillar 2)
- **Multi-Cloud Price & Carbon Matrix**: Query providers for AWS, GCP, and Azure real-time spot and on-demand rates.
- **`MigrateCloud` Proposals & Velero Scripts**: Automatically generates `velero backup create` and `velero restore create` cross-cloud migration scripts.

### ✅ Phase 3: Application-Layer Intelligence (Pillar 3)
- **JVM Heap Tuning**: Parses `-Xmx`/`-Xms` from container command/args and dynamically rewrites `-Xmx` to 75% of new memory limit in GitOps diffs.
- **NVIDIA DCGM GPU Profiling**: Scrapes Prometheus DCGM telemetry (`DCGM_FI_DEV_GPU_UTIL`, `DCGM_FI_DEV_FB_USED`, `DCGM_FI_DEV_TEMP`) and penalizes over-provisioned GPU requests (<20% utilization).

### ✅ Phase 4: BYOK ChatOps Natural Language Assistant (Pillar 4)
- **Multi-Provider LLM Layer**: Pluggable provider factory supporting Ollama, OpenAI, Anthropic, Groq, and Cohere.
- **15 Function Tool Schemas & Hybrid Router**: Fast-path regex pattern matching (<100ms) with LLM function-calling fallback.
- **React & CLI Chat Interfaces**: Embedded ChatOps tab in web UI and interactive `kubectl reconcilyx chat` CLI command.

### ✅ Phase 5: Unified Integration & Release Gauntlet (COMPLETED)
- **Multi-Cluster E2E Test Suite**: Multi-pillar pipeline assertion (`hack/e2e/e2e_test.go`).
- **Cold-Start Efficiency Benchmark**: Verified 100% efficiency in <1 minute (`internal/federated/coldstart_test.go`).

---

## Future Roadmap (v4.0+)

- **Cluster-API Automated Cloud Switching**: Native Cluster-API machine pool orchestration for automated `MigrateCloud` actuation.
- **eBPF Telemetry Facade**: Optional Cilium / Hubble eBPF telemetry plugin for kernel-level socket throughput profiling.
