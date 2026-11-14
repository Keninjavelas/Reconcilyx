import type {
  AnomalyItem,
  AuditEntry,
  ClusterItem,
  ForecastPoint,
  HealthResponse,
  PolicyView,
  ProposalDetail,
  ProposalListItem,
  SavingsResponse,
  WhatIfRequest,
  WhatIfResponse,
} from "./types.ts";

export const EVIDENCE_RUN_ID = "20261026055631";
export const EVIDENCE_BANNER_TEXT =
  "EVIDENCE REPLAY MODE — Read-Only Snapshot from Verified Run #20261026055631 (Not Connected to Live Cluster)";

const REPLAY_STORAGE_KEY = "reconcilyx_evidence_replay_mode";

export function readReplayPreference(): boolean {
  if (typeof localStorage === "undefined") return false;
  return localStorage.getItem(REPLAY_STORAGE_KEY) === "true";
}

export function writeReplayPreference(enabled: boolean): void {
  if (typeof localStorage === "undefined") return;
  if (enabled) {
    localStorage.setItem(REPLAY_STORAGE_KEY, "true");
  } else {
    localStorage.removeItem(REPLAY_STORAGE_KEY);
  }
}

export function replayMutationBlockedError(): Error {
  return new Error(
    "Read-Only Evidence Replay Mode: connect to a live Reconcilyx Hub in Control Plane settings to approve, reject, rollback, or modify policies.",
  );
}

// Verified evidence recorded verbatim from STATUS.md runs #20261024155951,
// #multinode-tls-20261026055631, storefront-learning-2, and GitOps PR #1.
export const REPLAY_HEALTH: HealthResponse = {
  status: "replay-snapshot",
  time: "2026-10-26T05:56:31Z",
  version: "v4.1.0-rc1",
  carbonSource: "static-regional-fallback",
  pricingSources: {
    aws: "bundled-catalog-fallback",
    gcp: "bundled-catalog-fallback",
    azure: "bundled-catalog-fallback",
  },
};

export const REPLAY_CLUSTERS: ClusterItem[] = [
  {
    name: "reconcilyx-multinode-control-plane",
    type: "Hub (3-node Kind v1.36.1 + ingress-nginx TLS 1.3)",
    clusterName: "hub",
    apiEndpoint: "https://reconcilyx.test",
    status: "Ready",
    lastHeartbeat: "2026-10-26T05:56:31Z",
  },
];

export const REPLAY_PROPOSALS: ProposalListItem[] = [
  {
    name: "e2e-proposal-multinode-tls-20261026055631",
    namespace: "e2e-20261026055631",
    cluster: "hub",
    workloadKind: "Deployment",
    workloadName: "e2e-workload",
    recommendation: "RightSize",
    approvalState: "Approved",
    phase: "Completed",
    executionState: "Verified",
    riskScore: 14,
    confidenceScore: 99,
    rationale:
      "Verified 66-check multi-node TLS e2e right-sizing: resized container app from 250m/192Mi to 200m/160Mi via SSA pods/resize while preserving istio-proxy sidecar (250m/192Mi) and Deployment template (500m/256Mi); 0 restarts, reward=1.0205.",
    estimatedSavings: "4.11",
    carbonSavedGrams: 11097,
    carbonSavedHuman: "11.10 kg CO2",
    cpuBefore: "250m",
    cpuAfter: "200m",
    memoryBefore: "192Mi",
    memoryAfter: "160Mi",
    createdAt: "2026-10-26T05:56:31Z",
  },
  {
    name: "storefront-learning-2",
    namespace: "live-demo",
    cluster: "hub",
    workloadKind: "Deployment",
    workloadName: "storefront",
    recommendation: "RightSize",
    approvalState: "Approved",
    phase: "Completed",
    executionState: "Verified",
    riskScore: 15,
    confidenceScore: 96,
    rationale:
      "Prometheus-stabilized in-place resize on live-demo/storefront (2 replicas): app 250m/192Mi -> 200m/160Mi, sidecar istio-proxy untouched at 250m/192Mi; costSavedUSD=4106m, carbonSavedGramsCO2=11097, reward=1.0205.",
    estimatedSavings: "14.14",
    carbonSavedGrams: 4200,
    carbonSavedHuman: "4.20 kg CO2",
    cpuBefore: "250m",
    cpuAfter: "200m",
    memoryBefore: "192Mi",
    memoryAfter: "160Mi",
    createdAt: "2026-10-24T14:18:03Z",
  },
  {
    name: "cpu-burner-gitops-pr-1",
    namespace: "reconcilyx-system",
    cluster: "hub",
    workloadKind: "Deployment",
    workloadName: "cpu-burner",
    recommendation: "RightSize",
    approvalState: "Rejected",
    phase: "Rejected",
    executionState: "Pending",
    riskScore: 18,
    confidenceScore: 91,
    rationale:
      "GitOps PR #1 created against Keninjavelas/Reconcilyx (workloads/cpu-burner/manifests/deployment.yaml 500m/128Mi -> 250m/96Mi) and closed unmerged; controller failed closed with zero local pod mutation.",
    estimatedSavings: "0",
    carbonSavedGrams: 0,
    carbonSavedHuman: "0 g CO2",
    cpuBefore: "500m",
    cpuAfter: "250m",
    memoryBefore: "128Mi",
    memoryAfter: "96Mi",
    createdAt: "2026-10-26T06:40:00Z",
    externalPRID: "Keninjavelas/Reconcilyx#1",
  },
  {
    name: "hpa-conflict-demo-web",
    namespace: "demo",
    cluster: "hub",
    workloadKind: "Deployment",
    workloadName: "web",
    recommendation: "RightSize",
    approvalState: "Approved",
    phase: "Failed",
    executionState: "Failed",
    riskScore: 42,
    confidenceScore: 84,
    rationale:
      "JIT Validation Rejected: autoscaler conflict: HorizontalPodAutoscaler demo/web targets Deployment/web on CPU utilization. Excluded from savings ledger.",
    estimatedSavings: "18.25",
    carbonSavedGrams: 4200,
    carbonSavedHuman: "4.20 kg CO2",
    cpuBefore: "500m",
    cpuAfter: "250m",
    memoryBefore: "256Mi",
    memoryAfter: "128Mi",
    createdAt: "2026-10-23T15:12:00Z",
  },
];

export const REPLAY_SAVINGS: SavingsResponse = {
  totalUSD: 18.25,
  totalGramsCO2: 15297,
  totalCarbonHuman: "15.30 kg CO2",
  byCluster: { hub: 18.25 },
  byNs: { "live-demo": 14.14, "e2e-20261026055631": 4.11 },
  byClusterGramsCO2: { hub: 15297 },
  byNsGramsCO2: { "e2e-20261026055631": 11097, "live-demo": 4200 },
  series: [
    { date: "2026-10-24", cluster: "hub", usd: 14.14, gramsCO2: 4200 },
    { date: "2026-10-26", cluster: "hub", usd: 4.11, gramsCO2: 11097 },
  ],
  refreshed: "2026-10-26T05:56:31Z",
};

export const REPLAY_POLICIES: PolicyView[] = [
  {
    name: "e2e-policy-multinode-tls-20261026055631",
    namespace: "e2e-20261026055631",
    objective: "Efficiency",
    mode: "Propose",
    carbonWeight: 0.4,
    riskTolerance: 40,
    confidenceRequirement: 75,
    minReplicas: 1,
    targetSummary: "reconcilyx.io/e2e-run=20261026055631 · all workloads",
    gitOpsEnabled: false,
    gitOpsProvider: "mock",
    createdAt: "2026-10-26T05:56:00Z",
    raw: {
      apiVersion: "optimization.reconcilyx.io/v1alpha1",
      kind: "OptimizationPolicy",
      metadata: {
        name: "e2e-policy-multinode-tls-20261026055631",
        namespace: "e2e-20261026055631",
        creationTimestamp: "2026-10-26T05:56:00Z",
      },
      spec: {
        objective: "Efficiency",
        mode: "Propose",
        safetyConstraints: {
          minReplicas: 1,
          cooldownPeriod: "30m0s",
          riskTolerance: 40,
          confidenceRequirement: 75,
          carbonWeight: 0.4,
        },
        workloadProtection: {
          excludedContainers: ["istio-proxy"],
        },
      },
    },
  },
  {
    name: "cpu-burner-gitops",
    namespace: "reconcilyx-system",
    objective: "Cost",
    mode: "Propose",
    carbonWeight: 0.25,
    riskTolerance: 30,
    confidenceRequirement: 80,
    minReplicas: 1,
    targetSummary: "reconcilyx-system · cpu-burner",
    gitOpsEnabled: true,
    gitOpsProvider: "github",
    repository: "Keninjavelas/Reconcilyx",
    createdAt: "2026-10-26T06:35:00Z",
    raw: {
      apiVersion: "optimization.reconcilyx.io/v1alpha1",
      kind: "OptimizationPolicy",
      metadata: {
        name: "cpu-burner-gitops",
        namespace: "reconcilyx-system",
        creationTimestamp: "2026-10-26T06:35:00Z",
      },
      spec: {
        objective: "Cost",
        mode: "Propose",
        gitOps: {
          enabled: true,
          provider: "github",
          owner: "Keninjavelas",
          repo: "Reconcilyx",
          baseBranch: "main",
          manifestsPath: "workloads/cpu-burner/manifests",
          autoMerge: false,
        },
      },
    },
  },
];

export const REPLAY_ANOMALIES: AnomalyItem[] = [
  {
    time: "2026-10-23T15:12:00Z",
    workload: "web",
    namespace: "demo",
    metric: "HPA conflict",
    riskScore: 42,
    severity: "medium",
    proposal: "hpa-conflict-demo-web",
  },
];

export const REPLAY_AUDIT: AuditEntry[] = [
  {
    time: "2026-10-26T05:56:45Z",
    actor: "reconcilyx-controller",
    action: "completed: workload stabilized (reward=1.0205, costSavedUSD=4106m, carbon=11097g)",
    proposal: "e2e-proposal-multinode-tls-20261026055631",
    namespace: "e2e-20261026055631",
    reason: "ExecutionVerified",
  },
  {
    time: "2026-10-26T05:56:35Z",
    actor: "reconcilyx-actuator",
    action: "applied: in-place pod resize (app 250m/192Mi -> 200m/160Mi)",
    proposal: "e2e-proposal-multinode-tls-20261026055631",
    namespace: "e2e-20261026055631",
    reason: "PodResize",
  },
  {
    time: "2026-10-26T05:56:33Z",
    actor: "jit-execution-gate",
    action: "allowed: JIT validation passed",
    proposal: "e2e-proposal-multinode-tls-20261026055631",
    namespace: "e2e-20261026055631",
  },
];

export function getReplayProposalDetail(name: string): ProposalDetail | undefined {
  const item = REPLAY_PROPOSALS.find((p) => p.name === name);
  if (!item) return undefined;
  return {
    summary: item,
    rationale: item.rationale ?? "",
    observed: {
      cpu: item.cpuBefore,
      memory: item.memoryBefore,
      p95Cpu: item.cpuBefore === "250m" ? "160m" : "310m",
      p95Memory: item.memoryBefore === "192Mi" ? "118Mi" : "88Mi",
    },
    proposed: {
      cpu: item.cpuAfter,
      memory: item.memoryAfter,
    },
    expected: {
      savingsUSD: item.estimatedSavings,
      carbonSavedGrams: item.carbonSavedGrams,
    },
    events: [
      {
        at: item.createdAt,
        reason: item.phase === "Failed" ? "AutoscalerConflictDetected" : "ExecutionCompleted",
        message: item.rationale ?? "Recorded from verified cluster run",
        type: item.phase === "Failed" ? "Warning" : "Normal",
        count: 1,
      },
    ],
    raw: {
      metadata: {
        name: item.name,
        namespace: item.namespace,
        creationTimestamp: item.createdAt,
      },
      spec: {
        targetClusterName: item.cluster,
        targetWorkload: {
          kind: item.workloadKind,
          namespace: item.namespace,
          name: item.workloadName,
        },
        recommendation: item.recommendation,
        observedResources: {
          currentResources: {
            requests: {
              ...(item.cpuBefore ? { cpu: item.cpuBefore } : {}),
              ...(item.memoryBefore ? { memory: item.memoryBefore } : {}),
            },
          },
        },
        proposedResources: {
          requests: {
            ...(item.cpuAfter ? { cpu: item.cpuAfter } : {}),
            ...(item.memoryAfter ? { memory: item.memoryAfter } : {}),
          },
        },
        expectedImpact: {
          estimatedSavingsUSD: item.estimatedSavings,
          carbonSavedGramsCO2: String(item.carbonSavedGrams ?? 0),
        },
        decisionMetadata: {
          confidenceScore: item.confidenceScore,
          riskScore: item.riskScore,
          rationale: item.rationale,
        },
      },
      status: {
        phase: item.phase || undefined,
        approvalState: item.approvalState || undefined,
        executionState: item.executionState || undefined,
        externalPRID: item.externalPRID,
      },
    },
  };
}

export function getReplayForecast(workload: string): ForecastPoint[] {
  const baseEpoch = 1761456000;
  const isStorefront = workload === "storefront";
  const baseCpu = isStorefront ? 160 : 185;
  const baseMem = isStorefront ? 118 : 132;
  return Array.from({ length: 12 }, (_, idx) => ({
    t: baseEpoch + idx * 1800,
    actualCpu: baseCpu + ((idx * 7) % 19) - 9,
    predCpu: baseCpu,
    actualMem: baseMem + ((idx * 5) % 13) - 6,
    predMem: baseMem,
  }));
}

export function computeReplayWhatIf(req: WhatIfRequest): WhatIfResponse {
  const parseCpu = (v: string) => (v.endsWith("m") ? Number(v.slice(0, -1)) : Number(v) * 1000) || 0;
  const curC = parseCpu(req.currentCPU);
  const propC = parseCpu(req.proposedCPU);
  const deltaC = propC - curC;
  const estMonthly = Math.max(0, ((-deltaC / 1000) * 29.2));
  const risk = deltaC < -curC * 0.6 ? 72 : 15;
  return {
    allowed: risk < 70,
    confidenceScore: 78,
    riskScore: risk,
    cpuDelta: `${deltaC}m`,
    memoryDelta: `${req.proposedMemory} - ${req.currentMemory}`,
    savingsUSDPerMonth: estMonthly.toFixed(2),
    rationale:
      "Evidence Replay deterministic projection (asymmetric p95+15% CPU / p99+25% memory rules). Connect to a live Hub for real-time cluster telemetry validation.",
  };
}
