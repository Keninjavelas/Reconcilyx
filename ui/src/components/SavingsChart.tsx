import {
  Bar,
  CartesianGrid,
  ComposedChart,
  Line,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import type { SavingsResponse } from "@/lib/types";
import { buildDailyLedger, formatLedgerDay } from "@/lib/ledger";
import clsx from "clsx";

export default function SavingsChart({
  data,
  className,
  height = 300,
  compact = false,
  windowDays,
}: {
  data: SavingsResponse["series"];
  className?: string;
  height?: number;
  compact?: boolean;
  windowDays?: number;
}) {
  const days = buildDailyLedger(data, { windowDays });
  const hasValues = days.some((day) => day.usd > 0 || day.gramsCO2 > 0);

  if (!days.length || !hasValues) {
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
          {windowDays
            ? `No qualifying proposal was recorded in the last ${windowDays} days.`
            : "Completed proposals will populate this series on the next control-plane refresh."}
        </span>
      </div>
    );
  }

  const spansYears = days[0].date.slice(0, 4) !== days[days.length - 1].date.slice(0, 4);
  const chart = days.map((day) => ({
    date: formatLedgerDay(day.date, spansYears),
    usd: Number(day.usd.toFixed(2)),
    kg: Number((day.gramsCO2 / 1000).toFixed(2)),
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
            interval="preserveStartEnd"
          />
          <YAxis
            yAxisId="usd"
            domain={[0, "auto"]}
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
              domain={[0, "auto"]}
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
                ? [`$${Number(value).toFixed(2)}`, "Saved / mo"]
                : [`${Number(value).toFixed(2)} kg`, "CO₂ / mo"]
            }
          />
          <Bar
            yAxisId="usd"
            dataKey="usd"
            fill="#C9EF4A"
            fillOpacity={0.72}
            radius={[2, 2, 0, 0]}
            maxBarSize={compact ? 10 : 18}
            isAnimationActive={false}
          />
          {!compact ? (
            <Line
              yAxisId="co2"
              type="linear"
              dataKey="kg"
              stroke="#FF5C35"
              strokeWidth={1.4}
              strokeDasharray="3 4"
              dot={chart.length <= 7 ? { r: 2.5, fill: "#FF5C35", strokeWidth: 0 } : false}
              isAnimationActive={false}
              activeDot={{ r: 3, fill: "#FF5C35", stroke: "#070806", strokeWidth: 2 }}
            />
          ) : null}
        </ComposedChart>
      </ResponsiveContainer>
    </div>
  );
}
