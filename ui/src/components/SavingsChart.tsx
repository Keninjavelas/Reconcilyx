import {
  Area,
  CartesianGrid,
  ComposedChart,
  Line,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import type { SavingsResponse } from "@/lib/types";
import clsx from "clsx";

function shortDate(value: string): string {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return parsed.toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

export default function SavingsChart({
  data,
  className,
  height = 300,
  compact = false,
}: {
  data: SavingsResponse["series"];
  className?: string;
  height?: number;
  compact?: boolean;
}) {
  if (!data.length) {
    return (
      <div
        className={clsx(
          "flex flex-col items-center justify-center gap-2 rounded-xl border border-dashed border-white/10 text-center",
          className,
        )}
        style={{ height }}
      >
        <span className="font-mono text-[10px] uppercase tracking-[0.18em] text-ink-500">
          No savings recorded
        </span>
        <span className="max-w-[36ch] text-[12px] leading-relaxed text-ink-400">
          Completed proposals will populate this series on the next control-plane refresh.
        </span>
      </div>
    );
  }

  const chart = data.map((pt) => ({
    date: shortDate(pt.date),
    usd: Number(pt.usd.toFixed(2)),
    kg: Number(((pt.gramsCO2 ?? 0) / 1000).toFixed(2)),
  }));

  return (
    <div className={clsx("w-full", className)} style={{ height }}>
      <ResponsiveContainer width="100%" height="100%">
        <ComposedChart data={chart} margin={{ top: 12, right: 4, left: compact ? -18 : 0, bottom: 0 }}>
          <CartesianGrid stroke="rgba(244,241,234,0.07)" vertical={false} />
          <XAxis
            dataKey="date"
            stroke="transparent"
            tick={{ fill: "#6D7368", fontSize: 10, fontFamily: "IBM Plex Mono" }}
            axisLine={false}
            tickLine={false}
            minTickGap={36}
          />
          <YAxis
            yAxisId="usd"
            stroke="transparent"
            tick={{ fill: "#6D7368", fontSize: 10, fontFamily: "IBM Plex Mono" }}
            axisLine={false}
            tickLine={false}
            tickFormatter={(value) => `$${Number(value).toFixed(0)}`}
            width={compact ? 44 : 56}
          />
          {!compact ? (
            <YAxis
              yAxisId="co2"
              orientation="right"
              stroke="transparent"
              tick={{ fill: "#6D7368", fontSize: 10, fontFamily: "IBM Plex Mono" }}
              axisLine={false}
              tickLine={false}
              tickFormatter={(value) => `${Number(value).toFixed(0)}kg`}
              width={48}
            />
          ) : null}
          <Tooltip
            cursor={{ stroke: "rgba(244,241,234,0.18)", strokeWidth: 1 }}
            contentStyle={{
              background: "#0D0F0C",
              border: "1px solid rgba(244,241,234,0.14)",
              borderRadius: 10,
              color: "#F4F1EA",
              fontSize: 12,
              boxShadow: "0 24px 48px -24px rgba(0,0,0,0.9)",
            }}
            labelStyle={{
              color: "#8E9488",
              fontFamily: "IBM Plex Mono",
              fontSize: 10,
              letterSpacing: "0.08em",
              textTransform: "uppercase",
              marginBottom: 6,
            }}
            formatter={(value, name) =>
              name === "usd"
                ? [`$${Number(value).toFixed(2)}`, "Saved"]
                : [`${Number(value).toFixed(2)} kg`, "CO₂"]
            }
          />
          <Area
            yAxisId="usd"
            type="monotone"
            dataKey="usd"
            stroke="#C9EF4A"
            strokeWidth={1.6}
            fill="rgba(201,239,74,0.10)"
            isAnimationActive={false}
            activeDot={{ r: 3, fill: "#C9EF4A", stroke: "#070806", strokeWidth: 2 }}
          />
          {!compact ? (
            <Line
              yAxisId="co2"
              type="monotone"
              dataKey="kg"
              stroke="#FF5C35"
              strokeWidth={1.4}
              strokeDasharray="3 4"
              dot={false}
              isAnimationActive={false}
              activeDot={{ r: 3, fill: "#FF5C35", stroke: "#070806", strokeWidth: 2 }}
            />
          ) : null}
        </ComposedChart>
      </ResponsiveContainer>
    </div>
  );
}
