import { useEffect, useMemo, useState } from "react";
import clsx from "clsx";
import PageHeader from "@/components/PageHeader";
import DataTable from "@/components/DataTable";
import Icon from "@/components/Icon";
import { api } from "@/lib/api";
import { useApi } from "@/lib/useApi";
import type {
  GitProvider,
  PolicyMode,
  PolicyObjective,
  PolicyView,
  RawPolicy,
  RawPolicySpec,
  RecommendationType,
} from "@/lib/types";
import { useAppStore } from "@/store/app";

const OBJECTIVES: PolicyObjective[] = ["Cost", "Performance", "Efficiency"];
const MODES: PolicyMode[] = ["Observe", "Propose", "AutoApply"];
const PROVIDERS: GitProvider[] = ["mock", "github", "gitlab"];
const RECOMMENDATIONS: { value: RecommendationType; label: string }[] = [
  { value: "RightSize", label: "Right-size" },
  { value: "ScaleToZero", label: "Scale to zero" },
  { value: "ReduceCPURequest", label: "Reduce CPU" },
  { value: "ReduceMemoryRequest", label: "Reduce memory" },
  { value: "MigrateCloud", label: "Migrate cloud" },
];

const MODE_COPY: Record<PolicyMode, string> = {
  Observe: "Record findings, never propose.",
  Propose: "Queue proposals for human review.",
  AutoApply: "Approve low-risk proposals automatically.",
};

const MODE_STYLE: Record<PolicyMode, string> = {
  Observe: "border-sky-500/35 bg-sky-500/10 text-sky-400",
  Propose: "border-amber-500/35 bg-amber-500/10 text-amber-400",
  AutoApply: "border-mint-500/35 bg-mint-500/10 text-mint-500",
};

export default function Policies() {
  const policiesQuery = useApi(api.policies.list);
  const policies = policiesQuery.data ?? [];
  const pushToast = useAppStore((s) => s.pushToast);
  const [showWizard, setShowWizard] = useState(false);

  const autoApply = policies.filter((policy) => policy.mode === "AutoApply").length;
  const carbonLed = policies.filter((policy) => policy.carbonWeight > 0).length;
  const gitOpsArmed = policies.filter((policy) => policy.gitOpsEnabled).length;

  async function createPolicy(policy: RawPolicy) {
    const created = await api.policies.create(policy);
    pushToast({
      kind: "success",
      title: "Policy created",
      message: `${created.metadata?.namespace ?? policy.metadata.namespace}/${created.metadata?.name ?? policy.metadata.name}`,
    });
    setShowWizard(false);
    policiesQuery.refetch();
  }

  return (
    <div className="space-y-10 lg:space-y-14">
      <PageHeader
        index="05"
        eyebrow="Governance"
        title={
          <>
            Rules that <span className="editorial text-mint-500">earn trust</span> before they act.
          </>
        }
        description="An OptimizationPolicy decides which workloads are in scope, how much risk is tolerable, and whether a human or the controller pulls the trigger. Every field below maps directly to the OptimizationPolicy custom resource."
        actions={
          <button className="btn-primary" onClick={() => setShowWizard(true)}>
            <Icon name="plus" size={14} />
            New policy
          </button>
        }
        meta={
          <>
            <span>{policiesQuery.data ? `${policies.length} policies` : "Policies not loaded"}</span>
            <span>{policiesQuery.data ? `${autoApply} auto-apply` : "—"}</span>
            <span>{policiesQuery.data ? `${carbonLed} carbon-led` : "—"}</span>
            <span>{policiesQuery.data ? `${gitOpsArmed} GitOps` : "—"}</span>
          </>
        }
      />

      {policiesQuery.error ? (
        <div
          role="alert"
          className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-ember-500/30 bg-ember-500/[0.06] px-4 py-3 text-[12px] text-ember-300"
        >
          <span className="break-words">
            Policies could not be loaded.
            <span className="ml-2 text-ink-400">{policiesQuery.error}</span>
          </span>
          <button className="btn-ghost h-8 px-3 text-[11px]" onClick={policiesQuery.refetch}>
            Retry
          </button>
        </div>
      ) : null}

      <div className="panel">
        <div className="panel-header">
          <div>
            <div className="eyebrow">Catalogue</div>
            <div className="panel-title mt-1">Active optimization policies.</div>
          </div>
          <span className="font-mono text-[9px] uppercase tracking-[0.14em] text-ink-500">
            {policiesQuery.data ? `${policies.length} resources` : "reading cluster…"}
          </span>
        </div>
        <DataTable<PolicyView>
          rows={policies}
          rowKey={(policy) => `${policy.namespace}/${policy.name}`}
          empty={
            policiesQuery.loading && !policiesQuery.data
              ? "Loading policies…"
              : "No OptimizationPolicies exist yet. Create one to start generating proposals."
          }
          columns={[
            {
              key: "policy",
              title: "Policy",
              render: (policy) => (
                <div className="min-w-[190px]">
                  <div className="text-[13px] text-ink-100">
                    {policy.namespace}/{policy.name}
                  </div>
                  <div className="mt-0.5 flex items-center gap-2 font-mono text-[9px] uppercase tracking-[0.1em] text-ink-500">
                    <span>{policy.objective}</span>
                    {policy.createdAt ? (
                      <>
                        <span className="text-ink-700">/</span>
                        <span>{new Date(policy.createdAt).toLocaleDateString()}</span>
                      </>
                    ) : null}
                  </div>
                </div>
              ),
            },
            {
              key: "mode",
              title: "Mode",
              render: (policy) => (
                <span className={clsx("chip border", MODE_STYLE[policy.mode])}>{policy.mode}</span>
              ),
            },
            {
              key: "targets",
              title: "Targets",
              render: (policy) => (
                <div
                  className="max-w-[340px] truncate font-mono text-[10px] text-ink-300"
                  title={policy.targetSummary}
                >
                  {policy.targetSummary}
                </div>
              ),
            },
            {
              key: "guardrails",
              title: "Guardrails",
              render: (policy) => (
                <div className="space-y-0.5 font-mono text-[9px] uppercase tracking-[0.08em] text-ink-500">
                  <div>
                    risk ≤ <span className="text-ink-200">{policy.riskTolerance ?? "unset"}</span>
                  </div>
                  <div>
                    confidence ≥ <span className="text-ink-200">{policy.confidenceRequirement ?? "unset"}</span>
                  </div>
                  <div>
                    replicas ≥ <span className="text-ink-200">{policy.minReplicas ?? "unset"}</span>
                  </div>
                </div>
              ),
            },
            {
              key: "carbon",
              title: "Carbon weight",
              render: (policy) => (
                <div className="w-[120px]">
                  <div className="flex items-baseline justify-between">
                    <span className="font-mono text-[9px] uppercase tracking-[0.08em] text-ink-500">
                      cost
                    </span>
                    <span className="num text-[10px] text-ink-200">
                      {Math.round(policy.carbonWeight * 100)}%
                    </span>
                  </div>
                  <div className="mt-1.5 h-[3px] w-full overflow-hidden bg-white/[0.06]">
                    <div
                      className="h-full bg-mint-500"
                      style={{
                        width: `${policy.carbonWeight > 0 ? Math.max(2, policy.carbonWeight * 100) : 0}%`,
                      }}
                    />
                  </div>
                </div>
              ),
            },
            {
              key: "gitops",
              title: "Actuation",
              render: (policy) => (
                <div className="min-w-[120px]">
                  <div className="font-mono text-[10px] uppercase tracking-[0.08em] text-ink-300">
                    {policy.gitOpsEnabled ? policy.gitOpsProvider : "live patch"}
                  </div>
                  <div className="mt-0.5 font-mono text-[9px] text-ink-500">
                    {policy.gitOpsEnabled
                      ? policy.repository ?? "repository unset"
                      : "in-place resize"}
                  </div>
                </div>
              ),
            },
          ]}
        />
      </div>

      {showWizard ? (
        <CreatePolicyWizard
          onClose={() => setShowWizard(false)}
          onCreate={createPolicy}
        />
      ) : null}
    </div>
  );
}

function CreatePolicyWizard({
  onClose,
  onCreate,
}: {
  onClose: () => void;
  onCreate: (policy: RawPolicy) => Promise<void>;
}) {
  const [name, setName] = useState("cost-and-carbon-policy");
  const [namespace, setNamespace] = useState("default");
  const [objective, setObjective] = useState<PolicyObjective>("Efficiency");
  const [mode, setMode] = useState<PolicyMode>("Propose");
  const [nsLabelValue, setNsLabelValue] = useState("");
  const [workloadKey, setWorkloadKey] = useState("app");
  const [workloadValue, setWorkloadValue] = useState("");
  const [carbonWeight, setCarbonWeight] = useState(0.3);
  const [riskTolerance, setRiskTolerance] = useState(60);
  const [confidenceRequirement, setConfidenceRequirement] = useState(70);
  const [minReplicas, setMinReplicas] = useState(2);
  const [errorRate, setErrorRate] = useState(5);
  const [allowed, setAllowed] = useState<RecommendationType[]>(
    RECOMMENDATIONS.map((item) => item.value),
  );
  const [excludedContainers, setExcludedContainers] = useState("istio-proxy");
  const [gitOpsEnabled, setGitOpsEnabled] = useState(false);
  const [gitProvider, setGitProvider] = useState<GitProvider>("mock");
  const [gitOwner, setGitOwner] = useState("");
  const [gitRepo, setGitRepo] = useState("");
  const [gitBaseBranch, setGitBaseBranch] = useState("main");
  const [gitPath, setGitPath] = useState("manifests/");
  const [autoMerge, setAutoMerge] = useState(false);
  const [creating, setCreating] = useState(false);
  const [submitError, setSubmitError] = useState<string | undefined>(undefined);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [onClose]);

  const nameValid = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/.test(name) && name.length <= 63;
  const namespaceValid =
    /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/.test(namespace) && namespace.length <= 63;
  const canCreate = nameValid && namespaceValid && !creating;

  const draft = useMemo<RawPolicy>(() => {
    const targetSelectors: RawPolicySpec["targetSelectors"] = {};
    if (nsLabelValue.trim()) {
      targetSelectors.namespaceSelector = {
        matchLabels: { "kubernetes.io/metadata.name": nsLabelValue.trim() },
      };
    }
    if (workloadKey.trim() && workloadValue.trim()) {
      targetSelectors.workloadSelector = {
        matchLabels: { [workloadKey.trim()]: workloadValue.trim() },
      };
    }

    const excluded = excludedContainers
      .split(",")
      .map((item) => item.trim())
      .filter(Boolean);

    return {
      apiVersion: "optimization.reconcilyx.io/v1alpha1",
      kind: "OptimizationPolicy",
      metadata: { name, namespace },
      spec: {
        objective,
        mode,
        targetSelectors:
          Object.keys(targetSelectors).length > 0 ? targetSelectors : undefined,
        safetyConstraints: {
          minReplicas,
          riskTolerance,
          confidenceRequirement,
          carbonWeight,
        },
        rollbackConstraints: {
          errorRateThresholdPercent: errorRate,
        },
        recommendationGovernance: {
          allowedRecommendations: allowed,
        },
        workloadProtection: excluded.length > 0 ? { excludedContainers: excluded } : undefined,
        gitOps: {
          enabled: gitOpsEnabled,
          provider: gitProvider,
          owner: gitOwner.trim() || undefined,
          repo: gitRepo.trim() || undefined,
          baseBranch: gitBaseBranch.trim() || "main",
          manifestsPath: gitPath.trim() || undefined,
          autoMerge,
        },
      },
    };
  }, [
    name,
    namespace,
    objective,
    mode,
    nsLabelValue,
    workloadKey,
    workloadValue,
    carbonWeight,
    riskTolerance,
    confidenceRequirement,
    minReplicas,
    errorRate,
    allowed,
    excludedContainers,
    gitOpsEnabled,
    gitProvider,
    gitOwner,
    gitRepo,
    gitBaseBranch,
    gitPath,
    autoMerge,
  ]);

  const manifest = useMemo(() => toYaml(stripUndefined(draft)), [draft]);

  function toggleRecommendation(value: RecommendationType) {
    setAllowed((current) =>
      current.includes(value) ? current.filter((item) => item !== value) : [...current, value],
    );
  }

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    if (!canCreate) return;
    setCreating(true);
    setSubmitError(undefined);
    try {
      await onCreate(draft);
    } catch (error) {
      setSubmitError(error instanceof Error ? error.message : String(error));
    } finally {
      setCreating(false);
    }
  }

  return (
    <div
      className="overlay fixed inset-0 z-50 flex items-start justify-center overflow-y-auto p-4 sm:p-6"
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="new-policy-title"
        className="panel my-4 w-full max-w-5xl animate-fade-in-up"
      >
        <div className="panel-header items-center">
          <div>
            <div className="eyebrow">New OptimizationPolicy</div>
            <div id="new-policy-title" className="panel-title mt-1">
              Define the contract, then let the controller work.
            </div>
          </div>
          <button className="btn-icon h-9 w-9" onClick={onClose} aria-label="Close">
            <Icon name="close" size={16} />
          </button>
        </div>

        <form onSubmit={submit}>
          <div className="grid gap-0 lg:grid-cols-[1.1fr_0.9fr]">
            <div className="max-h-[calc(100vh-14rem)] space-y-7 overflow-y-auto p-5 scrollbar-thin sm:p-6">
              <section className="space-y-4">
                <div className="eyebrow">Identity</div>
                <div className="grid gap-4 sm:grid-cols-2">
                  <Field label="Policy name" hint={!nameValid ? "Lowercase DNS label, max 63 characters." : undefined} invalid={!nameValid}>
                    <input
                      className="input font-mono text-[12px]"
                      value={name}
                      onChange={(event) => setName(event.target.value)}
                      autoFocus
                    />
                  </Field>
                  <Field
                    label="Namespace"
                    hint={!namespaceValid ? "Lowercase DNS label, max 63 characters." : undefined}
                    invalid={!namespaceValid}
                  >
                    <input
                      className="input font-mono text-[12px]"
                      value={namespace}
                      onChange={(event) => setNamespace(event.target.value)}
                    />
                  </Field>
                </div>
              </section>

              <section className="space-y-4 border-t border-white/[0.08] pt-6">
                <div className="eyebrow">Intent</div>
                <Field label="Objective">
                  <div className="grid grid-cols-3 gap-2">
                    {OBJECTIVES.map((value) => (
                      <ChoiceButton
                        key={value}
                        active={objective === value}
                        onClick={() => setObjective(value)}
                        title={value}
                        description={
                          value === "Cost"
                            ? "Minimize spend first."
                            : value === "Performance"
                              ? "Protect headroom first."
                              : "Balance both signals."
                        }
                      />
                    ))}
                  </div>
                </Field>
                <Field label="Mode">
                  <div className="grid grid-cols-3 gap-2">
                    {MODES.map((value) => (
                      <ChoiceButton
                        key={value}
                        active={mode === value}
                        onClick={() => setMode(value)}
                        title={value}
                        description={MODE_COPY[value]}
                      />
                    ))}
                  </div>
                </Field>
              </section>

              <section className="space-y-4 border-t border-white/[0.08] pt-6">
                <div>
                  <div className="eyebrow">Target scope</div>
                  <p className="mt-1.5 text-[11px] leading-relaxed text-ink-500">
                    Leave either value blank to match every namespace or workload.
                  </p>
                </div>
                <div className="grid gap-4 sm:grid-cols-2">
                  <Field label="Namespace name">
                    <input
                      className="input font-mono text-[12px]"
                      value={nsLabelValue}
                      onChange={(event) => setNsLabelValue(event.target.value)}
                      placeholder="all namespaces"
                    />
                  </Field>
                  <div className="grid grid-cols-[0.8fr_1.2fr] gap-2">
                    <Field label="Label key">
                      <input
                        className="input font-mono text-[12px]"
                        value={workloadKey}
                        onChange={(event) => setWorkloadKey(event.target.value)}
                      />
                    </Field>
                    <Field label="Label value">
                      <input
                        className="input font-mono text-[12px]"
                        value={workloadValue}
                        onChange={(event) => setWorkloadValue(event.target.value)}
                        placeholder="all workloads"
                      />
                    </Field>
                  </div>
                </div>
              </section>

              <section className="space-y-4 border-t border-white/[0.08] pt-6">
                <div className="eyebrow">Guardrails</div>
                <Field
                  label={`Carbon weight — ${Math.round(carbonWeight * 100)}% toward carbon`}
                  hint="0 is cost-only; 1 is carbon-only. The evaluator uses this to blend both signals."
                >
                  <input
                    type="range"
                    min={0}
                    max={1}
                    step={0.05}
                    value={carbonWeight}
                    onChange={(event) => setCarbonWeight(Number(event.target.value))}
                    className="range"
                  />
                  <div className="flex justify-between font-mono text-[9px] text-ink-600">
                    <span>cost first</span>
                    <span>balanced</span>
                    <span>carbon first</span>
                  </div>
                </Field>
                <div className="grid gap-4 sm:grid-cols-3">
                  <NumberField
                    label="Risk tolerance"
                    value={riskTolerance}
                    min={0}
                    max={100}
                    onChange={setRiskTolerance}
                    suffix="≤"
                  />
                  <NumberField
                    label="Confidence"
                    value={confidenceRequirement}
                    min={0}
                    max={100}
                    onChange={setConfidenceRequirement}
                    suffix="≥"
                  />
                  <NumberField
                    label="Min replicas"
                    value={minReplicas}
                    min={1}
                    max={100}
                    onChange={setMinReplicas}
                    suffix="≥"
                  />
                </div>
                <NumberField
                  label="Rollback above error rate"
                  value={errorRate}
                  min={0}
                  max={100}
                  onChange={setErrorRate}
                  suffix="%"
                />
              </section>

              <section className="space-y-4 border-t border-white/[0.08] pt-6">
                <div className="eyebrow">Recommendation governance</div>
                <div className="flex flex-wrap gap-2">
                  {RECOMMENDATIONS.map((item) => {
                    const active = allowed.includes(item.value);
                    return (
                      <button
                        key={item.value}
                        type="button"
                        onClick={() => toggleRecommendation(item.value)}
                        className={clsx(
                          "chip border transition",
                          active
                            ? "border-mint-500/40 bg-mint-500/10 text-mint-500"
                            : "border-white/[0.12] bg-transparent text-ink-500 hover:text-ink-300",
                        )}
                        aria-pressed={active}
                      >
                        {active ? <Icon name="check" size={10} /> : null}
                        {item.label}
                      </button>
                    );
                  })}
                </div>
                {allowed.length === 0 ? (
                  <p className="text-[11px] text-ember-400">
                    At least one recommendation should be allowed, or this policy can never propose anything.
                  </p>
                ) : null}
                <Field
                  label="Excluded containers"
                  hint="Comma-separated. Optimization is skipped when any of these containers is present."
                >
                  <input
                    className="input font-mono text-[12px]"
                    value={excludedContainers}
                    onChange={(event) => setExcludedContainers(event.target.value)}
                    placeholder="istio-proxy, linkerd-proxy"
                  />
                </Field>
              </section>

              <section className="space-y-4 border-t border-white/[0.08] pt-6">
                <div className="flex items-center justify-between gap-4">
                  <div>
                    <div className="eyebrow">GitOps actuation</div>
                    <p className="mt-1.5 text-[11px] leading-relaxed text-ink-500">
                      When enabled, approved proposals open a pull request instead of patching the cluster.
                    </p>
                  </div>
                  <label className="flex shrink-0 cursor-pointer items-center gap-2 text-[11px] text-ink-300">
                    <input
                      type="checkbox"
                      checked={gitOpsEnabled}
                      onChange={(event) => setGitOpsEnabled(event.target.checked)}
                      className="h-4 w-4 accent-mint-500"
                    />
                    Enabled
                  </label>
                </div>

                {gitOpsEnabled ? (
                  <div className="space-y-4">
                    <div className="grid grid-cols-3 gap-2">
                      {PROVIDERS.map((provider) => (
                        <ChoiceButton
                          key={provider}
                          active={gitProvider === provider}
                          onClick={() => setGitProvider(provider)}
                          title={provider}
                          description={
                            provider === "mock"
                              ? "Dry run, no remote calls."
                              : `Open PRs on ${provider}.`
                          }
                        />
                      ))}
                    </div>
                    <div className="grid gap-4 sm:grid-cols-2">
                      <Field label="Owner / namespace">
                        <input
                          className="input font-mono text-[12px]"
                          value={gitOwner}
                          onChange={(event) => setGitOwner(event.target.value)}
                          placeholder="acme"
                        />
                      </Field>
                      <Field label="Repository">
                        <input
                          className="input font-mono text-[12px]"
                          value={gitRepo}
                          onChange={(event) => setGitRepo(event.target.value)}
                          placeholder="cluster-manifests"
                        />
                      </Field>
                      <Field label="Base branch">
                        <input
                          className="input font-mono text-[12px]"
                          value={gitBaseBranch}
                          onChange={(event) => setGitBaseBranch(event.target.value)}
                        />
                      </Field>
                      <Field label="Manifests path">
                        <input
                          className="input font-mono text-[12px]"
                          value={gitPath}
                          onChange={(event) => setGitPath(event.target.value)}
                        />
                      </Field>
                    </div>
                    <label className="flex cursor-pointer items-center gap-2 text-[11px] text-ink-300">
                      <input
                        type="checkbox"
                        checked={autoMerge}
                        onChange={(event) => setAutoMerge(event.target.checked)}
                        className="h-4 w-4 accent-mint-500"
                      />
                      Auto-merge once required checks pass
                    </label>
                  </div>
                ) : null}
              </section>
            </div>

            <aside className="flex min-h-0 flex-col border-t border-white/[0.08] bg-ink-950/60 lg:border-l lg:border-t-0">
              <div className="flex items-center justify-between gap-3 border-b border-white/[0.08] px-5 py-3">
                <span className="eyebrow">Manifest preview</span>
                <button
                  type="button"
                  className="btn-ghost h-7 px-2.5 text-[10px]"
                  onClick={async () => {
                    try {
                      await navigator.clipboard.writeText(manifest);
                    } catch {
                      /* clipboard may be unavailable outside a secure context */
                    }
                  }}
                >
                  <Icon name="copy" size={11} />
                  Copy
                </button>
              </div>
              <pre className="max-h-[32rem] flex-1 overflow-auto p-5 font-mono text-[10.5px] leading-relaxed text-ink-300 scrollbar-thin">
                {manifest}
              </pre>
              <div className="border-t border-white/[0.08] px-5 py-3 font-mono text-[9px] uppercase leading-relaxed tracking-[0.12em] text-ink-600">
                Posted to /api/policies · stored as a custom resource
              </div>
            </aside>
          </div>

          <div className="flex flex-wrap items-center justify-between gap-3 border-t border-white/[0.08] px-5 py-4 sm:px-6">
            <div className="min-w-0 text-[11px] leading-relaxed text-ember-400">
              {submitError ? <span className="break-words">{submitError}</span> : null}
            </div>
            <div className="flex items-center gap-2">
              <button type="button" className="btn-ghost" onClick={onClose}>
                Cancel
              </button>
              <button type="submit" className="btn-primary" disabled={!canCreate || allowed.length === 0}>
                {creating ? "Creating…" : "Create policy"}
              </button>
            </div>
          </div>
        </form>
      </div>
    </div>
  );
}

function ChoiceButton({
  active,
  onClick,
  title,
  description,
}: {
  active: boolean;
  onClick: () => void;
  title: string;
  description: string;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      className={clsx(
        "rounded-xl border px-3 py-2.5 text-left transition",
        active
          ? "border-mint-500/45 bg-mint-500/[0.07]"
          : "border-white/[0.1] bg-transparent hover:border-white/20 hover:bg-white/[0.03]",
      )}
    >
      <div
        className={clsx(
          "font-mono text-[10px] uppercase tracking-[0.1em]",
          active ? "text-mint-500" : "text-ink-300",
        )}
      >
        {title}
      </div>
      <div className="mt-1 text-[10px] leading-snug text-ink-500">{description}</div>
    </button>
  );
}

function Field({
  label,
  hint,
  invalid,
  children,
}: {
  label: string;
  hint?: string;
  invalid?: boolean;
  children: React.ReactNode;
}) {
  return (
    <div className="block">
      <span className={clsx("field-label", invalid && "text-ember-400")}>{label}</span>
      {children}
      {hint ? (
        <span className={clsx("mt-1.5 block text-[10px] leading-snug", invalid ? "text-ember-400" : "text-ink-500")}>
          {hint}
        </span>
      ) : null}
    </div>
  );
}

function NumberField({
  label,
  value,
  min,
  max,
  onChange,
  suffix,
}: {
  label: string;
  value: number;
  min: number;
  max: number;
  onChange: (value: number) => void;
  suffix: string;
}) {
  return (
    <Field label={label}>
      <div className="relative">
        <input
          type="number"
          className="input pr-9 font-mono text-[12px]"
          value={value}
          min={min}
          max={max}
          onChange={(event) => {
            const next = Number(event.target.value);
            if (Number.isFinite(next)) onChange(Math.min(max, Math.max(min, next)));
          }}
        />
        <span className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 font-mono text-[10px] text-ink-500">
          {suffix}
        </span>
      </div>
    </Field>
  );
}

function stripUndefined(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(stripUndefined);
  if (value && typeof value === "object") {
    const out: Record<string, unknown> = {};
    for (const [key, entry] of Object.entries(value as Record<string, unknown>)) {
      if (entry === undefined) continue;
      out[key] = stripUndefined(entry);
    }
    return out;
  }
  return value;
}

function toYaml(value: unknown, indent = 0): string {
  const pad = " ".repeat(indent);

  if (value === null) return "null";
  if (typeof value === "string") {
    const needsQuotes = value === "" || /[:#\[\]{}",&*!|>'"%@`]/.test(value) || /^\s|\s$/.test(value);
    return needsQuotes ? JSON.stringify(value) : value;
  }
  if (typeof value === "number" || typeof value === "boolean") return String(value);

  if (Array.isArray(value)) {
    if (value.length === 0) return "[]";
    return value
      .map((entry) =>
        entry !== null && typeof entry === "object"
          ? `${pad}-\n${toYaml(entry, indent + 2)}`
          : `${pad}- ${toYaml(entry, indent + 2)}`,
      )
      .join("\n");
  }

  if (typeof value === "object") {
    const entries = Object.entries(value as Record<string, unknown>);
    if (entries.length === 0) return "{}";
    return entries
      .map(([key, entry]) => {
        if (entry !== null && typeof entry === "object") {
          const isEmpty = Array.isArray(entry) ? entry.length === 0 : Object.keys(entry).length === 0;
          if (isEmpty) return `${pad}${key}: ${Array.isArray(entry) ? "[]" : "{}"}`;
          return `${pad}${key}:\n${toYaml(entry, indent + 2)}`;
        }
        return `${pad}${key}: ${toYaml(entry, indent + 2)}`;
      })
      .join("\n");
  }

  return String(value);
}
