import type { ReactNode } from "react";
import clsx from "clsx";

export interface DiffRow {
  label: string;
  before: ReactNode;
  after: ReactNode;
  note?: ReactNode;
  tone?: "better" | "worse" | "flat" | "neutral";
}

export default function BeforeAfterCard({
  title,
  eyebrow,
  rows,
  className,
}: {
  title?: string;
  eyebrow?: string;
  rows: DiffRow[];
  className?: string;
}) {
  return (
    <div className={clsx("panel", className)}>
      <div className="panel-header">
        <div>
          {eyebrow ? <div className="eyebrow">{eyebrow}</div> : null}
          {title ? <div className="panel-title mt-1">{title}</div> : null}
        </div>
        <div className="flex items-center gap-4 font-mono text-[10px] uppercase tracking-[0.16em] text-ink-500">
          <span>Observed</span>
          <span>Proposed</span>
        </div>
      </div>
      <div>
        {rows.map((row, index) => (
          <div
            key={`${row.label}-${index}`}
            className="grid grid-cols-[minmax(0,1.2fr)_minmax(0,1fr)_28px_minmax(0,1fr)] items-center gap-3 border-b border-white/[0.055] px-5 py-4 last:border-0 sm:px-6"
          >
            <div>
              <div className="text-[13px] text-ink-200">{row.label}</div>
              {row.note ? <div className="mt-0.5 text-[11px] text-ink-500">{row.note}</div> : null}
            </div>
            <div className="num text-right text-[13px] text-ink-400">{row.before}</div>
            <div className="text-center text-ink-600" aria-hidden>
              →
            </div>
            <div
              className={clsx(
                "num text-right text-[13px] font-medium",
                row.tone === "better" && "text-mint-500",
                row.tone === "worse" && "text-ember-400",
                (!row.tone || row.tone === "flat" || row.tone === "neutral") && "text-ink-50",
              )}
            >
              {row.after}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
