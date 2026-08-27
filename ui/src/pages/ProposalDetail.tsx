import { useParams, Link } from "react-router-dom";
import { useApi } from "@/lib/useApi";
import { api } from "@/lib/api";
import type { ProposalDetail } from "@/lib/types";
import BeforeAfterCard from "@/components/BeforeAfterCard";
import KpiCard from "@/components/KpiCard";
import { ApprovalChip, ConfidenceBadge, PhaseChip, RiskBadge } from "@/components/Badges";
import { formatCarbonGrams, formatCurrency, useAppStore } from "@/store/app";

export default function ProposalDetailPage() {
  const { name } = useParams();
  const pushToast = useAppStore((s) => s.pushToast);

  const { data, loading, error } = useApi<ProposalDetail, [string]>(api.proposals.detail, name ?? "");

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center text-ink-500">
        Loading proposal…
      </div>
    );
  }

  if (error || !data) {
    return (
      <div className="space-y-6">
        <Link to="/proposals" className="text-sm text-ink-500 hover:text-mint-400">
          ← Back to proposals
        </Link>
        <div className="panel">
          <div className="px-6 py-12 text-center">
            <div className="font-medium text-ember-400">Proposal not found</div>
            <div className="mt-1 text-sm text-ink-500">
              {error ?? `No proposal with name ${name ?? ""}.`}
            </div>
          </div>
        </div>
      </div>
    );
  }

  const p = data;

  async function approve() {
    try {
      await api.proposals.approve(p.summary.name);
      pushToast({ kind: "success", title: "Approved proposal", message: p.summary.name });
    } catch (e) {
      pushToast({ kind: "error", title: "Approve failed", message: e instanceof Error ? e.message : String(e) });
    }
  }
  async function reject() {
    try {
      await api.proposals.reject(p.summary.name);
      pushToast({ kind: "warn", title: "Rejected proposal", message: p.summary.name });
    } catch (e) {
      pushToast({ kind: "error", title: "Reject failed", message: e instanceof Error ? e.message : String(e) });
    }
  }
  async function rollback() {
    try {
      await api.proposals.rollback(p.summary.name);
      pushToast({ kind: "warn", title: "Rollback scheduled", message: p.summary.name });
    } catch (e) {
      pushToast({ kind: "error", title: "Rollback failed", message: e instanceof Error ? e.message : String(e) });
    }
  }

  return (
    <div className="space-y-6 animate-fade-in-up">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <Link to="/proposals" className="text-sm text-ink-500 hover:text-mint-400">← Back to proposals</Link>
          <h1 className="mt-1 font-display text-4xl tracking-tight">
            {p.summary.namespace}/{p.summary.workloadName}
          </h1>
          <div className="mt-2 flex flex-wrap items-center gap-2 text-sm text-ink-100/70">
            <span className="font-mono">{p.summary.workloadKind}</span>
            <PhaseChip phase={p.summary.phase} />
            <ApprovalChip state={p.summary.approvalState} />
            <span className="font-mono text-xs text-ink-500">
              created {new Date(p.summary.createdAt).toLocaleString()}
            </span>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <button className="btn-ghost hover:text-ember-400 hover:bg-ember-500/10 hover:border-ember-500/30" onClick={reject}>
            Reject
          </button>
          <button
            className="btn-ghost hover:border-ember-500/30 hover:bg-ember-500/10" onClick={rollback}>
            Rollback
          </button>
          <button className="btn-primary" onClick={approve}>
            Approve proposal
          </button>
        </div>
      </div>

      <section className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-4">
        <KpiCard
          eyebrow="Est. monthly savings"
          value={formatCurrency(Number(p.summary.estimatedSavings ?? 0))}
          sub="Per workload/month"
          accent="mint"
          icon="💲"
        />
        <KpiCard
          eyebrow="Carbon saved"
          value={p.summary.carbonSavedHuman ?? formatCarbonGrams(p.summary.carbonSavedGrams ?? 0)}
          sub="Estimated workload-month reduction"
          accent="amber"
          icon="🌿"
        />
        <KpiCard
          eyebrow="Confidence vs risk"
          value={`${p.summary.confidenceScore} %`}
          sub={
            <div className="flex flex-wrap gap-2">
              <ConfidenceBadge score={p.summary.confidenceScore} />
              <RiskBadge score={p.summary.riskScore} />
            </div>
          }
          accent="sky"
          icon="🎯"
        />
        <KpiCard
          eyebrow="RL weighted reward"
          value={p.predictedImpact.rlReward.toFixed(2)}
          sub={`rules: ${p.predictedImpact.ruleHits.join(", ")}`}
          accent="mint"
          icon="🧠"
        />
      </section>

      <section className="grid grid-cols-1 xl:grid-cols-3 gap-6">
        <div className="xl:col-span-2 space-y-6">
          <BeforeAfterCard
            title="Resource changes"
            rows={[
              { label: "CPU request", before: p.summary.cpuBefore ?? "—", after: p.summary.cpuAfter ?? "—", lowerIsBetter: true },
              { label: "Memory request", before: p.summary.memoryBefore ?? "—", after: p.summary.memoryAfter ?? "—", lowerIsBetter: true },
              { label: "Est. monthly cost", before: "current", after: formatCurrency(Number(p.summary.estimatedSavings ?? 0)), lowerIsBetter: false },
              { label: "Est. monthly carbon", before: "current", after: p.summary.carbonSavedHuman ?? formatCarbonGrams(p.summary.carbonSavedGrams ?? 0), lowerIsBetter: false },
            ]}
          />

          <div className="panel">
            <div className="panel-header">
              <div>
                <div className="eyebrow">Engine rationale</div>
                <div className="panel-title">Why did the engine recommend this?</div>
              </div>
            </div>
            <div className="px-6 py-5 text-ink-100/80 leading-relaxed space-y-4">
              <p>{p.rationale}</p>
              <div>
                <div className="text-xs uppercase tracking-widest text-ink-500 mb-2">Rules fired</div>
                <ul className="space-y-1 text-sm">
                  {p.predictedImpact.ruleHits.map((h) => (
                    <li key={h} className="font-mono text-mint-400">{h}</li>
                  ))}
                </ul>
              </div>
            </div>
          </div>

          <div className="panel">
            <div className="panel-header">
              <div>
                <div className="eyebrow">Governance log</div>
                <div className="panel-title">Proposal timeline</div>
              </div>
            </div>
            <ol className="relative border-l-2 border-white/5 ml-6 my-2">
              {(p.execution?.events ?? []).map((e, i) => (
                <li key={i} className="relative pl-8 pb-6 last:pb-4">
                  <span
                    className={`absolute -left-[9px] top-1.5 w-4 h-4 rounded-full border-4 border-ink-900 ${
                      e.level === "error" ? "bg-ember-400" : e.level === "warn" ? "bg-amber-400" : "bg-mint-400"
                    }`}
                    aria-hidden
                  />
                  <div className="flex flex-wrap items-baseline gap-3">
                    <span className="font-mono text-xs text-ink-500">
                      {new Date(e.at).toLocaleString()}
                    </span>
                    <span className="text-sm">{e.message}</span>
                  </div>
                </li>
              ))}
            </ol>
          </div>
        </div>

        <div className="space-y-6">
          <div className="panel">
            <div className="panel-header">
              <div>
                <div className="eyebrow">Execution state</div>
                <div className="panel-title">What happens next?</div>
              </div>
            </div>
            <div className="px-6 py-5 space-y-4 text-sm">
              <div>
                <div className="text-ink-500 text-xs">Snapshot before</div>
                <div className="font-mono">{p.execution?.snapshots?.before ?? "—"}</div>
              </div>
              <div>
                <div className="text-ink-500 text-xs">Snapshot after</div>
                <div className="font-mono">{p.execution?.snapshots?.after ?? "—"}</div>
              </div>
              <div>
                <div className="text-ink-500 text-xs">GitOps provider</div>
                <div className="font-mono">{p.gitOps?.provider ?? "mock"}</div>
                {p.gitOps?.mergeStatus ? (
                  <div className="mt-1 text-xs text-mint-400">{p.gitOps.mergeStatus}</div>
                ) : null}
              </div>
            </div>
          </div>

          <div className="panel">
            <div className="panel-header">
              <div>
                <div className="eyebrow">Predicted confidence</div>
                <div className="panel-title">Engine signals</div>
              </div>
            </div>
            <div className="px-6 py-5 space-y-5 text-sm">
              <SignalRow
                label="Forecast confidence"
                value={p.predictedImpact.forecastConfidence}
                goodHigher
              />
              <SignalRow
                label="Anomaly severity (low is better)"
                value={1 - p.predictedImpact.anomalySeverity}
                goodHigher
              />
              <SignalRow
                label="RL weighted reward (0–1)"
                value={p.predictedImpact.rlReward}
                goodHigher
              />
            </div>
          </div>
        </div>
      </section>
    </div>
  );
}

function SignalRow({
  label,
  value,
  goodHigher,
}: {
  label: string;
  value: number;
  goodHigher: boolean;
}) {
  const pct = Math.min(100, Math.max(0, Math.round(value * 100)));
  const color =
    pct >= 75 ? "bg-mint-400" : pct >= 45 ? "bg-amber-400" : "bg-ember-400";
  void goodHigher;
  return (
    <div>
      <div className="flex items-center justify-between text-xs text-ink-500">
        <span>{label}</span>
        <span>{pct} %</span>
      </div>
      <div className="mt-2 h-1.5 w-full rounded-full bg-white/5 overflow-hidden">
        <div className={`h-full ${color}`} style={{ width: `${pct}%` }} />
      </div>
    </div>
  );
}
