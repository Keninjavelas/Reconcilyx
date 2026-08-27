import {
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import type { SavingsResponse } from "@/lib/types";
import clsx from "clsx";

export default function SavingsChart({
  data,
  className,
  height = 280,
}: {
  data: SavingsResponse["series"];
  className?: string;
  height?: number;
}) {
  const chart = data.map((pt) => ({
    date: pt.date,
    "Saved (USD)": Number(pt.usd.toFixed(2)),
    "Saved (kg CO₂)": Number(((pt.gramsCO2 ?? 0) / 1000).toFixed(2)),
  }));
  return (
    <div className={clsx("w-full", className)} style={{ height }}>
      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={chart} margin={{ top: 10, right: 16, left: 0, bottom: 0 }}>
          <defs>
            <linearGradient id="usdFill" x1="0" y1="0" x2="0" y2="1">
              <stop offset="5%" stopColor="#34D399" stopOpacity={0.4} />
              <stop offset="95%" stopColor="#34D399" stopOpacity={0} />
            </linearGradient>
          </defs>
          <CartesianGrid stroke="rgba(255,255,255,0.05)" vertical={false} />
          <XAxis
            dataKey="date"
            stroke="#475569"
            tick={{ fill: "#94a3b8", fontSize: 12 }}
            axisLine={false}
            tickLine={false}
          />
          <YAxis
            yAxisId="left"
            stroke="#475569"
            tick={{ fill: "#94a3b8", fontSize: 12 }}
            axisLine={false}
            tickLine={false}
            tickFormatter={(v) => `$${Number(v).toFixed(0)}`}
          />
          <YAxis
            yAxisId="right"
            orientation="right"
            stroke="#475569"
            tick={{ fill: "#94a3b8", fontSize: 12 }}
            axisLine={false}
            tickLine={false}
            tickFormatter={(v) => `${Number(v).toFixed(0)} kg`}
          />
          <Tooltip
            contentStyle={{
              background: "rgba(17, 24, 39, 0.96)",
              border: "1px solid rgba(255,255,255,0.06)",
              borderRadius: 12,
              color: "#F4F6FA",
              fontSize: 13,
            }}
            labelStyle={{ color: "#C9D1DC", fontWeight: 600 }}
          />
          <Legend
            verticalAlign="top"
            iconType="circle"
            wrapperStyle={{ color: "#94a3b8", fontSize: 12, paddingBottom: 12 }}
          />
          <Line
            yAxisId="left"
            type="monotone"
            dataKey="Saved (USD)"
            stroke="#34D399"
            strokeWidth={2.25}
            dot={false}
            activeDot={{ r: 5, stroke: "#0B0F14", strokeWidth: 2 }}
          />
          <Line
            yAxisId="right"
            type="monotone"
            dataKey="Saved (kg CO₂)"
            stroke="#F59E0B"
            strokeWidth={2.25}
            dot={false}
            activeDot={{ r: 5, stroke: "#0B0F14", strokeWidth: 2 }}
          />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}
