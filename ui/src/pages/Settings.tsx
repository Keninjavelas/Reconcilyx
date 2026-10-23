import { useState } from "react";
import clsx from "clsx";
import PageHeader from "@/components/PageHeader";
import Icon from "@/components/Icon";
import { api, getApiBaseUrl, getApiKey, setApiBaseUrl, setApiKey } from "@/lib/api";
import { getLlmConfig, setLlmConfig, LLM_PROVIDERS, type LlmProvider } from "@/lib/llm";
import { useApi } from "@/lib/useApi";
import { formatCurrency, useAppStore } from "@/store/app";

type TestState =
  | { status: "idle" }
  | { status: "testing" }
  | { status: "ok"; message: string }
  | { status: "error"; message: string };

const SERVER_ENV: { key: string; description: string }[] = [
  { key: "RECONCILYX_API_KEY", description: "Requires X-API-Key on every non-public API route." },
  { key: "RECONCILYX_CORS_ORIGINS", description: "Comma-separated list of browser origins allowed to call the API." },
  { key: "RECONCILYX_API_PORT", description: "Port the control-plane API listens on." },
  { key: "RECONCILYX_CARBON_PROVIDER", description: "electricitymaps, watttime, or static." },
  { key: "RECONCILYX_EM_API_KEY", description: "Electricity Maps token used for grid intensity." },
  { key: "RECONCILYX_WT_API_KEY", description: "WattTime token used for grid intensity." },
  { key: "RECONCILYX_CARBON_STATIC_FALLBACK", description: "Flat gCO₂/kWh value used when no live provider is configured." },
  { key: "GITHUB_TOKEN", description: "Opens GitOps pull requests against GitHub repositories." },
  { key: "GITLAB_TOKEN", description: "Opens GitOps pull requests against GitLab repositories." },
  { key: "SLACK_WEBHOOK_URL", description: "Posts proposal and execution notifications to Slack." },
];

const PROVIDER_HINTS: Record<LlmProvider, string> = {
  ollama: "Runs on the control plane's host. No key is sent from the browser.",
  openai: "Platform API key, usually beginning with sk-.",
  anthropic: "Console API key, usually beginning with sk-ant-.",
  groq: "Groq Cloud key, usually beginning with gsk_.",
  cohere: "Cohere API key from the dashboard.",
};

export default function Settings() {
  const pushToast = useAppStore((s) => s.pushToast);
  const autoRefresh = useAppStore((s) => s.autoRefresh);
  const toggleAutoRefresh = useAppStore((s) => s.toggleAutoRefresh);
  const goalUSD = useAppStore((s) => s.monthlySavingsGoalUSD);
  const setMonthlySavingsGoalUSD = useAppStore((s) => s.setMonthlySavingsGoalUSD);

  const healthQuery = useApi(api.healthz.get);

  const [controlPlaneUrl, setControlPlaneUrl] = useState(() => getApiBaseUrl());
  const [controlPlaneKey, setControlPlaneKey] = useState(() => getApiKey());
  const [test, setTest] = useState<TestState>({ status: "idle" });

  const [llmProvider, setLlmProvider] = useState<LlmProvider>(() => getLlmConfig().provider);
  const [llmKey, setLlmKey] = useState(() => getLlmConfig().apiKey);

  const [goalInput, setGoalInput] = useState(() => String(goalUSD));

  function saveConnection(event: React.FormEvent) {
    event.preventDefault();
    try {
      const normalized = setApiBaseUrl(controlPlaneUrl);
      setApiKey(controlPlaneKey);
      setControlPlaneUrl(normalized);
      setTest({ status: "idle" });
      pushToast({
        kind: "success",
        title: "Connection saved",
        message: normalized ? `Requests will use ${normalized}.` : "Requests will use this site's origin.",
      });
    } catch (error) {
      pushToast({
        kind: "error",
        title: "Invalid control plane URL",
        message: error instanceof Error ? error.message : "Use an http or https URL.",
      });
    }
  }

  async function saveAndTestConnection() {
    setTest({ status: "testing" });
    try {
      const normalized = setApiBaseUrl(controlPlaneUrl);
      setApiKey(controlPlaneKey);
      setControlPlaneUrl(normalized);
      const started = performance.now();
      const health = await api.healthz.get();
      const latency = Math.round(performance.now() - started);
      setTest({
        status: "ok",
        message: `Answered "${health.status}" in ${latency} ms · reported ${new Date(health.time).toLocaleString()}`,
      });
      healthQuery.refetch();
      pushToast({
        kind: "success",
        title: "Control plane reachable",
        message: normalized || window.location.origin,
      });
    } catch (error) {
      setTest({
        status: "error",
        message: error instanceof Error ? error.message : String(error),
      });
    }
  }

  function saveAssistant(event: React.FormEvent) {
    event.preventDefault();
    setLlmConfig(llmProvider, llmKey.trim());
    pushToast({
      kind: "success",
      title: "Assistant settings saved",
      message: "Stored in this browser and attached to each chat request.",
    });
  }

  function saveGoal(event: React.FormEvent) {
    event.preventDefault();
    const parsed = Number(goalInput);
    if (!Number.isFinite(parsed) || parsed <= 0) {
      pushToast({
        kind: "error",
        title: "Invalid goal",
        message: "Enter a monthly savings goal greater than zero.",
      });
      return;
    }
    setMonthlySavingsGoalUSD(parsed);
    setGoalInput(String(parsed));
    pushToast({
      kind: "success",
      title: "Monthly goal saved",
      message: `${formatCurrency(parsed, 0)} stored in this browser.`,
    });
  }

  const healthOnline = healthQuery.data?.status === "ok";

  return (
    <div className="space-y-10 lg:space-y-14">
      <PageHeader
        index="06"
        eyebrow="Console configuration"
        title={
          <>
            Tune the console.{" "}
            <span className="editorial text-mint-500">Not the cluster.</span>
          </>
        }
        description="Connection details, assistant credentials, and preferences live in this browser. Cluster-side providers — carbon intensity, Git tokens, notifications — are configured on the operator Deployment and are intentionally not editable here."
        meta={
          <>
            <span>
              Control plane{" "}
              {healthQuery.data
                ? healthOnline
                  ? "reachable"
                  : `reported ${healthQuery.data.status}`
                : healthQuery.loading
                  ? "checking…"
                  : "unreachable"}
            </span>
            <span>{getApiBaseUrl() || `${window.location.origin} (same origin)`}</span>
            <span>Browser-local settings</span>
          </>
        }
      />

      <section className="panel">
        <div className="panel-header">
          <div>
            <div className="eyebrow">Connection</div>
            <div className="panel-title mt-1">Control plane API.</div>
          </div>
          <span
            className={clsx(
              "chip border",
              healthOnline
                ? "border-mint-500/35 bg-mint-500/10 text-mint-500"
                : healthQuery.error
                  ? "border-ember-500/35 bg-ember-500/10 text-ember-400"
                  : "border-white/[0.12] bg-white/[0.03] text-ink-400",
            )}
          >
            {healthQuery.loading && !healthQuery.data
              ? "checking"
              : healthOnline
                ? "online"
                : "unreachable"}
          </span>
        </div>
        <div className="grid gap-0 lg:grid-cols-[1.15fr_0.85fr]">
          <form onSubmit={saveConnection} className="space-y-5 p-5 sm:p-6">
            <div>
              <label className="field-label" htmlFor="control-plane-url">
                API origin
              </label>
              <input
                id="control-plane-url"
                type="url"
                value={controlPlaneUrl}
                onChange={(event) => setControlPlaneUrl(event.target.value)}
                placeholder="https://reconcilyx.example.com"
                className="input font-mono text-[12px]"
              />
              <p className="mt-2 text-[11px] leading-relaxed text-ink-500">
                Leave blank when the console is served by the same ingress as the API.
              </p>
            </div>
            <div>
              <label className="field-label" htmlFor="control-plane-key">
                API key
              </label>
              <input
                id="control-plane-key"
                type="password"
                value={controlPlaneKey}
                onChange={(event) => setControlPlaneKey(event.target.value)}
                placeholder="Optional when RECONCILYX_API_KEY is unset"
                autoComplete="off"
                className="input font-mono text-[12px]"
              />
              <p className="mt-2 text-[11px] leading-relaxed text-ink-500">
                Sent as <span className="font-mono text-ink-300">X-API-Key</span> on every request.
                A remote API must allow this origin through{" "}
                <span className="font-mono text-ink-300">RECONCILYX_CORS_ORIGINS</span>.
              </p>
            </div>

            <div className="flex flex-wrap items-center gap-2">
              <button type="submit" className="btn-ghost">
                Save
              </button>
              <button
                type="button"
                className="btn-primary"
                onClick={saveAndTestConnection}
                disabled={test.status === "testing"}
              >
                <Icon name="refresh" size={13} />
                {test.status === "testing" ? "Testing…" : "Save & test"}
              </button>
              <button
                type="button"
                className="btn-ghost ml-auto"
                onClick={() => {
                  setApiBaseUrl("");
                  setApiKey("");
                  setControlPlaneUrl("");
                  setControlPlaneKey("");
                  setTest({ status: "idle" });
                  healthQuery.refetch();
                  pushToast({
                    kind: "info",
                    title: "Connection cleared",
                    message: "Requests will use this site's origin with no API key.",
                  });
                }}
              >
                Clear
              </button>
            </div>

            {test.status === "ok" ? (
              <div className="rounded-xl border border-mint-500/30 bg-mint-500/[0.06] px-4 py-3 text-[11px] leading-relaxed text-mint-500">
                {test.message}
              </div>
            ) : test.status === "error" ? (
              <div className="rounded-xl border border-ember-500/30 bg-ember-500/[0.06] px-4 py-3 text-[11px] leading-relaxed text-ember-400">
                <span className="font-medium">Connection test failed. </span>
                <span className="break-words">{test.message}</span>
              </div>
            ) : null}
          </form>

          <aside className="border-t border-white/[0.08] bg-ink-950/50 p-5 sm:p-6 lg:border-l lg:border-t-0">
            <div className="eyebrow">Live diagnostic</div>
            <dl className="mt-4 space-y-4">
              <Diagnostic
                label="Health"
                value={
                  healthQuery.data
                    ? healthQuery.data.status
                    : healthQuery.error
                      ? "unreachable"
                      : "pending"
                }
                tone={healthOnline ? "good" : healthQuery.error ? "bad" : "flat"}
              />
              <Diagnostic
                label="Last check"
                value={
                  healthQuery.fetchedAt
                    ? new Date(healthQuery.fetchedAt).toLocaleTimeString()
                    : "not yet"
                }
              />
              <Diagnostic
                label="Server report"
                value={
                  healthQuery.data?.time
                    ? new Date(healthQuery.data.time).toLocaleString()
                    : "not reported"
                }
              />
              <Diagnostic label="UI build" value="reconcilyx-dashboard 2.0.0" />
            </dl>
            {healthQuery.error ? (
              <p className="mt-5 break-words rounded-xl border border-ember-500/25 bg-ember-500/[0.05] p-3 text-[10.5px] leading-relaxed text-ember-400">
                {healthQuery.error}
              </p>
            ) : (
              <p className="mt-5 text-[10.5px] leading-relaxed text-ink-500">
                The header reads the same endpoint. A failed check here means the console cannot
                reach the API, not that the operator is down.
              </p>
            )}
          </aside>
        </div>
      </section>

      <section className="grid gap-4 xl:grid-cols-2">
        <form onSubmit={saveAssistant} className="panel flex flex-col">
          <div className="panel-header">
            <div>
              <div className="eyebrow">Assistant</div>
              <div className="panel-title mt-1">Bring your own model key.</div>
            </div>
            <Icon name="lock" size={16} className="text-ink-500" />
          </div>
          <div className="flex-1 space-y-5 p-5 sm:p-6">
            <div>
              <label className="field-label" htmlFor="llm-provider">
                Provider
              </label>
              <select
                id="llm-provider"
                value={llmProvider}
                onChange={(event) => setLlmProvider(event.target.value as LlmProvider)}
                className="input font-mono text-[12px]"
              >
                {LLM_PROVIDERS.map((provider) => (
                  <option key={provider} value={provider}>
                    {provider}
                  </option>
                ))}
              </select>
              <p className="mt-2 text-[11px] leading-relaxed text-ink-500">
                {PROVIDER_HINTS[llmProvider]}
              </p>
            </div>
            <div>
              <label className="field-label" htmlFor="llm-key">
                API key
              </label>
              <input
                id="llm-key"
                type="password"
                value={llmKey}
                onChange={(event) => setLlmKey(event.target.value)}
                placeholder={llmProvider === "ollama" ? "Not required" : "Provider key"}
                autoComplete="off"
                className="input font-mono text-[12px]"
                disabled={llmProvider === "ollama"}
              />
            </div>
            <p className="rounded-xl border border-white/[0.08] bg-white/[0.02] p-3 text-[10.5px] leading-relaxed text-ink-500">
              The key is stored in this browser and attached to each{" "}
              <span className="font-mono text-ink-300">POST /api/chat</span> request. This console
              does not write assistant credentials into the cluster.
            </p>
          </div>
          <div className="flex items-center justify-between border-t border-white/[0.08] px-5 py-4 sm:px-6">
            <span className="font-mono text-[9px] uppercase tracking-[0.14em] text-ink-600">
              {llmKey ? "key present" : "no key stored"}
            </span>
            <button type="submit" className="btn-primary">
              Save assistant
            </button>
          </div>
        </form>

        <div className="panel flex flex-col">
          <div className="panel-header">
            <div>
              <div className="eyebrow">Preferences</div>
              <div className="panel-title mt-1">How this console behaves.</div>
            </div>
            <Icon name="terminal" size={16} className="text-ink-500" />
          </div>
          <div className="flex-1 space-y-0 p-5 sm:p-6">
            <div className="flex items-start justify-between gap-6 pb-6">
              <div>
                <div className="text-[13px] text-ink-100">Auto-refresh</div>
                <p className="mt-1 max-w-[36ch] text-[11px] leading-relaxed text-ink-500">
                  Re-read the control plane every 30 seconds while a page is open.
                </p>
              </div>
              <button
                type="button"
                role="switch"
                aria-checked={autoRefresh}
                onClick={toggleAutoRefresh}
                className={clsx(
                  "relative h-6 w-11 shrink-0 rounded-full border transition",
                  autoRefresh
                    ? "border-mint-500/50 bg-mint-500/25"
                    : "border-white/[0.14] bg-white/[0.04]",
                )}
              >
                <span
                  className={clsx(
                    "absolute top-1/2 h-4 w-4 -translate-y-1/2 rounded-full transition-all",
                    autoRefresh ? "left-[22px] bg-mint-500" : "left-[3px] bg-ink-400",
                  )}
                />
                <span className="sr-only">Toggle auto-refresh</span>
              </button>
            </div>

            <form onSubmit={saveGoal} className="border-t border-white/[0.08] pt-6">
              <label className="field-label" htmlFor="savings-goal">
                Monthly savings goal
              </label>
              <div className="flex gap-2">
                <div className="relative flex-1">
                  <span className="pointer-events-none absolute left-3.5 top-1/2 -translate-y-1/2 font-mono text-[12px] text-ink-500">
                    $
                  </span>
                  <input
                    id="savings-goal"
                    type="number"
                    min={1}
                    step="any"
                    value={goalInput}
                    onChange={(event) => setGoalInput(event.target.value)}
                    className="input pl-7 font-mono text-[12px]"
                  />
                </div>
                <button type="submit" className="btn-ghost">
                  Save
                </button>
              </div>
              <p className="mt-2 text-[11px] leading-relaxed text-ink-500">
                Used only by the overview progress bar. Stored in this browser; it does not change
                controller behaviour.
              </p>
            </form>
          </div>
          <div className="border-t border-white/[0.08] px-5 py-3 font-mono text-[9px] uppercase tracking-[0.14em] text-ink-600 sm:px-6">
            Current goal {formatCurrency(goalUSD, 0)}
          </div>
        </div>
      </section>

      <section className="panel">
        <div className="panel-header">
          <div>
            <div className="eyebrow">Server-side configuration</div>
            <div className="panel-title mt-1">What this console cannot change.</div>
          </div>
          <span className="font-mono text-[9px] uppercase tracking-[0.14em] text-ink-500">
            set on the operator Deployment
          </span>
        </div>
        <div className="grid gap-px bg-white/[0.06] md:grid-cols-2">
          {SERVER_ENV.map((entry) => (
            <div key={entry.key} className="bg-ink-900 px-5 py-3.5 md:px-6">
              <div className="font-mono text-[10px] tracking-[0.02em] text-mint-500">{entry.key}</div>
              <div className="mt-1 text-[11px] leading-relaxed text-ink-400">{entry.description}</div>
            </div>
          ))}
        </div>
        <p className="border-t border-white/[0.08] px-5 py-4 text-[11px] leading-relaxed text-ink-500 sm:px-6">
          These values belong to the cluster operator, not the browser. Set them through your Helm
          values or Deployment environment and restart the control plane to apply changes.
        </p>
      </section>
    </div>
  );
}

function Diagnostic({
  label,
  value,
  tone = "flat",
}: {
  label: string;
  value: string;
  tone?: "good" | "bad" | "flat";
}) {
  return (
    <div className="flex items-start justify-between gap-4 border-b border-white/[0.06] pb-3 last:border-b-0">
      <dt className="font-mono text-[9px] uppercase tracking-[0.14em] text-ink-500">{label}</dt>
      <dd
        className={clsx(
          "max-w-[26ch] break-words text-right font-mono text-[10.5px]",
          tone === "good" ? "text-mint-500" : tone === "bad" ? "text-ember-400" : "text-ink-200",
        )}
      >
        {value}
      </dd>
    </div>
  );
}
