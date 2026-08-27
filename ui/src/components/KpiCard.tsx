import clsx from "clsx";
import type { ReactNode } from "react";

export interface KpiCardProps {
  eyebrow?: string;
  value: ReactNode;
  sub?: ReactNode;
  delta?: {
    label?: string;
    value: number;
    direction: "up" | "down";
    good?: boolean;
  };
  accent?: "mint" | "amber" | "ember" | "sky";
  className?: string;
  icon?: ReactNode;
}

const accentClass = {
  mint: "from-mint-500/20 via-mint-500/10 to-transparent text-mint-300",
  amber: "from-amber-500/20 via-amber-500/10 to-transparent text-amber-300",
  ember: "from-ember-500/20 via-ember-500/10 to-transparent text-ember-300",
  sky: "from-sky-500/20 via-sky-500/10 to-transparent text-sky-300",
} as const;

const chipClass = {
  mint: "bg-mint-500/15 text-mint-400",
  amber: "bg-amber-500/15 text-amber-400",
  ember: "bg-ember-500/15 text-ember-400",
  sky: "bg-sky-500/15 text-sky-400",
} as const;

export default function KpiCard({
  eyebrow,
  value,
  sub,
  delta,
  accent = "mint",
  className,
  icon,
}: KpiCardProps) {
  return (
    <div className={clsx("panel p-6 relative", className)}>
      <div
        aria-hidden
        className={clsx(
          "pointer-events-none absolute -top-24 -right-16 w-64 h-64 rounded-full blur-3xl opacity-70 bg-gradient-to-br",
          accentClass[accent],
        )}
      />
      <div className="relative">
        <div className="flex items-start justify-between">
          <div className="eyebrow">{eyebrow ?? "KPI"}</div>
          {icon ? <div className={clsx("text-2xl", chipClass[accent], "w-9 h-9 grid place-items-center rounded-lg border border-white/10")}>{icon}</div> : null}
        </div>
        <div className="mt-3 kpi-value">{value}</div>
        <div className="mt-2 flex flex-wrap items-center gap-3 text-sm text-ink-100/70">
          {sub ? <span>{sub}</span> : null}
          {delta ? (
            <span
              className={clsx(
                "chip",
                delta.good
                  ? "bg-mint-500/10 text-mint-400"
                  : "bg-ember-500/10 text-ember-400",
              )}
            >
              <span className="mr-1">
                {delta.direction === "up" ? "▲" : "▼"}
              </span>
              {delta.label ?? "vs. prev"} · {Math.abs(delta.value).toFixed(1)}%
            </span>
          ) : null}
        </div>
      </div>
    </div>
  );
}
