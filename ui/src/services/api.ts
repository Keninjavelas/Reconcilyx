export interface Proposal {
  name: string;
  namespace: string;
  cluster?: string;
  workloadKind: string;
  workloadName: string;
  recommendation: string;
  approvalState: "Pending" | "Approved" | "Rejected" | "AutoApproved";
  phase: string;
  executionState?: string;
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

export interface SavingsMetrics {
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

export interface Policy {
  name: string;
  namespace: string;
  mode?: string;
  [key: string]: unknown;
}

export interface ForecastPoint {
  t: number;
  actualCpu?: number;
  predCpu?: number;
  actualMem?: number;
  predMem?: number;
}

const API_BASE_URL = import.meta.env.VITE_API_URL || "";

async function fetchJSON<T>(endpoint: string, options?: RequestInit): Promise<T> {
  const apiKey = localStorage.getItem("reconcilyx_api_key") || "";
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    Accept: "application/json",
    ...(apiKey ? { "X-API-Key": apiKey } : {}),
  };

  const res = await fetch(`${API_BASE_URL}${endpoint}`, { ...options, headers });
  if (!res.ok) {
    const body = await res.text().catch(() => "");
    throw new Error(`HTTP ${res.status}: ${res.statusText} — ${body.slice(0, 200)}`);
  }
  return (await res.json()) as T;
}

type ListEnvelope<T> = { items: T[]; total: number };

export const api = {
  getProposals: async (): Promise<Proposal[]> => {
    const env = await fetchJSON<ListEnvelope<Proposal>>("/api/proposals");
    return env.items ?? [];
  },

  approveProposal: async (name: string, reason = "Approved via API"): Promise<void> => {
    await fetchJSON(`/api/proposals/${encodeURIComponent(name)}/approve`, {
      method: "POST",
      body: JSON.stringify({ reason }),
    });
  },

  rejectProposal: async (name: string, reason = "Rejected via API"): Promise<void> => {
    await fetchJSON(`/api/proposals/${encodeURIComponent(name)}/reject`, {
      method: "POST",
      body: JSON.stringify({ reason }),
    });
  },

  rollbackProposal: async (name: string, reason = "Rollback requested via API"): Promise<void> => {
    await fetchJSON(`/api/proposals/${encodeURIComponent(name)}/rollback`, {
      method: "POST",
      body: JSON.stringify({ reason }),
    });
  },

  getSavingsMetrics: async (): Promise<SavingsMetrics> => {
    return await fetchJSON<SavingsMetrics>("/api/metrics/savings");
  },

  getPolicies: async (): Promise<Policy[]> => {
    const env = await fetchJSON<ListEnvelope<Policy>>("/api/policies");
    return env.items ?? [];
  },

  getForecast: async (workload: string, namespace = "default"): Promise<ForecastPoint[]> => {
    const env = await fetchJSON<{ workload: string; namespace: string; series: ForecastPoint[] }>(
      `/api/forecast/${encodeURIComponent(workload)}?namespace=${encodeURIComponent(namespace)}`,
    );
    return env.series ?? [];
  },
};
