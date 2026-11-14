import type {
  AnomalyItem,
  AuditEntry,
  ChatResponse,
  ClusterItem,
  ForecastPoint,
  ForecastResponse,
  HealthResponse,
  LabelSelector,
  PolicyView,
  ProposalDetail,
  ProposalListItem,
  ProposalRef,
  ProposalTimelineEvent,
  RawPolicy,
  RawProposal,
  RawProposalEvent,
  SavingsResponse,
  WhatIfRequest,
  WhatIfResponse,
} from "./types";
import {
  REPLAY_ANOMALIES,
  REPLAY_AUDIT,
  REPLAY_CLUSTERS,
  REPLAY_HEALTH,
  REPLAY_POLICIES,
  REPLAY_PROPOSALS,
  REPLAY_SAVINGS,
  computeReplayWhatIf,
  getReplayForecast,
  getReplayProposalDetail,
  readReplayPreference,
  replayMutationBlockedError,
} from "./evidenceReplay";

const apiUrlStorageKey = "reconcilyx_api_url";
const apiKeyStorageKey = "reconcilyx_api_key";
const configuredBuildApiUrl =
  (import.meta.env.VITE_API_BASE as string | undefined) ??
  (import.meta.env.VITE_API_URL as string | undefined) ??
  "";
const configuredBuildApiKey = (import.meta.env.VITE_API_KEY as string | undefined) ?? "";

function normalizeApiUrl(value: string): string {
  const trimmed = value.trim();
  if (!trimmed) return "";

  const url = new URL(trimmed);
  if (url.protocol !== "http:" && url.protocol !== "https:") {
    throw new Error("API URL must use http or https");
  }
  return url.toString().replace(/\/$/, "");
}

export function getApiBaseUrl(): string {
  try {
    return normalizeApiUrl(localStorage.getItem(apiUrlStorageKey) ?? configuredBuildApiUrl);
  } catch {
    return normalizeApiUrl(configuredBuildApiUrl);
  }
}

export function setApiBaseUrl(value: string): string {
  const normalized = normalizeApiUrl(value);
  if (normalized) {
    localStorage.setItem(apiUrlStorageKey, normalized);
  } else {
    localStorage.removeItem(apiUrlStorageKey);
  }
  window.dispatchEvent(new Event("reconcilyx-api-url-changed"));
  return normalized;
}

export function getApiKey(): string {
  return localStorage.getItem(apiKeyStorageKey) ?? configuredBuildApiKey;
}

export function setApiKey(value: string): void {
  if (value) {
    localStorage.setItem(apiKeyStorageKey, value);
  } else {
    localStorage.removeItem(apiKeyStorageKey);
  }
  window.dispatchEvent(new Event("reconcilyx-api-url-changed"));
}

type FetchOpts = RequestInit & { params?: Record<string, string | number | boolean | undefined> };

async function json<T>(path: string, opts: FetchOpts = {}): Promise<T> {
  const url = new URL(path, getApiBaseUrl() || window.location.origin);
  if (opts.params) {
    for (const [k, v] of Object.entries(opts.params)) {
      if (v !== undefined && v !== null && v !== "") url.searchParams.set(k, String(v));
    }
  }
  const headers = new Headers(opts.headers ?? {});
  headers.set("Accept", "application/json");
  const apiKey = getApiKey();
  if (apiKey) headers.set("X-API-Key", apiKey);
  if (opts.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  const res = await fetch(url.toString(), { ...opts, headers });
  if (!res.ok) {
    const body = await res.text().catch(() => "");
    throw new Error(`HTTP ${res.status} ${res.statusText} — ${body.slice(0, 240)}`);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

type ListEnvelope<T> = { items: T[]; total: number };

function asString(value: string | number | undefined): string | undefined {
  if (value === undefined || value === null || value === "") return undefined;
  return String(value);
}

function asNumber(value: string | number | undefined): number | undefined {
  if (value === undefined || value === null || value === "") return undefined;
  const n = Number(value);
  return Number.isFinite(n) ? n : undefined;
}

function formatCarbonHuman(grams?: number): string | undefined {
  if (grams === undefined || grams <= 0) return undefined;
  if (grams >= 1_000_000) return `${(grams / 1_000_000).toFixed(2)} tonnes CO₂`;
  if (grams >= 1000) return `${(grams / 1000).toFixed(1)} kg CO₂`;
  return `${Math.round(grams)} g CO₂`;
}

function toProposalListItem(p: RawProposal): ProposalListItem {
  const spec = p.spec;
  const before = spec.observedResources?.currentResources?.requests ?? {};
  const after = spec.proposedResources?.requests ?? {};
  const impact = spec.expectedImpact ?? {};
  const carbon = asNumber(impact.carbonSavedGramsCO2);

  return {
    name: p.metadata.name,
    namespace: p.metadata.namespace || spec.targetWorkload?.namespace || "default",
    cluster: spec.targetClusterName,
    workloadKind: spec.targetWorkload?.kind || "Workload",
    workloadName: spec.targetWorkload?.name || p.metadata.name,
    recommendation: spec.recommendation,
    approvalState: p.status?.approvalState ?? "",
    phase: p.status?.phase ?? "",
    executionState: p.status?.executionState ?? "",
    riskScore: spec.decisionMetadata?.riskScore ?? 0,
    confidenceScore: spec.decisionMetadata?.confidenceScore ?? 0,
    rationale: spec.decisionMetadata?.rationale,
    estimatedSavings: asString(impact.estimatedSavingsUSD),
    carbonSavedGrams: carbon,
    carbonSavedHuman: formatCarbonHuman(carbon),
    cpuBefore: asString(before.cpu),
    cpuAfter: asString(after.cpu),
    memoryBefore: asString(before.memory),
    memoryAfter: asString(after.memory),
    createdAt: p.metadata.creationTimestamp ?? new Date().toISOString(),
    externalPRID: p.status?.externalPRID,
  };
}

function eventTime(e: RawProposalEvent): string {
  return (
    e.lastTimestamp ??
    e.eventTime ??
    e.firstTimestamp ??
    e.metadata?.creationTimestamp ??
    new Date().toISOString()
  );
}

function toTimelineEvent(e: RawProposalEvent): ProposalTimelineEvent {
  return {
    at: eventTime(e),
    reason: e.reason ?? "Event",
    message: e.message ?? "",
    type: e.type === "Warning" ? "Warning" : "Normal",
    count: e.count,
  };
}

function selectorLabels(selector?: LabelSelector | null): string[] {
  if (!selector?.matchLabels) return [];
  return Object.entries(selector.matchLabels).map(([key, value]) =>
    key === "kubernetes.io/metadata.name" ? value : `${key}=${value}`,
  );
}

function policyTargetSummary(policy: RawPolicy): string {
  const nsLabels = selectorLabels(policy.spec.targetSelectors?.namespaceSelector);
  const workloadLabels = selectorLabels(policy.spec.targetSelectors?.workloadSelector);
  const ns = nsLabels.length > 0 ? nsLabels.join(", ") : "all namespaces";
  const workload = workloadLabels.length > 0 ? workloadLabels.join(", ") : "all workloads";
  return `${ns} · ${workload}`;
}

function toPolicyView(p: RawPolicy): PolicyView {
  const spec = p.spec;
  const gitOps = spec.gitOps ?? {};
  const repository =
    gitOps.owner && gitOps.repo ? `${gitOps.owner}/${gitOps.repo}` : gitOps.repo || undefined;

  return {
    name: p.metadata.name,
    namespace: p.metadata.namespace || "default",
    objective: spec.objective,
    mode: spec.mode,
    carbonWeight: spec.safetyConstraints?.carbonWeight ?? 0,
    riskTolerance: spec.safetyConstraints?.riskTolerance,
    confidenceRequirement: spec.safetyConstraints?.confidenceRequirement,
    minReplicas: spec.safetyConstraints?.minReplicas,
    targetSummary: policyTargetSummary(p),
    gitOpsEnabled: Boolean(gitOps.enabled),
    gitOpsProvider: gitOps.provider ?? "mock",
    repository,
    createdAt: p.metadata.creationTimestamp,
    raw: p,
  };
}

export const api = {
  proposals: {
    list(params?: {
      phase?: string;
      approvalState?: string;
      namespace?: string;
      cluster?: string;
      workload?: string;
      sortBy?: string;
      limit?: number;
    }): Promise<ProposalListItem[]> {
      if (readReplayPreference()) {
        const items = REPLAY_PROPOSALS.filter((p) => {
          if (params?.phase && p.phase !== params.phase) return false;
          if (params?.approvalState && p.approvalState !== params.approvalState) return false;
          if (params?.namespace && p.namespace !== params.namespace) return false;
          if (params?.cluster && p.cluster !== params.cluster) return false;
          if (params?.workload && p.workloadName !== params.workload) return false;
          return true;
        });
        return Promise.resolve(items.slice(0, params?.limit ?? items.length));
      }
      return json<ListEnvelope<ProposalListItem>>("/api/proposals", { params }).then(
        (env) => env.items ?? [],
      );
    },
    detail(name: string, namespace = "default"): Promise<ProposalDetail> {
      if (readReplayPreference()) {
        const detail = getReplayProposalDetail(name);
        if (!detail) {
          return Promise.reject(new Error(`Proposal ${namespace}/${name} not found in replay snapshot`));
        }
        return Promise.resolve(detail);
      }
      return json<{ proposal: RawProposal; events?: RawProposalEvent[] }>(
        `/api/proposals/${encodeURIComponent(name)}`,
        { params: { namespace } },
      ).then((env) => {
        const raw = env.proposal;
        const spec = raw.spec;
        const observed = spec.observedResources?.currentResources?.requests ?? {};
        const p95 = spec.observedResources?.p95Usage ?? {};
        const proposed = spec.proposedResources?.requests ?? {};
        const impact = spec.expectedImpact ?? {};
        const events = (env.events ?? [])
          .map(toTimelineEvent)
          .sort((a, b) => new Date(a.at).getTime() - new Date(b.at).getTime());

        return {
          summary: toProposalListItem(raw),
          raw,
          rationale: spec.decisionMetadata?.rationale ?? "",
          events,
          observed: {
            cpu: asString(observed.cpu),
            memory: asString(observed.memory),
            p95Cpu: asString(p95.cpu),
            p95Memory: asString(p95.memory),
          },
          proposed: {
            cpu: asString(proposed.cpu),
            memory: asString(proposed.memory),
          },
          expected: {
            savingsUSD: asString(impact.estimatedSavingsUSD),
            carbonSavedGrams: asNumber(impact.carbonSavedGramsCO2),
            resourceDelta: impact.resourceDelta,
          },
          migrationPlan: spec.migrationPlan,
        };
      });
    },
    approve(ref: ProposalRef, reason = "Approved via UI"): Promise<void> {
      if (readReplayPreference()) return Promise.reject(replayMutationBlockedError());
      return json<void>(`/api/proposals/${encodeURIComponent(ref.name)}/approve`, {
        method: "POST",
        params: { namespace: ref.namespace },
        body: JSON.stringify({ reason }),
      });
    },
    reject(ref: ProposalRef, reason = "Rejected via UI"): Promise<void> {
      if (readReplayPreference()) return Promise.reject(replayMutationBlockedError());
      return json<void>(`/api/proposals/${encodeURIComponent(ref.name)}/reject`, {
        method: "POST",
        params: { namespace: ref.namespace },
        body: JSON.stringify({ reason }),
      });
    },
    rollback(ref: ProposalRef, reason = "Rollback requested via UI"): Promise<void> {
      if (readReplayPreference()) return Promise.reject(replayMutationBlockedError());
      return json<void>(`/api/proposals/${encodeURIComponent(ref.name)}/rollback`, {
        method: "POST",
        params: { namespace: ref.namespace },
        body: JSON.stringify({ reason }),
      });
    },
    bulkApprove(refs: ProposalRef[], reason?: string): Promise<void[]> {
      if (readReplayPreference()) return Promise.reject(replayMutationBlockedError());
      return Promise.all(refs.map((ref) => api.proposals.approve(ref, reason ?? "Bulk approved via UI")));
    },
    bulkReject(refs: ProposalRef[], reason?: string): Promise<void[]> {
      if (readReplayPreference()) return Promise.reject(replayMutationBlockedError());
      return Promise.all(refs.map((ref) => api.proposals.reject(ref, reason ?? "Bulk rejected via UI")));
    },
  },
  savings: {
    get(): Promise<SavingsResponse> {
      if (readReplayPreference()) return Promise.resolve(REPLAY_SAVINGS);
      return json<SavingsResponse>("/api/metrics/savings");
    },
  },
  forecast: {
    get(workload: string, namespace = "default"): Promise<ForecastPoint[]> {
      if (readReplayPreference()) return Promise.resolve(getReplayForecast(workload));
      return json<ForecastResponse>(`/api/forecast/${encodeURIComponent(workload)}`, {
        params: { namespace },
      }).then((env) => env.series ?? []);
    },
    whatIf(req: WhatIfRequest): Promise<WhatIfResponse> {
      if (readReplayPreference()) return Promise.resolve(computeReplayWhatIf(req));
      return json<WhatIfResponse>("/api/what-if", {
        method: "POST",
        body: JSON.stringify(req),
      });
    },
  },
  policies: {
    list(): Promise<PolicyView[]> {
      if (readReplayPreference()) return Promise.resolve(REPLAY_POLICIES);
      return json<ListEnvelope<RawPolicy>>("/api/policies").then((env) =>
        (env.items ?? []).map(toPolicyView),
      );
    },
    create(policy: RawPolicy): Promise<RawPolicy> {
      if (readReplayPreference()) return Promise.reject(replayMutationBlockedError());
      return json<RawPolicy>("/api/policies", {
        method: "POST",
        body: JSON.stringify(policy),
      });
    },
  },
  anomalies: {
    list(limit = 10): Promise<AnomalyItem[]> {
      if (readReplayPreference()) return Promise.resolve(REPLAY_ANOMALIES.slice(0, limit));
      return json<ListEnvelope<AnomalyItem>>("/api/anomalies", { params: { limit } }).then(
        (env) => env.items ?? [],
      );
    },
  },
  clusters: {
    list(): Promise<ClusterItem[]> {
      if (readReplayPreference()) return Promise.resolve(REPLAY_CLUSTERS);
      return json<ListEnvelope<ClusterItem>>("/api/clusters").then((env) => env.items ?? []);
    },
  },
  audit: {
    tail(limit = 20): Promise<AuditEntry[]> {
      if (readReplayPreference()) return Promise.resolve(REPLAY_AUDIT.slice(0, limit));
      return json<{ items: AuditEntry[] }>("/api/audit", { params: { limit } }).then(
        (res) => res.items ?? [],
      );
    },
  },
  healthz: {
    get(): Promise<HealthResponse> {
      if (readReplayPreference()) return Promise.resolve(REPLAY_HEALTH);
      return json<HealthResponse>("/healthz");
    },
  },
  chat: {
    send(params: {
      message: string;
      provider?: string;
      apiKey?: string;
      confirmToken?: string;
    }): Promise<ChatResponse> {
      if (readReplayPreference()) return Promise.reject(replayMutationBlockedError());
      return json<ChatResponse>("/api/chat", {
        method: "POST",
        body: JSON.stringify(params),
      });
    },
  },
};

export default api;
