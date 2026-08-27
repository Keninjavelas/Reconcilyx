import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from "recharts";
import KpiCard from "@/components/KpiCard";
import SavingsChart from "@/components/SavingsChart";
import { useApi } from "@/lib/useApi";
import { api } from "@/lib/api";
import { formatCarbonGrams, formatCurrency } from "@/store/app";
import type { SavingsResponse } from "@/lib/types";

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

function Bar2Dual({
  rows,
  yLeft,
  yRight,
  title,
}: {
  rows: { label: string; left: number; right: number }[];
  yLeft: string;
  yRight: string;
  title?: string;
}) {
  const data = rows.map((r) => ({ name: r.label, [yLeft]: r.left, [yRight]: r.right }));
  return (
    <div className="panel">
      <div className="panel-header">
        <div>
          <div className="eyebrow">Breakdown</div>
          <div className="panel-title">{title}</div>
        </div>
      </div>
      <div className="px-6 pb-6 h-64">
        {rows.length === 0 ? (
          <div className="flex h-full items-center justify-center text-sm text-ink-500">
            No breakdown data yet. Approve proposals to populate this chart.
          </div>
        ) : (
          <ResponsiveContainer width="100%" height="100%">
            <BarChart data={data} margin={{ top: 10, right: 10, bottom: 0, left: 0 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="rgba(255,255,255,0.05)" />
              <XAxis dataKey="name" stroke="#8E98A3" fontSize={12} tickLine={false} axisLine={false} />
              <YAxis yAxisId="left" stroke="#34D399" fontSize={12} tickLine={false} axisLine={false} />
              <YAxis yAxisId="right" orientation="right" stroke="#F59E0B" fontSize={12} tickLine={false} axisLine={false} />
              <Tooltip
                contentStyle={{ background: "#0B0F14", border: "1px solid rgba(255,255,255,0.08)", borderRadius: 12 }}
                labelStyle={{ color: "#8E98A3" }}
              />
              <Legend iconType="circle" wrapperStyle={{ fontSize: 12, color: "#8E98A3" }} />
              <Bar yAxisId="left" dataKey={yLeft} fill="#34D399" radius={[6, 6, 0, 0]} />
              <Bar yAxisId="right" dataKey={yRight} fill="#F59E0B" radius={[6, 6, 0, 0]} />
            </BarChart>
          </ResponsiveContainer>
        )}
      </div>
    </div>
  );
}

export default function Savings() {
  const { data } = useApi(api.savings.get);
  const s = data ?? EMPTY_SAVINGS;

  const clusterRows = Object.keys(s.byCluster).map((cluster) => ({
    label: cluster,
    left: Number((s.byCluster[cluster] ?? 0).toFixed(2)),
    right: Math.round((s.byClusterGramsCO2?.[cluster] ?? 0) / 1000),
  }));

  const nsRows = Object.keys(s.byNs).map((ns) => ({
    label: ns,
    left: Number((s.byNs[ns] ?? 0).toFixed(2)),
    right: Math.round((s.byNsGramsCO2?.[ns] ?? 0) / 1000),
  }));

  const proposalCount = s.series.length;

  return (
    <div className="space-y-8 animate-fade-in-up">
      <div>
        <div className="eyebrow">FinOps · Cost + Carbon</div>
        <h1 className="mt-1 font-display text-4xl tracking-tight">Savings</h1>
        <p className="mt-2 max-w-2xl text-ink-100/70">
          Each approved optimization reduces both cluster spend and your grid-adjusted
          CO₂ footprint. The same policies power both signals.
        </p>
        {s.refreshed ? (
          <div className="mt-2 text-xs text-ink-500 font-mono">
            Refreshed {new Date(s.refreshed).toLocaleString()}
          </div>
        ) : null}
      </div>

      <section className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-4">
        <KpiCard
          eyebrow="Total savings (USD)"
          value={formatCurrency(s.totalUSD ?? 0)}
          sub="Across all completed &amp; stabilizing proposals."
          accent="mint"
          icon="💲"
        />
        <KpiCard
          eyebrow="Total CO₂ saved"
          value={s.totalCarbonHuman ?? formatCarbonGrams(s.totalGramsCO2 ?? 0)}
          sub="Workload-month reductions weighted by regional grid carbon intensity."
          accent="amber"
          icon="🌿"
        />
        <KpiCard
          eyebrow="Proposals tracked"
          value={proposalCount.toString()}
          sub="30-day rolling window."
          accent="sky"
          icon="📈"
        />
        <KpiCard
          eyebrow="Avg. CO₂ per USD saved"
          value={
            (s.totalUSD ?? 0) > 0
              ? `${Math.round((s.totalGramsCO2 ?? 0) / s.totalUSD)} g / $`
              : "—"
          }
          sub="Intensity ratio of your current policy mix."
          accent="mint"
          icon="⚖️"
        />
      </section>

      <section className="grid grid-cols-1 xl:grid-cols-2 gap-6">
        <Bar2Dual
          title="By cluster"
          rows={clusterRows}
          yLeft="USD saved /mo"
          yRight="kg CO₂ saved /mo"
        />
        <Bar2Dual
          title="By namespace"
          rows={nsRows}
          yLeft="USD saved /mo"
          yRight="kg CO₂ saved /mo"
        />
      </section>

      <section className="panel">
        <div className="panel-header">
          <div>
            <div className="eyebrow">30-day trend</div>
            <div className="panel-title">USD &amp; CO₂ saved per day</div>
          </div>
        </div>
        <div className="px-6 pb-6">
          <SavingsChart data={s.series} height={340} />
        </div>
      </section>
    </div>
  );
}
