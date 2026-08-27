export type ApprovalState =
  | "Pending"
  | "Approved"
  | "Rejected"
  | "AutoApproved";

export type ProposalPhase =
  | "Detected"
  | "Evaluating"
  | "Approved"
  | "AwaitingExternalApproval"
  | "AwaitingHumanApproval"
  | "Scheduled"
  | "Executing"
  | "Verifying"
  | "Stabilizing"
  | "Completed"
  | "Rollback"
  | "Rejected";

export type ExecutionState =
  | "None"
  | "Planned"
  | "InProgress"
  | "Succeeded"
  | "Failed"
  | "RolledBack";

export type RecommendationType =
  | "RightSize"
  | "RightSizeCPU"
  | "RightSizeMemory"
  | "Idle"
  | "ScaleDownReplicas"
  | "NoChange";

export interface ProposalListItem {
  name: string;
  namespace: string;
  cluster?: string;
  workloadKind: string;
  workloadName: string;
  recommendation: RecommendationType;
  approvalState: ApprovalState;
  phase: ProposalPhase;
  executionState?: ExecutionState;
  riskScore: number;
  confidenceScore: number;
  rationale?: string;
  estimatedSavings?: string;
  carbonSavedGrams?: number;
  carbonSavedHuman?: string;
  cpuBefore?: string;
  cpuAfter?: string;
  memoryBefore?: string;
  memoryAfter?: string;
  createdAt: string;
  externalPRID?: string;
}

export interface SavingsPoint {
  date: string;
  cluster?: string;
  usd: number;
  gramsCO2?: number;
}

export interface SavingsResponse {
  totalUSD: number;
  totalGramsCO2?: number;
  totalCarbonHuman?: string;
  byCluster: Record<string, number>;
  byNs: Record<string, number>;
  byClusterGramsCO2?: Record<string, number>;
  byNsGramsCO2?: Record<string, number>;
  series: SavingsPoint[];
  refreshed: string;
}

export interface OptimizationPolicySpec {
  mode?: "Observe" | "Propose" | "AutoApply" | "ManualApproval";
  targetSelectors?: {
    namespaceSelector?: { matchLabels?: Record<string, string> };
    workloadSelector?: { matchLabels?: Record<string, string> };
  };
  safetyConstraints?: {
    carbonWeight?: number;
    maxCPURequestIncreasePct?: number;
    maxMemoryRequestIncreasePct?: number;
    stabilizationWindowSeconds?: number;
  };
  analysis?: {
    minConfidenceScore?: number;
    maxRiskScore?: number;
    minReplicas?: number;
  };
  gitOps?: {
    enabled?: boolean;
    provider?: "mock" | "github" | "gitlab";
    autoMerge?: boolean;
  };
}

export interface PolicyListItem {
  name: string;
  namespace: string;
  mode: string;
  workloadsMatched: number;
  proposalsActive: number;
  carbonWeight: number;
  createdAt: string;
}

export interface ForecastPoint {
  t: number;
  actualCpu?: number;
  predCpu?: number;
  actualMem?: number;
  predMem?: number;
}

export interface ForecastResponse {
  workload: string;
  namespace: string;
  series: ForecastPoint[];
}

export interface ProposalDetail {
  summary: ProposalListItem;
  rationale: string;
  predictedImpact: {
    forecastConfidence: number;
    anomalySeverity: number;
    rlReward: number;
    ruleHits: string[];
  };
  gitOps?: {
    provider: string;
    prID?: string;
    prURL?: string;
    mergeStatus?: string;
  };
  execution?: {
    snapshots: { before: string; after: string };
    events: { at: string; message: string; level: "info" | "warn" | "error" }[];
  };
}

export interface ChatResponse {
  answer: string;
  function?: string;
  fastPath?: boolean;
  chartData?: Record<string, unknown>;
  confirmationRequired?: boolean;
  confirmationToken?: string;
  confirmationDescription?: string;
  actionStatus?: string;
  actionTarget?: string;
}

export interface WhatIfRequest {
  namespace: string;
  workloadName: string;
  workloadKind?: string;
  currentCPU: string;
  currentMemory: string;
  proposedCPU: string;
  proposedMemory: string;
  p95CPU?: string;
  p95Memory?: string;
}

export interface WhatIfResponse {
  allowed: boolean;
  confidenceScore: number;
  riskScore: number;
  cpuDelta: string;
  memoryDelta: string;
  savingsUSDPerMonth: string;
  rationale: string;
}

export interface AnomalyItem {
  time: string;
  workload: string;
  namespace: string;
  metric: string;
  riskScore: number;
  severity: "low" | "medium" | "high";
  proposal?: string;
}

export interface ClusterItem {
  name: string;
  type: string;
  clusterName?: string;
  apiEndpoint?: string;
  status: string;
  lastHeartbeat?: string;
}

export interface AuditEntry {
  time: string;
  actor: string;
  action: string;
  proposal?: string;
  namespace?: string;
  reason?: string;
}
