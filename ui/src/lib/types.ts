export type ApprovalState = "Pending" | "Approved" | "Rejected";

export type ProposalPhase =
  | "Detected"
  | "Evaluating"
  | "Approved"
  | "AwaitingExternalApproval"
  | "Rejected"
  | "Expired"
  | "Scheduled"
  | "Executing"
  | "Verifying"
  | "Stabilizing"
  | "Completed"
  | "Failed"
  | "RollbackRequired"
  | "RollingBack"
  | "RollbackVerification"
  | "RolledBack";

export type ExecutionState =
  | "Pending"
  | "Applying"
  | "Stabilizing"
  | "Verified"
  | "Reverted"
  | "Failed";

export type RecommendationType =
  | "RightSize"
  | "ScaleToZero"
  | "ReduceCPURequest"
  | "ReduceMemoryRequest"
  | "MigrateCloud";

export interface ProposalRef {
  name: string;
  namespace: string;
}

export interface ProposalListItem {
  name: string;
  namespace: string;
  cluster?: string;
  workloadKind: string;
  workloadName: string;
  recommendation: RecommendationType;
  approvalState: ApprovalState | "";
  phase: ProposalPhase | "";
  executionState?: ExecutionState | "";
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

export interface MigrationPlan {
  sourceCluster?: string;
  targetCloud: string;
  targetRegion: string;
  targetInstance: string;
  costDeltaUSD: number;
  carbonSavedGrams: number;
  migrationScript?: string;
}

export interface RawProposal {
  metadata: {
    name: string;
    namespace: string;
    creationTimestamp?: string;
    resourceVersion?: string;
    generation?: number;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
  };
  spec: {
    targetClusterName?: string;
    targetWorkload: {
      apiVersion?: string;
      kind: string;
      namespace?: string;
      name: string;
    };
    recommendation: RecommendationType;
    observedResources?: {
      currentResources?: {
        requests?: Record<string, string>;
        limits?: Record<string, string>;
      };
      p95Usage?: Record<string, string>;
    };
    proposedResources?: {
      requests?: Record<string, string>;
      limits?: Record<string, string>;
    };
    expectedImpact?: {
      resourceDelta?: Record<string, string>;
      estimatedSavingsUSD?: string;
      carbonSavedGramsCO2?: string;
    };
    decisionMetadata?: {
      confidenceScore: number;
      riskScore: number;
      rationale?: string;
    };
    migrationPlan?: MigrationPlan;
  };
  status?: {
    phase?: ProposalPhase;
    approvalState?: ApprovalState;
    executionState?: ExecutionState;
    policyApplied?: string;
    lastEvaluationTime?: string;
    expirationTime?: string;
    reason?: string;
    executionSnapshot?: {
      resourceVersion: string;
      generation: number;
      containerResources: Record<string, { resources?: unknown }>;
      replicaCount: number;
      annotations?: Record<string, string>;
      timestamp: string;
    };
    executionMetadata?: {
      executionID: string;
      fieldManager: string;
      policyVersion: string;
      proposalVersion: string;
      telemetryTimestamp: string;
    };
    conditions?: {
      type: string;
      status: string;
      reason?: string;
      message?: string;
      lastTransitionTime?: string;
    }[];
    yieldCount?: number;
    externalPRID?: string;
  };
}

export interface RawProposalEvent {
  metadata?: { name?: string; creationTimestamp?: string };
  type?: string;
  reason?: string;
  message?: string;
  count?: number;
  firstTimestamp?: string;
  lastTimestamp?: string;
  eventTime?: string;
}

export interface ProposalTimelineEvent {
  at: string;
  reason: string;
  message: string;
  type: "Normal" | "Warning";
  count?: number;
}

export interface ProposalDetail {
  summary: ProposalListItem;
  raw: RawProposal;
  rationale: string;
  events: ProposalTimelineEvent[];
  observed: {
    cpu?: string;
    memory?: string;
    p95Cpu?: string;
    p95Memory?: string;
  };
  proposed: {
    cpu?: string;
    memory?: string;
  };
  expected: {
    savingsUSD?: string;
    carbonSavedGrams?: number;
    resourceDelta?: Record<string, string>;
  };
  migrationPlan?: MigrationPlan;
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

export type PolicyObjective = "Cost" | "Performance" | "Efficiency";
export type PolicyMode = "Observe" | "Propose" | "AutoApply";
export type GitProvider = "mock" | "github" | "gitlab";

export interface LabelSelector {
  matchLabels?: Record<string, string>;
  matchExpressions?: { key: string; operator: string; values?: string[] }[];
}

export interface RawPolicySpec {
  objective: PolicyObjective;
  mode: PolicyMode;
  targetSelectors?: {
    namespaceSelector?: LabelSelector | null;
    workloadSelector?: LabelSelector | null;
  };
  resourceConstraints?: {
    minCPU?: string;
    maxCPU?: string;
    minMemory?: string;
    maxMemory?: string;
  };
  safetyConstraints?: {
    minReplicas?: number;
    maxLatencyIncreaseMs?: number;
    cooldownPeriod?: string;
    riskTolerance?: number;
    confidenceRequirement?: number;
    carbonWeight?: number;
  };
  rollbackConstraints?: {
    errorRateThresholdPercent?: number;
    oomKilledWindow?: string;
  };
  recommendationGovernance?: {
    allowedRecommendations?: RecommendationType[];
  };
  workloadProtection?: {
    excludedContainers?: string[];
  };
  gitOps?: {
    enabled?: boolean;
    provider?: GitProvider;
    owner?: string;
    repo?: string;
    baseBranch?: string;
    manifestsPath?: string;
    autoMerge?: boolean;
  };
}

export interface RawPolicy {
  apiVersion?: string;
  kind?: string;
  metadata: {
    name: string;
    namespace: string;
    creationTimestamp?: string;
  };
  spec: RawPolicySpec;
}

export interface PolicyView {
  name: string;
  namespace: string;
  objective: PolicyObjective;
  mode: PolicyMode;
  carbonWeight: number;
  riskTolerance?: number;
  confidenceRequirement?: number;
  minReplicas?: number;
  targetSummary: string;
  gitOpsEnabled: boolean;
  gitOpsProvider: GitProvider;
  repository?: string;
  createdAt?: string;
  raw: RawPolicy;
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

export interface HealthResponse {
  status: string;
  time: string;
}
