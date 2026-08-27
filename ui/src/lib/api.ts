import type {
  AnomalyItem,
  AuditEntry,
  ChatResponse,
  ClusterItem,
  ForecastPoint,
  ForecastResponse,
  OptimizationPolicySpec,
  ProposalDetail,
  ProposalListItem,
  PolicyListItem,
  SavingsResponse,
  WhatIfRequest,
  WhatIfResponse,
} from "./types";

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
    throw new Error(`HTTP ${res.status} ${res.statusText} — ${body.slice(0, 200)}`);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

type ListEnvelope<T> = { items: T[]; total: number };

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
      return json<ListEnvelope<ProposalListItem>>("/api/proposals", { params }).then(
        (env) => env.items ?? [],
      );
    },
    detail(name: string): Promise<ProposalDetail> {
      return json<ProposalDetail>(`/api/proposals/${encodeURIComponent(name)}`);
    },
    approve(name: string, reason?: string): Promise<void> {
      return json<void>(`/api/proposals/${encodeURIComponent(name)}/approve`, {
        method: "POST",
        body: JSON.stringify({ reason: reason ?? "Approved via UI" }),
      });
    },
    reject(name: string, reason?: string): Promise<void> {
      return json<void>(`/api/proposals/${encodeURIComponent(name)}/reject`, {
        method: "POST",
        body: JSON.stringify({ reason: reason ?? "Rejected via UI" }),
      });
    },
    rollback(name: string, reason?: string): Promise<void> {
      return json<void>(`/api/proposals/${encodeURIComponent(name)}/rollback`, {
        method: "POST",
        body: JSON.stringify({ reason: reason ?? "Rollback requested via UI" }),
      });
    },
    bulkApprove(names: string[], reason?: string): Promise<void[]> {
      return Promise.all(names.map((n) => api.proposals.approve(n, reason ?? "Bulk approved via UI")));
    },
    bulkReject(names: string[], reason?: string): Promise<void[]> {
      return Promise.all(names.map((n) => api.proposals.reject(n, reason ?? "Bulk rejected via UI")));
    },
  },
  savings: {
    get(): Promise<SavingsResponse> {
      return json<SavingsResponse>("/api/metrics/savings");
    },
  },
  forecast: {
    get(workload: string, namespace = "default"): Promise<ForecastPoint[]> {
      return json<ForecastResponse>(`/api/forecast/${encodeURIComponent(workload)}`, {
        params: { namespace },
      }).then((env) => env.series ?? []);
    },
    whatIf(req: WhatIfRequest): Promise<WhatIfResponse> {
      return json<WhatIfResponse>("/api/what-if", {
        method: "POST",
        body: JSON.stringify(req),
      });
    },
  },
  policies: {
    list(): Promise<PolicyListItem[]> {
      return json<ListEnvelope<PolicyListItem>>("/api/policies").then((env) => env.items ?? []);
    },
    create(
      body: OptimizationPolicySpec & { metadata?: { name: string; namespace: string }; name?: string; namespace?: string },
    ): Promise<{ metadata?: { name: string; namespace: string }; name: string }> {
      return json<{ metadata?: { name: string; namespace: string }; name: string }>("/api/policies", {
        method: "POST",
        body: JSON.stringify(body),
      });
    },
  },
  anomalies: {
    list(limit = 10): Promise<AnomalyItem[]> {
      return json<ListEnvelope<AnomalyItem>>("/api/anomalies", { params: { limit } }).then(
        (env) => env.items ?? [],
      );
    },
  },
  clusters: {
    list(): Promise<ClusterItem[]> {
      return json<ListEnvelope<ClusterItem>>("/api/clusters").then((env) => env.items ?? []);
    },
  },
  audit: {
    tail(limit = 20): Promise<AuditEntry[]> {
      return json<{ entries: AuditEntry[] }>("/api/audit", { params: { limit } }).then(
        (res) => res.entries ?? [],
      );
    },
  },
  healthz: {
    get(): Promise<{ ok: boolean; refreshed: string }> {
      return json("/healthz");
    },
  },
  chat: {
    send(params: {
      message: string;
      provider?: string;
      apiKey?: string;
      confirmToken?: string;
    }): Promise<ChatResponse> {
      return json<ChatResponse>("/api/chat", {
        method: "POST",
        body: JSON.stringify(params),
      });
    },
  },
};

export default api;
