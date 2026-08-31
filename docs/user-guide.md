# User Guide — Reconcilyx v4.0

This is the end-to-end operator's manual for Reconcilyx v4.0: The Autonomous FinOps Agent. It covers installation, configuration, approval workflows, carbon-awareness, federated learning, BYOK ChatOps, Shift-Left PR checks, Karpenter node consolidation, real-time eBPF streaming, and Model Registry canary rollouts.

---

## 1. Installation

### 1.1. Helm Chart (Enterprise Standard)

```bash
helm repo add reconcilyx https://charts.reconcilyx.io
helm upgrade --install reconcilyx reconcilyx/reconcilyx \
  --namespace reconcilyx-system --create-namespace \
  --set llm.provider="ollama" \
  --set pricing.providers.aws.enabled=true \
  --set federated.role="hub"
```

### 1.2. CLI Plugin Installation (`kubectl-reconcilyx`)

```bash
# Build the CLI binary
make build-cli

# Verify installation
.\bin\kubectl-reconcilyx.exe --help
```

---

## 2. Using the 9 Pillars of Reconcilyx v4.0

### 2.1. In-Place Pod Resizing (Zero Pod Restarts)
Reconcilyx automatically updates running container CPU and Memory allocations without triggering pod restarts using Kubernetes `InPlacePodVerticalScaling`.

### 2.2. Pillar 1: Byzantine-Resistant Federated Learning
- **Hub Mode**: Set `federated.role=hub` to aggregate incoming Q-deltas from spoke clusters using Trimmed Mean outlier rejection.
- **Spoke Mode**: Set `federated.role=spoke` and `federated.hubURL=https://hub.example.com`. Newly registered spokes bootstrap to 100% efficiency in <1 minute.

### 2.3. Pillar 2: Multi-Cloud Arbitrage
Evaluates real-time price & carbon matrix across AWS, GCP, and Azure. Generates `MigrateCloud` proposals with executable Velero cross-cloud backup & restore scripts.

### 2.4. Pillar 3: Carbon & Application Intelligence
- Tracks gCO₂e saved per proposal.
- Automatically rewrites JVM container arguments (`-Xmx` tuned to 75% of new memory limit).
- Profiles NVIDIA DCGM GPU telemetry (`DCGM_FI_DEV_GPU_UTIL`, `DCGM_FI_DEV_FB_USED`).

### 2.5. Pillar 4: BYOK ChatOps Natural Language Assistant
Connect to your choice of LLM provider:
```bash
# Interactive CLI ChatOps with Ollama (local / zero-cloud)
.\bin\kubectl-reconcilyx.exe chat --provider ollama

# Interactive CLI ChatOps with OpenAI
.\bin\kubectl-reconcilyx.exe chat --provider openai --llm-key "sk-..."
```

### 2.6. Pillar 5: Shift-Left FinOps (Pre-Merge PR Checks)
Integrates with GitHub Actions / GitLab CI to audit proposed Kubernetes manifests in pull requests before code is merged. Posts Markdown comments with cost ($/mo) and carbon (kg CO₂e) diffs.

### 2.7. Pillar 6: Agentic AI Actionable LLM
Execute write commands directly via chat:
```text
reconcilyx-ai > apply proposal prop-nginx-102
reconcilyx-ai > rollback workload payment-service
```

### 2.8. Pillar 7: Live Node Optimization & Karpenter Consolidation
Monitors node capacity utilization post-resizing. Automatically generates `NodeConsolidationProposal` and patches Karpenter `NodeClaim` resources when node utilization drops below 40%.

### 2.9. Pillar 8: Real-Time eBPF Telemetry Streaming
Stream sub-second 200ms kernel telemetry via `EBPFMetricsStreamer` for instant reactive anomaly throttling before OOM events occur.

### 2.10. Pillar 9: Model Registry & Canary A/B Testing
Stores versioned global models (`v42`, `v43`). Canary opt-in clusters receive `v43` while stable clusters remain on `v42`. Automatically triggers rollback if canary RL performance degrades.
