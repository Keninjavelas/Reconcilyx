import { ReactNode } from "react";
import clsx from "clsx";

export interface DiffRow {
  label: string;
  before: ReactNode;
  after: ReactNode;
  unit?: string;
  lowerIsBetter?: boolean;
}

function compare(before: string | number | undefined, after: string | number | undefined, lowerBetter: boolean) {
  if (before === undefined || after === undefined || typeof before !== typeof after) return "flat";
  const b = Number(before);
  const a = Number(after);
  if (Number.isNaN(b) || Number.isNaN(a)) return "flat";
  if (Math.abs(a - b) < 1e-9) return "flat";
  const better = lowerBetter ? a < b : a > b;
  if (better) return "better";
  return "worse";
}

export default function BeforeAfterCard({
  title,
  rows,
  className,
}: {
  title?: string;
  rows: DiffRow[];
  className?: string;
}) {
  return (
    <div className={clsx("panel", className)}>
      {title ? (
        <div className="panel-header">
          <div className="panel-title">{title}</div>
        </div>
      ) : null}
      <div className="divide-y divide-white/5">
        {rows.map((r, i) => {
          const state = compare(
            typeof r.before === "string" ? undefined : (r.before as number),
            typeof r.after === "string" ? undefined : (r.after as number),
            r.lowerIsBetter ?? true,
          );
          return (
            <div key={i} className="grid grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)_auto] items-center gap-4 px-6 py-4">
              <div className="text-sm text-ink-100/70">{r.label}</div>
              <div className="font-mono text-sm text-ink-100/90 text-right">
                {r.before}
                <span className="ml-1 text-ink-500">{r.unit}</span>
              </div>
              <div
                className={clsx(
                  "font-mono text-sm text-right font-medium",
                  state === "better" && "text-mint-400",
                  state === "worse" && "text-ember-400",
                  state === "flat" && "text-ink-100/90",
                )}
              >
                {r.after}
                <span className="ml-1 text-ink-500">{r.unit}</span>
              </div>
              <div
                className={clsx(
                  "chip",
                  state === "better" && "bg-mint-500/10 text-mint-400 border border-mint-500/20",
                  state === "worse" && "bg-ember-500/10 text-ember-400 border border-ember-500/20",
                  state === "flat" && "bg-white/[0.03] text-ink-100/60 border border-white/10",
                )}
              >
                {state === "better" ? "✓ better" : state === "worse" ? "⚠ worse" : "— same"}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
