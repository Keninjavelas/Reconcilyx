import { useMemo, useState } from "react";
import DataTable from "@/components/DataTable";
import { useApi } from "@/lib/useApi";
import { api } from "@/lib/api";
import type { PolicyListItem } from "@/lib/types";
import clsx from "clsx";
import { useAppStore } from "@/store/app";

export default function Policies() {
  const { data } = useApi(api.policies.list);
  const policies = data ?? [];
  const pushToast = useAppStore((s) => s.pushToast);
  const [showWizard, setShowWizard] = useState(false);

  return (
    <div className="space-y-6 animate-fade-in-up">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <div className="eyebrow">Policy catalogue</div>
          <h1 className="mt-1 font-display text-4xl tracking-tight">Policies</h1>
          <p className="mt-2 max-w-2xl text-ink-100/70">
            OptimizationPolicies bind your workloads to rules. Each policy carries its own
            carbonWeight — turning cost-first and carbon-first dials independently.
          </p>
        </div>
        <button className="btn-primary" onClick={() => setShowWizard(true)}>
          + New policy
        </button>
      </div>

      <div className="panel">
        <DataTable<PolicyListItem>
          rows={policies}
          rowKey={(r) => `${r.namespace}/${r.name}`}
          empty="No OptimizationPolicies yet. Create one to start generating proposals."
          columns={[
            {
              key: "name",
              title: "Policy",
              render: (r) => (
                <div>
                  <div className="font-medium">
                    {r.namespace}/{r.name}
                  </div>
                  <div className="text-xs font-mono text-ink-500">
                    created {new Date(r.createdAt).toLocaleDateString()}
                  </div>
                </div>
              ),
            },
            {
              key: "mode",
              title: "Mode",
              render: (r) => (
                <ModeChip mode={r.mode} />
              ),
            },
            {
              key: "workloadsMatched",
              title: "Workloads matched",
              className: "text-right",
              render: (r) => (
                <div className="text-right font-mono">{r.workloadsMatched}</div>
              ),
            },
            {
              key: "proposalsActive",
              title: "Active proposals",
              className: "text-right",
              render: (r) => (
                <div className="text-right font-mono">{r.proposalsActive}</div>
              ),
            },
            {
              key: "carbonWeight",
              title: "Carbon weight",
              render: (r) => (
                <div>
                  <div className="flex items-center justify-between text-xs text-ink-500 mb-1">
                    <span>cost ↔ carbon</span>
                    <span className="font-mono">{r.carbonWeight.toFixed(2)}</span>
                  </div>
                  <div className="h-1.5 w-full rounded-full bg-white/5 overflow-hidden relative">
                    <div className="absolute inset-0 bg-gradient-to-r from-mint-500/40 via-ink-900 to-amber-500/40" />
                    <div
                      className="absolute top-[-3px] w-2 h-4 rounded-full border-2 border-ink-900 bg-white"
                      style={{ left: `calc(${r.carbonWeight * 100}% - 4px)` }}
                    />
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
          onCreate={async (body) => {
            try {
              const created = await api.policies.create(body);
              const ns = created.metadata?.namespace ?? body.metadata.namespace;
              const nm = created.metadata?.name ?? body.metadata.name;
              pushToast({
                kind: "success",
                title: "Policy created",
                message: `${ns}/${nm}`,
              });
              setShowWizard(false);
            } catch (e) {
              pushToast({
                kind: "error",
                title: "Policy create failed",
                message: e instanceof Error ? e.message : String(e),
              });
            }
          }}
        />
      ) : null}
    </div>
  );
}

function ModeChip({ mode }: { mode: PolicyListItem["mode"] }) {
  const map: Record<string, string> = {
    Optimize: "bg-white/[0.04] text-mint-400 border-mint-500/30",
    AnalyzeOnly: "bg-white/[0.04] text-sky-400 border-sky-500/30",
    AutoApply: "bg-amber-500/10 text-amber-400 border-amber-500/30",
  };
  const cls = map[mode] ?? "bg-white/[0.04] text-ink-100/70 border-white/10";
  return (
    <span className={clsx("chip border", cls)}>
      {mode}
    </span>
  );
}

type Body = {
  metadata: { name: string; namespace: string };
  spec?: any;
};

function CreatePolicyWizard({
  onClose,
  onCreate,
}: {
  onClose: () => void;
  onCreate: (b: Body) => Promise<void>;
}) {
  const [step, setStep] = useState(0);
  const [name, setName] = useState("cost-and-carbon-policy");
  const [namespace, setNamespace] = useState("production");
  const [mode, setMode] = useState<PolicyListItem["mode"]>("Optimize");
  const [nsLabelKey, setNsLabelKey] = useState("kubernetes.io/metadata.name");
  const [nsLabelValue, setNsLabelValue] = useState("production");
  const [wklLabelKey, setWklLabelKey] = useState("app");
  const [wklLabelValue, setWklLabelValue] = useState("*");
  const [carbonWeight, setCarbonWeight] = useState(0.3);
  const [maxIncreasePct, setMaxIncreasePct] = useState(10);
  const [stabilizationWindow, setStabilizationWindow] = useState(30);
  const [minConfidence, setMinConfidence] = useState(60);
  const [maxRiskScore, setMaxRiskScore] = useState(60);
  const [minReplicas, setMinReplicas] = useState(2);
  const [gitOpsProvider, setGitOpsProvider] = useState<"mock" | "github" | "gitlab" | "ado">("mock");
  const [autoMerge, setAutoMerge] = useState(true);
  const pushToast = useAppStore((s) => s.pushToast);

  const steps = [
    "Name & mode",
    "Target workloads",
    "Carbon & safety",
    "Risk analysis",
    "Rollback guard",
    "GitOps",
    "Review YAML",
  ];

  const preview: Body = useMemo(() => ({
    metadata: { name, namespace },
    spec: {
      mode,
      targetSelectors: {
        namespaceMatchLabels: { [nsLabelKey]: nsLabelValue },
        workloadLabelSelector: {
          matchLabels: wklLabelValue === "*" ? undefined : { [wklLabelKey]: wklLabelValue },
        },
      },
      safetyConstraints: {
        carbonWeight,
        stabilizationWindowMinutes: stabilizationWindow,
        maxIncreasePct,
        maxRiskScore,
        killSwitch: true,
      },
      analysis: {
        minConfidence: minConfidence / 100,
        minReplicas,
      },
      gitOps: {
        enabled: gitOpsProvider !== "mock" ? true : false,
        provider: gitOpsProvider,
        autoMerge,
      },
    },
  } as unknown as Body), [
    name, namespace, mode, nsLabelKey, nsLabelValue, wklLabelKey, wklLabelValue,
    carbonWeight, maxIncreasePct, stabilizationWindow, minConfidence, maxRiskScore,
    minReplicas, gitOpsProvider, autoMerge,
  ]);

  const yaml = useMemo(
    () => buildYaml(preview),
    [preview],
  );

  async function submit() {
    try {
      await onCreate(preview);
    } catch (e) {
      pushToast({
        kind: "error",
        title: "Submit failed",
        message: e instanceof Error ? e.message : String(e),
      });
    }
  }

  return (
    <div className="fixed inset-0 z-50 bg-ink-950/80 backdrop-blur-sm flex items-center justify-center p-4 animate-fade-in-up">
      <div className="panel w-full max-w-3xl max-h-[90vh] overflow-y-auto">
        <div className="panel-header">
          <div>
            <div className="eyebrow">
              Step {step + 1} of {steps.length}
            </div>
            <div className="panel-title">{steps[step]}</div>
          </div>
          <button className="btn-ghost" onClick={onClose}>
            ✕
          </button>
        </div>

        <div className="px-6 mb-4">
          <div className="h-1 w-full rounded-full bg-white/5 overflow-hidden">
            <div
              className="h-full bg-gradient-to-r from-mint-500 to-sky-500 transition-all duration-300"
              style={{ width: `${((step + 1) / steps.length) * 100}%` }}
            />
          </div>
        </div>

        <div className="px-6 pb-6 space-y-5">
          {step === 0 ? (
            <>
              <Field label="Policy name">
                <input className="input" value={name} onChange={(e) => setName(e.target.value)} />
              </Field>
              <Field label="Namespace">
                <input className="input" value={namespace} onChange={(e) => setNamespace(e.target.value)} />
              </Field>
              <Field label="Mode">
                <div className="grid grid-cols-3 gap-2">
                  {(["AnalyzeOnly", "Optimize", "AutoApply"] as const).map((m) => (
                    <button
                      key={m}
                      onClick={() => setMode(m)}
                      className={clsx(
                        "rounded-xl border px-4 py-3 text-left transition-all",
                        mode === m
                          ? "bg-white/[0.04] border-mint-500/30 text-mint-400"
                          : "border-white/5 hover:bg-white/[0.03]",
                      )}
                    >
                      <div className="font-medium">{m}</div>
                      <div className="text-xs text-ink-500 mt-0.5">
                        {m === "AnalyzeOnly"
                          ? "Report only, never act."
                          : m === "Optimize"
                            ? "Human review before apply."
                            : "Auto-approve low-risk changes."}
                      </div>
                    </button>
                  ))}
                </div>
              </Field>
            </>
          ) : null}

          {step === 1 ? (
            <>
              <Field label="Namespace match label key">
                <input className="input" value={nsLabelKey} onChange={(e) => setNsLabelKey(e.target.value)} />
              </Field>
              <Field label="Namespace match label value">
                <input className="input" value={nsLabelValue} onChange={(e) => setNsLabelValue(e.target.value)} />
              </Field>
              <Field label="Workload match label key">
                <input className="input" value={wklLabelKey} onChange={(e) => setWklLabelKey(e.target.value)} />
              </Field>
              <Field label="Workload match label value (use * for all)">
                <input className="input" value={wklLabelValue} onChange={(e) => setWklLabelValue(e.target.value)} />
              </Field>
            </>
          ) : null}

          {step === 2 ? (
            <>
              <Field label="Carbon weight — cost (0.00) ↔ CO₂ (1.00)">
                <div className="mt-2 font-mono text-lg text-center">{carbonWeight.toFixed(2)}</div>
                <input
                  type="range"
                  min={0}
                  max={1}
                  step={0.05}
                  className="slider w-full"
                  value={carbonWeight}
                  onChange={(e) => setCarbonWeight(Number(e.target.value))}
                />
                <div className="mt-2 flex justify-between text-xs text-ink-500">
                  <span>Cost-first (0.00)</span>
                  <span>Balanced (0.30 default)</span>
                  <span>Carbon-first (1.00)</span>
                </div>
                <div className="mt-3 text-sm text-ink-100/70 leading-relaxed panel !border-white/5 !bg-white/[0.02] !p-4">
                  Multi-objective reward:{" "}
                  <span className="font-mono text-mint-400">
                    (1−w)·σ(costSaved/50) + w·σ(carbonSaved/50000)
                  </span>
                  . Raising w makes the RL prefer carbon-saving recommendations even when the
                  USD payback is slower.
                </div>
              </Field>
              <Field label="Max CPU/memory increase %">
                <input
                  type="number"
                  className="input"
                  value={maxIncreasePct}
                  onChange={(e) => setMaxIncreasePct(Number(e.target.value))}
                />
              </Field>
              <Field label={`Stabilization window (${stabilizationWindow} min)`}>
                <input
                  type="number"
                  className="input"
                  value={stabilizationWindow}
                  onChange={(e) => setStabilizationWindow(Number(e.target.value))}
                />
              </Field>
            </>
          ) : null}

          {step === 3 ? (
            <>
              <Field label={`Min. forecast confidence (${minConfidence} %)`}>
                <input
                  type="range"
                  min={0}
                  max={100}
                  className="slider w-full"
                  value={minConfidence}
                  onChange={(e) => setMinConfidence(Number(e.target.value))}
                />
              </Field>
              <Field label={`Max risk score allowed (${maxRiskScore})`}>
                <input
                  type="range"
                  min={0}
                  max={100}
                  className="slider w-full"
                  value={maxRiskScore}
                  onChange={(e) => setMaxRiskScore(Number(e.target.value))}
                />
              </Field>
              <Field label={`Min replicas to consider (${minReplicas})`}>
                <input
                  type="number"
                  min={1}
                  className="input"
                  value={minReplicas}
                  onChange={(e) => setMinReplicas(Number(e.target.value))}
                />
              </Field>
            </>
          ) : null}

          {step === 4 ? (
            <>
              <Field label="Rollback guardrails">
                <div className="panel !border-white/5 !bg-white/[0.02] !p-5 space-y-2 text-sm text-ink-100/80">
                  <p>✓ In-place SSA rollback to pre-execution snapshot within 10 seconds.</p>
                  <p>✓ Auto-rollback on error-rate increase &gt; 10 % (15 min window).</p>
                  <p>✓ Auto-rollback on P95 latency &gt; safety.maxIncreasePct.</p>
                  <p>✓ Kill-switch: `reconcilyx/reject-all: "true"` namespace label.</p>
                </div>
              </Field>
            </>
          ) : null}

          {step === 5 ? (
            <>
              <Field label="GitOps provider">
                <div className="grid grid-cols-2 md:grid-cols-4 gap-2">
                  {(["mock", "github", "gitlab", "ado"] as const).map((p) => (
                    <button
                      key={p}
                      onClick={() => setGitOpsProvider(p)}
                      className={clsx(
                        "rounded-xl border px-4 py-3 text-left transition-all",
                        gitOpsProvider === p
                          ? "bg-white/[0.04] border-mint-500/30 text-mint-400"
                          : "border-white/5 hover:bg-white/[0.03]",
                      )}
                    >
                      <div className="font-medium uppercase tracking-wide text-xs">{p}</div>
                      <div className="text-xs text-ink-500 mt-1">
                        {p === "mock" ? "Disabled / dry-run" : "Open PRs on change"}
                      </div>
                    </button>
                  ))}
                </div>
              </Field>
              <Field label="Auto-merge on approval + CI green">
                <label className="flex items-center gap-3 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={autoMerge}
                    onChange={(e) => setAutoMerge(e.target.checked)}
                    className="w-4 h-4 accent-mint-500"
                  />
                  <span className="text-sm text-ink-100/80">Arm merge-queue arming</span>
                </label>
              </Field>
            </>
          ) : null}

          {step === 6 ? (
            <>
              <Field label="Preview manifest">
                <div className="relative">
                  <button
                    onClick={() => {
                      navigator.clipboard.writeText(yaml);
                      pushToast({ kind: "success", title: "Copied YAML", message: "CRD manifest copied to clipboard!" });
                    }}
                    className="absolute top-2 right-2 px-3 py-1 text-xs rounded-lg bg-white/10 hover:bg-white/20 text-white font-medium transition"
                  >
                    📋 Copy YAML
                  </button>
                  <pre className="overflow-x-auto font-mono text-xs p-4 rounded-xl bg-black/40 border border-white/10 text-ink-100/90 max-h-[420px]">
                    {yaml}
                  </pre>
                </div>
              </Field>
            </>
          ) : null}
        </div>

        <div className="px-6 pb-6 flex flex-wrap items-center justify-between gap-3">
          <button
            className="btn-ghost"
            onClick={() => (step > 0 ? setStep(step - 1) : onClose())}
          >
            {step > 0 ? "← Back" : "Cancel"}
          </button>
          {step < steps.length - 1 ? (
            <button className="btn-primary" onClick={() => setStep(step + 1)}>
              Continue →
            </button>
          ) : (
            <button className="btn-primary" onClick={submit}>
              Create policy
            </button>
          )}
        </div>
      </div>
    </div>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <div className="text-xs uppercase tracking-widest text-ink-500 mb-2">{label}</div>
      {children}
    </div>
  );
}

function buildYaml(p: Body): string {
  const lines: string[] = [];
  lines.push("apiVersion: reconcilyx.io/v1alpha1");
  lines.push("kind: OptimizationPolicy");
  lines.push("metadata:");
  lines.push(`  name: ${p.metadata.name}`);
  lines.push(`  namespace: ${p.metadata.namespace}`);
  lines.push("spec:");
  const spec = p.spec as NonNullable<Body["spec"]>;
  lines.push(`  mode: ${spec.mode}`);
  lines.push("  targetSelectors:");
  lines.push("    namespaceMatchLabels:");
  for (const [k, v] of Object.entries(spec.targetSelectors?.namespaceMatchLabels ?? {})) {
    lines.push(`      ${k}: ${v}`);
  }
  if (spec.targetSelectors?.workloadLabelSelector?.matchLabels) {
    lines.push("    workloadLabelSelector:");
    lines.push("      matchLabels:");
    for (const [k, v] of Object.entries(spec.targetSelectors.workloadLabelSelector.matchLabels)) {
      lines.push(`        ${k}: ${v}`);
    }
  }
  lines.push("  safetyConstraints:");
  lines.push(`    carbonWeight: ${(spec.safetyConstraints?.carbonWeight ?? 0.3).toFixed(2)}`);
  lines.push(`    maxIncreasePct: ${spec.safetyConstraints?.maxIncreasePct ?? 10}`);
  lines.push(`    stabilizationWindowMinutes: ${spec.safetyConstraints?.stabilizationWindowMinutes ?? 30}`);
  lines.push(`    maxRiskScore: ${spec.safetyConstraints?.maxRiskScore ?? 60}`);
  lines.push("  analysis:");
  lines.push(`    minConfidence: ${Number(spec.analysis?.minConfidence ?? 0.6).toFixed(2)}`);
  lines.push(`    minReplicas: ${spec.analysis?.minReplicas ?? 2}`);
  lines.push("  gitOps:");
  lines.push(`    enabled: ${spec.gitOps?.enabled ? "true" : "false"}`);
  lines.push(`    provider: ${spec.gitOps?.provider ?? "mock"}`);
  lines.push(`    autoMerge: ${spec.gitOps?.autoMerge ? "true" : "false"}`);
  return lines.join("\n");
}
