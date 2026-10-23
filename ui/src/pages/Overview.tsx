import { Link, useNavigate } from "react-router-dom";
import type { ProposalListItem, SavingsResponse } from "@/lib/types";
import KpiCard from "@/components/KpiCard";
import SavingsChart from "@/components/SavingsChart";
import DataTable from "@/components/DataTable";
import PageHeader from "@/components/PageHeader";
import Icon from "@/components/Icon";
import { ApprovalChip, PhaseChip, RecommendationLabel } from "@/components/Badges";
import { api } from "@/lib/api";
import { useApi } from "@/lib/useApi";
import { formatCarbonGrams, formatCurrency, useAppStore } from "@/store/app";
import clsx from "clsx";

const EMPTY_SAVINGS: SavingsResponse = {
  totalUSD: 0,
  totalGramsCO2: 0,
  totalCarbonHuman: "0 g CO₂",
  byCluster: {},
  byNs: {},
  byClusterGramsCO2: {},
  byNsGramsCO2: {},
  series: [],
  refreshed: "",
};

const IN_FLIGHT = new Set(["Scheduled", "Executing", "Verifying", "Stabilizing", "RollingBack"]);
const ATTENTION = new Set(["Failed", "RollbackRequired", "RollbackVerification"]);

export default function Overview() {
  const navigate = useNavigate();
  const autoRefresh = useAppStore((s) => s.autoRefresh);
  const refresh = useAppStore((s) => s.refresh);
  const goalUSD = useAppStore((s) => s.monthlySavingsGoalUSD);

  const savingsQuery = useApi(api.savings.get);
  const proposalsQuery = useApi(api.proposals.list, { limit: 100, sortBy: "-createdAt" });
  const policiesQuery = useApi(api.policies.list);
  const clustersQuery = useApi(api.clusters.list);
  const anomaliesQuery = useApi(() => api.anomalies.list(4));

  const savings = savingsQuery.data ?? EMPTY_SAVINGS;
  const proposals = proposalsQuery.data ?? [];
  const policies = policiesQuery.data ?? [];
  const clusters = clustersQuery.data ?? [];
  const anomalies = anomaliesQuery.data ?? [];

  const pending = proposals.filter((p) => p.approvalState === "Pending").length;
  const completed = proposals.filter((p) => p.phase === "Completed").length;
  const inFlight = proposals.filter((p) => IN_FLIGHT.has(p.phase)).length;
  const attention = proposals.filter((p) => ATTENTION.has(p.phase)).length;
  const scored = proposals.filter((p) => p.confidenceScore > 0);
  const avgConfidence = scored.length
    ? Math.round(scored.reduce((sum, p) => sum + p.confidenceScore, 0) / scored.length)
    : undefined;

  const totalUSD = savings.totalUSD ?? 0;
  const goalPct = Math.min(100, Math.max(0, Math.round((totalUSD / Math.max(1, goalUSD)) * 100)));
  const autoPolicies = policies.filter((p) => p.mode === "AutoApply").length;
  const carbonPolicies = policies.filter((p) => p.carbonWeight > 0).length;

  const errors = [savingsQuery.error, proposalsQuery.error, policiesQuery.error].filter(Boolean);

  const mix = [
    { label: "Awaiting review", value: pending, tone: "bg-amber-500" },
    { label: "In flight", value: inFlight, tone: "bg-sky-500" },
    { label: "Completed", value: completed, tone: "bg-mint-500" },
    { label: "Needs attention", value: attention, tone: "bg-ember-500" },
  ];
  const mixTotal = Math.max(1, proposals.length);
  const refreshedLabel = savings.refreshed
    ? new Date(savings.refreshed).toLocaleString()
    : "not reported";

  return (
    <div className="space-y-10 lg:space-y-14">
      <PageHeader
        index="01"
        eyebrow="Operator overview"
        title={
          <>
            Cut spend.{" "}
            <span className="editorial text-mint-500">Cut carbon.</span>
            <br className="hidden sm:block" /> Keep every pod running.
          </>
        }
        description="Reconcilyx turns telemetry into reviewable proposals, then applies approved request changes in place through Server-Side Apply — no restarts, no rollout."
        actions={
          <>
            <Link to="/proposals?approvalState=Pending" className="btn-primary">
              {pending > 0 ? `Review ${pending} pending` : "Review proposals"}
              <Icon name="arrow-right" size={14} />
            </Link>
            <Link to="/forecast" className="btn-ghost">
              Open what-if
            </Link>
          </>
        }
        meta={
          <>
            <span>Ledger refreshed {refreshedLabel}</span>
            <span>{autoRefresh ? "Auto-refresh 30s" : "Auto-refresh off"}</span>
            <span>{proposals.length} proposals tracked</span>
            <span>{policiesQuery.data ? `${policies.length} policies` : "policies not loaded"}</span>
          </>
        }
      />

      {errors.length > 0 ? (
        <div
          role="alert"
          className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-ember-500/30 bg-ember-500/[0.06] px-4 py-3 text-[12px] text-ember-300"
        >
          <span className="break-words">
            One or more control-plane queries failed. Showing the last successful data.
            <span className="ml-2 text-ink-400">{errors[0]}</span>
          </span>
          <button
            className="btn-ghost h-8 px-3 text-[11px]"
            onClick={() => {
              refresh();
              savingsQuery.refetch();
              proposalsQuery.refetch();
              policiesQuery.refetch();
            }}
          >
            Retry
          </button>
        </div>
      ) : null}

      <section
        aria-label="Portfolio summary"
        className="grid grid-cols-1 gap-px overflow-hidden rounded-2xl border border-white/[0.09] bg-white/[0.07] sm:grid-cols-2 xl:grid-cols-4"
      >
        <KpiCard
          className="bg-ink-900"
          label="Spend avoided"
          value={formatCurrency(totalUSD, 0)}
          detail="Monthly recurring savings from completed proposals."
          loading={savingsQuery.loading && !savingsQuery.data}
        />
        <KpiCard
          className="bg-ink-900"
          label="Carbon avoided"
          value={savings.totalCarbonHuman ?? formatCarbonGrams(savings.totalGramsCO2 ?? 0)}
          detail="Grid-intensity-adjusted reduction across the same proposals."
          loading={savingsQuery.loading && !savingsQuery.data}
        />
        <KpiCard
          className="bg-ink-900"
          label="Awaiting review"
          value={String(pending).padStart(2, "0")}
          detail={`${completed} completed · ${inFlight} in flight · ${attention} need attention`}
          loading={proposalsQuery.loading && !proposalsQuery.data}
        />
        <KpiCard
          className="bg-ink-900"
          label="Mean confidence"
          value={avgConfidence === undefined ? "—" : `${avgConfidence}%`}
          detail={
            scored.length
              ? `Blended across ${scored.length} scored proposals.`
              : "No scored proposals reported yet."
          }
          loading={proposalsQuery.loading && !proposalsQuery.data}
        />
      </section>

      <section className="grid gap-4 xl:grid-cols-3">
        <div className="panel xl:col-span-2">
          <div className="panel-header">
            <div>
              <div className="eyebrow">30-day ledger</div>
              <div className="panel-title mt-1">Savings, in dollars and in carbon.</div>
            </div>
            <div className="flex items-center gap-4 font-mono text-[9px] uppercase tracking-[0.14em] text-ink-500">
              <span className="flex items-center gap-2">
                <span className="h-px w-4 bg-mint-500" aria-hidden />
                USD
              </span>
              <span className="flex items-center gap-2">
                <span className="h-px w-4 border-t border-dashed border-ember-500" aria-hidden />
                CO₂
              </span>
            </div>
          </div>
          <div className="px-3 pb-4 pt-5 sm:px-5">
            <SavingsChart data={savings.series} height={288} />
          </div>
        </div>

        <div className="panel flex flex-col">
          <div className="panel-header">
            <div>
              <div className="eyebrow">Monthly target</div>
              <div className="panel-title mt-1">Against the goal you set.</div>
            </div>
            <Link
              to="/settings"
              className="font-mono text-[9px] uppercase tracking-[0.14em] text-ink-500 transition hover:text-mint-500"
            >
              Edit
            </Link>
          </div>
          <div className="flex flex-1 flex-col justify-between p-5 sm:p-6">
            <div>
              <div className="flex items-end justify-between gap-4">
                <div className="kpi-value">{goalPct}%</div>
                <div className="text-right font-mono text-[10px] leading-relaxed text-ink-500">
                  {formatCurrency(totalUSD, 0)}
                  <br />
                  of {formatCurrency(goalUSD, 0)}
                </div>
              </div>
              <div className="mt-5 h-[3px] w-full overflow-hidden rounded-full bg-white/[0.08]">
                <div
                  className="h-full rounded-full bg-mint-500 transition-all duration-700"
                  style={{ width: `${goalPct}%` }}
                />
              </div>
              <p className="mt-3 text-[11px] leading-relaxed text-ink-500">
                Target stored in this browser. Savings are sourced from the control plane ledger.
              </p>
            </div>

            <div className="mt-7 border-t border-white/[0.08] pt-5">
              <div className="flex items-center justify-between">
                <span className="eyebrow">Operating mix</span>
                <span className="font-mono text-[9px] text-ink-600">{proposals.length} total</span>
              </div>
              <ul className="mt-4 space-y-3">
                {mix.map((row) => (
                  <li key={row.label}>
                    <div className="flex items-center justify-between text-[12px]">
                      <span className="text-ink-300">{row.label}</span>
                      <span className="num text-ink-100">{row.value}</span>
                    </div>
                    <div className="mt-1.5 h-[2px] w-full bg-white/[0.06]">
                      <div
                        className={clsx("h-full", row.tone)}
                        style={{ width: `${Math.min(100, (row.value / mixTotal) * 100)}%` }}
                      />
                    </div>
                  </li>
                ))}
              </ul>
            </div>
          </div>
        </div>
      </section>

      <section className="grid gap-4 xl:grid-cols-3">
        <div className="panel xl:col-span-2">
          <div className="panel-header">
            <div>
              <div className="eyebrow">Recent activity</div>
              <div className="panel-title mt-1">Latest proposals.</div>
            </div>
            <Link to="/proposals" className="link-arrow font-mono text-[9px] uppercase tracking-[0.14em] text-ink-400 hover:text-mint-500">
              View all
              <Icon name="arrow-right" size={12} />
            </Link>
          </div>
          <DataTable<ProposalListItem>
            onRowClick={(row) =>
              navigate(
                `/proposals/${encodeURIComponent(row.name)}?namespace=${encodeURIComponent(row.namespace)}`,
              )
            }
            rows={proposals.slice(0, 6)}
            rowKey={(row) => `${row.namespace}/${row.name}`}
            empty={
              proposalsQuery.loading && !proposalsQuery.data
                ? "Loading proposals…"
                : "No proposals yet. Create a policy to start evaluating workloads."
            }
            columns={[
              {
                key: "workload",
                title: "Workload",
                render: (row) => (
                  <div className="min-w-[180px]">
                    <div className="text-[13px] text-ink-100">
                      {row.namespace}/{row.workloadName}
                    </div>
                    <div className="mt-0.5 font-mono text-[9px] uppercase tracking-[0.1em] text-ink-500">
                      {row.workloadKind} · {row.name}
                    </div>
                  </div>
                ),
              },
              {
                key: "recommendation",
                title: "Recommendation",
                render: (row) => <RecommendationLabel value={row.recommendation} />,
              },
              {
                key: "status",
                title: "Status",
                render: (row) => (
                  <div className="flex flex-wrap gap-1.5">
                    <PhaseChip phase={row.phase} />
                    <ApprovalChip state={row.approvalState} />
                  </div>
                ),
              },
              {
                key: "impact",
                title: "Est. impact",
                className: "text-right",
                render: (row) => (
                  <div className="text-right">
                    <div className="num text-[12px] text-mint-500">
                      {row.estimatedSavings ? `${formatCurrency(Number(row.estimatedSavings))}/mo` : "—"}
                    </div>
                    <div className="num mt-0.5 text-[10px] text-ink-500">
                      {row.carbonSavedHuman ?? "carbon not reported"}
                    </div>
                  </div>
                ),
              },
            ]}
          />
        </div>

        <div className="space-y-4">
          <div className="panel">
            <div className="panel-header">
              <div>
                <div className="eyebrow">Governance</div>
                <div className="panel-title mt-1">Policy posture.</div>
              </div>
              <Link
                to="/policies"
                className="font-mono text-[9px] uppercase tracking-[0.14em] text-ink-500 transition hover:text-mint-500"
              >
                Open
              </Link>
            </div>
            <div className="grid grid-cols-3 divide-x divide-white/[0.07] border-b border-white/[0.07]">
              <Stat label="Policies" value={policiesQuery.data ? String(policies.length) : "—"} />
              <Stat label="Auto-apply" value={policiesQuery.data ? String(autoPolicies) : "—"} />
              <Stat label="Carbon-led" value={policiesQuery.data ? String(carbonPolicies) : "—"} />
            </div>
            <div className="px-5 py-4">
              <div className="flex items-center justify-between">
                <span className="eyebrow">Fleet</span>
                <span className="font-mono text-[9px] text-ink-600">
                  {clustersQuery.data ? `${clusters.length} registered` : "not loaded"}
                </span>
              </div>
              <ul className="mt-3 space-y-2.5">
                {clusters.slice(0, 4).map((cluster) => (
                  <li key={cluster.name} className="flex items-center justify-between gap-3 text-[12px]">
                    <span className="flex min-w-0 items-center gap-2">
                      <span
                        className={clsx(
                          "status-dot shrink-0",
                          cluster.status === "ready" ? "bg-mint-500 text-mint-500" : "bg-amber-500 text-amber-500",
                        )}
                        aria-hidden
                      />
                      <span className="truncate text-ink-200">{cluster.name}</span>
                    </span>
                    <span className="font-mono text-[9px] uppercase tracking-[0.1em] text-ink-500">
                      {cluster.type} · {cluster.status || "unknown"}
                    </span>
                  </li>
                ))}
                {clustersQuery.data && clusters.length === 0 ? (
                  <li className="text-[12px] text-ink-500">No clusters registered.</li>
                ) : null}
                {!clustersQuery.data && clustersQuery.loading ? (
                  <li className="text-[12px] text-ink-500">Loading fleet…</li>
                ) : null}
              </ul>
            </div>
          </div>

          <div className="panel">
            <div className="panel-header">
              <div>
                <div className="eyebrow">Risk watch</div>
                <div className="panel-title mt-1">Elevated-risk proposals.</div>
              </div>
              <span className="font-mono text-[9px] uppercase tracking-[0.14em] text-ink-500">
                {anomalies.length} listed
              </span>
            </div>
            {anomalies.length === 0 ? (
              <div className="px-5 py-8 text-center text-[12px] leading-relaxed text-ink-500">
                {anomaliesQuery.loading
                  ? "Loading risk signals…"
                  : "No elevated-risk proposals reported."}
              </div>
            ) : (
              <ul className="divide-y divide-white/[0.055]">
                {anomalies.map((anomaly) => (
                  <li key={`${anomaly.proposal ?? anomaly.workload}-${anomaly.time}`} className="px-5 py-3.5">
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <div className="truncate text-[12px] text-ink-200">
                          {anomaly.namespace}/{anomaly.workload}
                        </div>
                        <div className="mt-0.5 font-mono text-[9px] uppercase tracking-[0.1em] text-ink-500">
                          {anomaly.metric}
                        </div>
                      </div>
                      <span
                        className={clsx(
                          "chip border",
                          anomaly.severity === "high"
                            ? "border-ember-500/35 bg-ember-500/10 text-ember-400"
                            : anomaly.severity === "medium"
                              ? "border-amber-500/35 bg-amber-500/10 text-amber-400"
                              : "border-white/10 bg-white/[0.03] text-ink-300",
                        )}
                      >
                        {anomaly.severity} · {anomaly.riskScore}
                      </span>
                    </div>
                    {anomaly.proposal ? (
                      <Link
                        to={`/proposals/${encodeURIComponent(anomaly.proposal)}?namespace=${encodeURIComponent(anomaly.namespace)}`}
                        className="link-arrow mt-2 inline-flex font-mono text-[9px] uppercase tracking-[0.12em] text-ink-500 hover:text-mint-500"
                      >
                        Inspect proposal
                        <Icon name="arrow-up-right" size={11} />
                      </Link>
                    ) : null}
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>
      </section>
    </div>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="px-4 py-4 text-center">
      <div className="num text-[22px] leading-none tracking-[-0.04em] text-ink-50">{value}</div>
      <div className="mt-2 font-mono text-[8px] uppercase tracking-[0.16em] text-ink-500">{label}</div>
    </div>
  );
}
