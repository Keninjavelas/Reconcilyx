import { useMemo } from "react";
import { Link, useNavigate } from "react-router-dom";
import PageHeader from "@/components/PageHeader";
import KpiCard from "@/components/KpiCard";
import SavingsChart from "@/components/SavingsChart";
import DataTable from "@/components/DataTable";
import Icon from "@/components/Icon";
import { ApprovalChip, PhaseChip } from "@/components/Badges";
import { api } from "@/lib/api";
import { useApi } from "@/lib/useApi";
import type { ProposalListItem, SavingsResponse } from "@/lib/types";
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

const SAVED_PHASES = new Set(["Stabilizing", "Completed"]);

function aggregateDaily(series: SavingsResponse["series"]) {
  const byDate = new Map<string, { date: string; usd: number; gramsCO2: number }>();
  for (const point of series) {
    const key = point.date.slice(0, 10);
    const entry = byDate.get(key) ?? { date: key, usd: 0, gramsCO2: 0 };
    entry.usd += Number(point.usd) || 0;
    entry.gramsCO2 += Number(point.gramsCO2) || 0;
    byDate.set(key, entry);
  }
  return [...byDate.values()].sort((a, b) => a.date.localeCompare(b.date));
}

function breakdownRows(
  usd: Record<string, number>,
  grams: Record<string, number> | undefined,
  limit?: number,
) {
  const rows = Object.entries(usd)
    .map(([label, value]) => ({
      label,
      usd: Number(value) || 0,
      grams: Number(grams?.[label]) || 0,
    }))
    .sort((a, b) => b.usd - a.usd);
  return limit ? rows.slice(0, limit) : rows;
}

export default function Savings() {
  const navigate = useNavigate();
  const autoRefresh = useAppStore((s) => s.autoRefresh);
  const savingsQuery = useApi(api.savings.get);
  const proposalsQuery = useApi(api.proposals.list, { limit: 200, sortBy: "-createdAt" });

  const savings = savingsQuery.data ?? EMPTY_SAVINGS;
  const proposals = proposalsQuery.data ?? [];

  const totalUSD = savings.totalUSD ?? 0;
  const totalGrams = savings.totalGramsCO2 ?? 0;

  const completed = proposals.filter((p) => p.phase === "Completed").length;
  const stabilizing = proposals.filter((p) => p.phase === "Stabilizing").length;
  const approved = proposals.filter((p) => p.approvalState === "Approved").length;

  const dailySeries = useMemo(() => aggregateDaily(savings.series), [savings.series]);
  const clusterRows = useMemo(
    () => breakdownRows(savings.byCluster, savings.byClusterGramsCO2),
    [savings.byCluster, savings.byClusterGramsCO2],
  );
  const namespaceRows = useMemo(
    () => breakdownRows(savings.byNs, savings.byNsGramsCO2, 8),
    [savings.byNs, savings.byNsGramsCO2],
  );

  const topProposals = useMemo(
    () =>
      proposals
        .filter(
          (p) =>
            Number(p.estimatedSavings) > 0 &&
            (SAVED_PHASES.has(p.phase) || p.approvalState === "Approved"),
        )
        .sort((a, b) => Number(b.estimatedSavings) - Number(a.estimatedSavings))
        .slice(0, 8),
    [proposals],
  );

  const intensity = totalUSD > 0 ? Math.round(totalGrams / totalUSD) : undefined;
  const errors = [savingsQuery.error, proposalsQuery.error].filter(Boolean);
  const refreshedLabel = savings.refreshed
    ? new Date(savings.refreshed).toLocaleString()
    : "not reported";

  return (
    <div className="space-y-10 lg:space-y-14">
      <PageHeader
        index="04"
        eyebrow="Cost & carbon ledger"
        title={
          <>
            A record of everything you{" "}
            <span className="editorial text-mint-500">didn&apos;t spend.</span>
          </>
        }
        description="Every value on this page is read from the control plane's proposal ledger: the expected monthly impact recorded on approved, stabilizing, and completed proposals. Nothing here is extrapolated client-side."
        actions={
          <Link to="/proposals?approvalState=Pending" className="btn-ghost">
            Review pending
            <Icon name="arrow-up-right" size={14} />
          </Link>
        }
        meta={
          <>
            <span>Ledger refreshed {refreshedLabel}</span>
            <span>{autoRefresh ? "Auto-refresh 30s" : "Auto-refresh off"}</span>
            <span>{approved} approved · {completed} completed</span>
          </>
        }
      />

      {errors.length > 0 ? (
        <div
          role="alert"
          className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-ember-500/30 bg-ember-500/[0.06] px-4 py-3 text-[12px] text-ember-300"
        >
          <span className="break-words">
            The savings ledger could not be fully refreshed.
            <span className="ml-2 text-ink-400">{errors[0]}</span>
          </span>
          <button
            className="btn-ghost h-8 px-3 text-[11px]"
            onClick={() => {
              savingsQuery.refetch();
              proposalsQuery.refetch();
            }}
          >
            Retry
          </button>
        </div>
      ) : null}

      <section
        aria-label="Savings summary"
        className="grid grid-cols-1 gap-px overflow-hidden rounded-2xl border border-white/[0.09] bg-white/[0.07] sm:grid-cols-2 xl:grid-cols-4"
      >
        <KpiCard
          className="bg-ink-900"
          label="Spend avoided"
          value={formatCurrency(totalUSD, 0)}
          detail="Expected monthly savings on approved, stabilizing, and completed proposals."
          loading={savingsQuery.loading && !savingsQuery.data}
        />
        <KpiCard
          className="bg-ink-900"
          label="Carbon avoided"
          value={savings.totalCarbonHuman ?? formatCarbonGrams(totalGrams)}
          detail="The same proposals, weighted by the grid carbon intensity the control plane recorded."
          loading={savingsQuery.loading && !savingsQuery.data}
        />
        <KpiCard
          className="bg-ink-900"
          label="Completed proposals"
          value={String(completed).padStart(2, "0")}
          detail={`${stabilizing} stabilizing · ${approved} approved and in the ledger.`}
          loading={proposalsQuery.loading && !proposalsQuery.data}
        />
        <KpiCard
          className="bg-ink-900"
          label="Carbon per dollar"
          value={intensity === undefined ? "—" : `${intensity.toLocaleString()} g / $`}
          detail={
            intensity === undefined
              ? "No savings recorded, so the ratio is undefined."
              : "Grams of CO₂ avoided per dollar saved. Lower means a cleaner dollar."
          }
          loading={savingsQuery.loading && !savingsQuery.data}
        />
      </section>

      <section className="panel">
        <div className="panel-header">
          <div>
            <div className="eyebrow">Daily ledger</div>
            <div className="panel-title mt-1">Savings by proposal date.</div>
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
          <SavingsChart data={dailySeries} height={320} />
        </div>
        <div className="border-t border-white/[0.08] px-5 py-3 font-mono text-[9px] uppercase leading-relaxed tracking-[0.12em] text-ink-600 sm:px-6">
          Points are grouped by the creation date recorded on each proposal. The series is not a billing statement.
        </div>
      </section>

      <section className="grid gap-4 xl:grid-cols-2">
        <BreakdownPanel
          eyebrow="Distribution"
          title="By cluster."
          rows={clusterRows}
          empty="No cluster has recorded savings yet."
        />
        <BreakdownPanel
          eyebrow="Distribution"
          title="By namespace."
          rows={namespaceRows}
          empty="No namespace has recorded savings yet."
          note={
            Object.keys(savings.byNs).length > namespaceRows.length
              ? `Showing the top ${namespaceRows.length} of ${Object.keys(savings.byNs).length} namespaces.`
              : undefined
          }
        />
      </section>

      <section className="panel">
        <div className="panel-header">
          <div>
            <div className="eyebrow">Largest line items</div>
            <div className="panel-title mt-1">The proposals carrying the ledger.</div>
          </div>
          <Link
            to="/proposals"
            className="link-arrow font-mono text-[9px] uppercase tracking-[0.14em] text-ink-400 hover:text-mint-500"
          >
            All proposals
            <Icon name="arrow-right" size={12} />
          </Link>
        </div>
        <DataTable<ProposalListItem>
          onRowClick={(row) =>
            navigate(
              `/proposals/${encodeURIComponent(row.name)}?namespace=${encodeURIComponent(row.namespace)}`,
            )
          }
          rows={topProposals}
          rowKey={(row) => `${row.namespace}/${row.name}`}
          empty={
            proposalsQuery.loading && !proposalsQuery.data
              ? "Loading proposals…"
              : "No approved or completed proposal reports a positive monthly saving yet."
          }
          columns={[
            {
              key: "workload",
              title: "Workload",
              render: (row) => (
                <div className="min-w-[170px]">
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
              key: "savings",
              title: "Monthly saving",
              className: "text-right",
              render: (row) => (
                <div className="num text-right text-[12px] text-mint-500">
                  {formatCurrency(Number(row.estimatedSavings))}
                </div>
              ),
            },
            {
              key: "carbon",
              title: "Carbon avoided",
              className: "text-right",
              render: (row) => (
                <div className="num text-right text-[11px] text-ink-300">
                  {row.carbonSavedHuman ?? "—"}
                </div>
              ),
            },
          ]}
        />
      </section>
    </div>
  );
}

function BreakdownPanel({
  eyebrow,
  title,
  rows,
  empty,
  note,
}: {
  eyebrow: string;
  title: string;
  rows: { label: string; usd: number; grams: number }[];
  empty: string;
  note?: string;
}) {
  const max = Math.max(1, ...rows.map((row) => row.usd));
  const total = rows.reduce((sum, row) => sum + row.usd, 0);

  return (
    <div className="panel flex flex-col">
      <div className="panel-header">
        <div>
          <div className="eyebrow">{eyebrow}</div>
          <div className="panel-title mt-1">{title}</div>
        </div>
        {rows.length > 0 ? (
          <span className="font-mono text-[10px] text-ink-400">{formatCurrency(total, 0)}</span>
        ) : null}
      </div>

      {rows.length === 0 ? (
        <div className="flex flex-1 items-center justify-center px-6 py-14 text-center text-[12px] leading-relaxed text-ink-500">
          {empty}
        </div>
      ) : (
        <ul className="space-y-5 px-5 py-6 sm:px-6">
          {rows.map((row) => (
            <li key={row.label}>
              <div className="flex items-baseline justify-between gap-4">
                <span className="truncate text-[12px] text-ink-200">{row.label || "unassigned"}</span>
                <span className="num shrink-0 text-[11px] text-ink-200">
                  {formatCurrency(row.usd)}
                </span>
              </div>
              <div className="mt-2 flex items-center gap-3">
                <div className="h-[3px] flex-1 overflow-hidden bg-white/[0.06]">
                  <div
                    className={clsx("h-full bg-mint-500")}
                    style={{ width: `${Math.max(2, (row.usd / max) * 100)}%` }}
                  />
                </div>
                <span className="w-[92px] shrink-0 text-right font-mono text-[9px] text-ink-500">
                  {formatCarbonGrams(row.grams)}
                </span>
              </div>
            </li>
          ))}
        </ul>
      )}

      {note ? (
        <div className="mt-auto border-t border-white/[0.08] px-5 py-3 font-mono text-[9px] uppercase tracking-[0.12em] text-ink-600 sm:px-6">
          {note}
        </div>
      ) : null}
    </div>
  );
}
