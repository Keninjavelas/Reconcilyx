import type { ProposalListItem, SavingsResponse } from "@/lib/types";
import KpiCard from "@/components/KpiCard";
import SavingsChart from "@/components/SavingsChart";
import DataTable from "@/components/DataTable";
import { ApprovalChip, ConfidenceBadge, PhaseChip, RiskBadge } from "@/components/Badges";
import { api } from "@/lib/api";
import { useApi } from "@/lib/useApi";
import { formatCarbonGrams, formatCurrency, useAppStore } from "@/store/app";
import { Link, useNavigate } from "react-router-dom";

const EMPTY_SAVINGS: SavingsResponse = {
  totalUSD: 0,
  totalGramsCO2: 0,
  totalCarbonHuman: "0 g CO₂",
  byCluster: {},
  byNs: {},
  byClusterGramsCO2: {},
  byNsGramsCO2: {},
  series: [],
  refreshed: new Date().toISOString(),
};

export default function Overview() {
  const navigate = useNavigate();
  const refreshEpoch = useAppStore((s) => s.refreshEpoch);
  const goalUSD = useAppStore((s) => s.monthlySavingsGoalUSD);
  const setShortcutsOpen = useAppStore((s) => s.setShortcutsOpen);

  const { data: savings } = useApi(api.savings.get);
  const { data: proposals } = useApi(api.proposals.list, { limit: 25, sortBy: "-createdAt" });
  const { data: anomalies } = useApi(() => api.anomalies.list(5));

  const finalSavings = savings ?? EMPTY_SAVINGS;
  const finalProposals = proposals ?? [];
  const finalAnomalies = anomalies ?? [];

  const pendingApproval = finalProposals.filter((p) => p.approvalState === "Pending").length;
  const completed = finalProposals.filter((p) => p.phase === "Completed").length;

  const currentUSD = finalSavings.totalUSD ?? 0;
  const goalPct = Math.min(100, Math.round((currentUSD / Math.max(1, goalUSD)) * 100));

  const BADGES = [
    {
      id: "b1",
      icon: "🏆",
      title: "First 10% Cut",
      desc: "Applied 1st optimization proposal",
      unlocked: completed >= 1,
    },
    {
      id: "b2",
      icon: "⚡",
      title: "SSA Master",
      desc: "3+ Zero-Downtime In-Place Pod Resizes",
      unlocked: completed >= 3,
    },
    {
      id: "b3",
      icon: "🌿",
      title: "Carbon Sentinel",
      desc: "Abated over 1kg of grid CO₂",
      unlocked: (finalSavings.totalGramsCO2 ?? 0) >= 1000,
    },
    {
      id: "b4",
      icon: "🎯",
      title: "Fleet Right-sizer",
      desc: "Evaluated 5+ active proposals",
      unlocked: finalProposals.length >= 5,
    },
  ];

  return (
    <div className="space-y-8 animate-fade-in-up">
      {/* Header & Quick Action Bar */}
      <section className="flex flex-wrap items-end justify-between gap-4 border-b border-white/5 pb-6">
        <div>
          <div className="eyebrow flex items-center gap-2">
            <span>Operator Console</span>
            <span>·</span>
            <span>{refreshEpoch > 0 ? `epoch ${refreshEpoch}` : "live"}</span>
          </div>
          <h1 className="mt-1 font-display text-4xl md:text-5xl tracking-tight">
            Optimizations running across your clusters.
          </h1>
          <p className="mt-2 max-w-2xl text-ink-100/70 text-sm">
            Reconcilyx cuts cost <em>and</em> carbon footprint by adjusting container requests
            in-place via Server-Side Apply (no pod restarts, zero downtime).
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Link to="/proposals?approvalState=Pending" className="btn-primary">
            Review {pendingApproval} pending
          </Link>
          <Link to="/forecast" className="btn-ghost">
            📊 What-If Simulator
          </Link>
          <Link to="/policies" className="btn-ghost">
            🛡️ Policies
          </Link>
          <button
            onClick={() => setShortcutsOpen(true)}
            className="btn-ghost text-xs font-mono"
            title="Keyboard shortcuts (?)"
          >
            ⌨️ ?
          </button>
        </div>
      </section>

      {/* KPI Cards */}
      <section className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-4">
        <KpiCard
          eyebrow="Saved this month"
          value={formatCurrency(currentUSD)}
          sub="Monthly recurring savings from all Completed proposals."
          accent="mint"
          icon="💲"
        />
        <KpiCard
          eyebrow="Carbon saved this month"
          value={
            finalSavings.totalCarbonHuman ??
            formatCarbonGrams(finalSavings.totalGramsCO2 ?? 0)
          }
          sub="Grid-intensity-aware CO₂ reduction across applied proposals."
          accent="amber"
          icon="🌿"
        />
        <KpiCard
          eyebrow="Pending approvals"
          value={pendingApproval}
          sub={`${completed} proposals completed successfully this cycle.`}
          accent="sky"
          icon="🗂"
        />
        <KpiCard
          eyebrow="Avg. recommendation confidence"
          value={
            finalProposals.length > 0
              ? `${Math.round(
                  finalProposals.reduce((a, p) => a + (p.confidenceScore ?? 0), 0) /
                    finalProposals.length,
                )} %`
              : "—"
          }
          sub="Blended confidence score across active proposals."
          accent="mint"
          icon="🎯"
        />
      </section>

      {/* Monthly Target Progress & Achievement Badges */}
      <section className="grid grid-cols-1 xl:grid-cols-3 gap-6">
        {/* Savings Goal Card */}
        <div className="panel p-6 flex flex-col justify-between">
          <div>
            <div className="eyebrow">Monthly Goal Progress</div>
            <h3 className="panel-title mt-1">Target: {formatCurrency(goalUSD)} / mo</h3>
            <p className="text-xs text-ink-500 mt-1">
              Currently achieved {goalPct}% of target cost optimization goal.
            </p>
          </div>
          <div className="mt-6 space-y-2">
            <div className="flex items-center justify-between text-xs font-mono">
              <span className="text-mint-400 font-bold">{formatCurrency(currentUSD)}</span>
              <span className="text-ink-500">{goalPct}% achieved</span>
            </div>
            <div className="h-3 w-full rounded-full bg-white/5 overflow-hidden p-0.5 border border-white/10">
              <div
                className="h-full rounded-full bg-gradient-to-r from-mint-500 via-sky-400 to-amber-400 transition-all duration-1000 shadow-glow"
                style={{ width: `${goalPct}%` }}
              />
            </div>
          </div>
        </div>

        {/* Gamification Badges */}
        <div className="panel xl:col-span-2 p-6">
          <div className="flex items-center justify-between mb-4">
            <div>
              <div className="eyebrow">Gamification &amp; Milestones</div>
              <h3 className="panel-title mt-0.5">FinOps Achievement Badges</h3>
            </div>
            <span className="text-xs text-ink-500">
              {BADGES.filter((b) => b.unlocked).length} / {BADGES.length} Unlocked
            </span>
          </div>
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
            {BADGES.map((badge) => (
              <div
                key={badge.id}
                className={`p-3 rounded-xl border transition-all ${
                  badge.unlocked
                    ? "bg-mint-500/10 border-mint-500/30 text-mint-100"
                    : "bg-white/[0.02] border-white/5 text-ink-500 opacity-60"
                }`}
              >
                <div className="text-2xl">{badge.icon}</div>
                <div className="font-semibold text-xs mt-2">{badge.title}</div>
                <div className="text-[10px] text-ink-400 mt-0.5 leading-tight">
                  {badge.desc}
                </div>
                <div className="mt-2 text-[9px] uppercase font-mono tracking-wider">
                  {badge.unlocked ? "✅ Unlocked" : "🔒 Locked"}
                </div>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* 30-Day Savings Chart & Cluster Breakdown */}
      <section className="grid grid-cols-1 xl:grid-cols-3 gap-6">
        <div className="panel xl:col-span-2">
          <div className="panel-header">
            <div>
              <div className="eyebrow">30-day savings</div>
              <div className="panel-title">Cost &amp; CO₂ saved, side by side.</div>
            </div>
            <div className="flex items-center gap-2 text-xs text-ink-500">
              <span className="w-2 h-2 rounded-full bg-mint-400" /> USD
              <span className="ml-3 w-2 h-2 rounded-full bg-amber-400" /> CO₂
            </div>
          </div>
          <div className="px-6 pb-6">
            <SavingsChart data={finalSavings.series} />
          </div>
        </div>

        <div className="panel">
          <div className="panel-header">
            <div>
              <div className="eyebrow">Breakdown by cluster</div>
              <div className="panel-title">Where savings come from.</div>
            </div>
          </div>
          <ul className="divide-y divide-white/5">
            {Object.entries(finalSavings.byCluster).map(([cluster, usd]) => {
              const carbon = finalSavings.byClusterGramsCO2?.[cluster] ?? 0;
              return (
                <li key={cluster} className="px-6 py-4">
                  <div className="flex items-center justify-between gap-4">
                    <div>
                      <div className="font-medium text-sm">{cluster}</div>
                      <div className="text-xs text-ink-500 mt-0.5">
                        {formatCarbonGrams(carbon)} saved
                      </div>
                    </div>
                    <div className="font-mono text-mint-400 font-semibold">{formatCurrency(usd)}</div>
                  </div>
                  <div className="mt-3 h-1.5 w-full rounded-full bg-white/5 overflow-hidden">
                    <div
                      className="h-full bg-gradient-to-r from-mint-500 to-sky-500"
                      style={{
                        width: `${Math.min(
                          100,
                          Math.round(
                            ((usd ?? 0) / Math.max(1, finalSavings.totalUSD)) * 100,
                          ),
                        )}%`,
                      }}
                    />
                  </div>
                </li>
              );
            })}
            {Object.keys(finalSavings.byCluster).length === 0 ? (
              <li className="px-6 py-6 text-sm text-ink-500">
                No cluster data yet. Create proposals to see a breakdown.
              </li>
            ) : null}
          </ul>
        </div>
      </section>

      {/* Recent Anomalies & Proposals */}
      <section className="grid grid-cols-1 xl:grid-cols-3 gap-6">
        {/* Proposals List */}
        <div className="panel xl:col-span-2">
          <div className="panel-header">
            <div>
              <div className="eyebrow">Recent</div>
              <div className="panel-title">Latest optimization proposals</div>
            </div>
            <Link to="/proposals" className="btn-ghost text-xs">
              View all proposals →
            </Link>
          </div>
          <DataTable<ProposalListItem>
            onRowClick={(r) => navigate(`/proposals/${r.name}`)}
            rows={finalProposals.slice(0, 6)}
            rowKey={(r) => r.name}
            empty="No proposals yet."
            columns={[
              {
                key: "workloadName",
                title: "Workload",
                render: (r) => (
                  <div>
                    <div className="font-medium text-sm">
                      {r.namespace}/{r.workloadName}
                    </div>
                    <div className="text-xs text-ink-500 font-mono">
                      {r.workloadKind} · {r.name}
                    </div>
                  </div>
                ),
              },
              { key: "recommendation", title: "Recommendation" },
              {
                key: "phase",
                title: "Status",
                render: (r) => (
                  <div className="flex flex-wrap items-center gap-2">
                    <PhaseChip phase={r.phase} />
                    <ApprovalChip state={r.approvalState} />
                  </div>
                ),
              },
              {
                key: "scores",
                title: "Safety",
                render: (r) => (
                  <div className="flex flex-wrap items-center gap-2">
                    <ConfidenceBadge score={r.confidenceScore} />
                    <RiskBadge score={r.riskScore} />
                  </div>
                ),
              },
              {
                key: "impact",
                title: "Impact",
                render: (r) => (
                  <div className="text-right">
                    {r.estimatedSavings ? (
                      <div className="font-mono text-mint-400 font-semibold">
                        {formatCurrency(Number(r.estimatedSavings))}/mo
                      </div>
                    ) : null}
                    {r.carbonSavedHuman ? (
                      <div className="text-xs text-amber-400 mt-0.5">
                        {r.carbonSavedHuman}/mo
                      </div>
                    ) : null}
                  </div>
                ),
                className: "text-right",
              },
            ]}
          />
        </div>

        {/* Anomalies Card */}
        <div className="panel p-6 space-y-4">
          <div className="flex items-center justify-between border-b border-white/5 pb-3">
            <div>
              <div className="eyebrow">Anomaly Detector</div>
              <h3 className="panel-title text-base mt-0.5">Detected Spikes</h3>
            </div>
            <span className="text-xs font-mono text-amber-400">
              {finalAnomalies.length} active
            </span>
          </div>

          <div className="space-y-3">
            {finalAnomalies.map((anom, idx) => (
              <div
                key={idx}
                className="p-3 rounded-xl bg-white/[0.02] border border-white/5 space-y-1 text-xs"
              >
                <div className="flex items-center justify-between">
                  <span className="font-mono font-medium text-white">
                    {anom.namespace}/{anom.workload}
                  </span>
                  <span
                    className={`px-2 py-0.5 rounded text-[10px] font-bold uppercase ${
                      anom.severity === "high"
                        ? "bg-ember-500/20 text-ember-400 border border-ember-500/30"
                        : "bg-amber-500/20 text-amber-400 border border-amber-500/30"
                    }`}
                  >
                    {anom.severity} risk ({anom.riskScore})
                  </span>
                </div>
                <div className="text-ink-400">Metric: {anom.metric}</div>
              </div>
            ))}

            {finalAnomalies.length === 0 && (
              <div className="text-xs text-ink-500 py-6 text-center">
                ✅ No active resource anomalies detected. Fleet running optimal.
              </div>
            )}
          </div>
        </div>
      </section>
    </div>
  );
}
