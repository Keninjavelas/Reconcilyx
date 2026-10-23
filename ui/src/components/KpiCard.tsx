import clsx from "clsx";
import type { ReactNode } from "react";

export interface KpiCardProps {
  label: string;
  value: ReactNode;
  detail?: ReactNode;
  slot?: ReactNode;
  className?: string;
  loading?: boolean;
}

export default function KpiCard({
  label,
  value,
  detail,
  slot,
  className,
  loading = false,
}: KpiCardProps) {
  return (
    <div className={clsx("relative flex min-h-[172px] flex-col justify-between p-6 lg:p-7", className)}>
      <div className="flex items-start justify-between gap-4">
        <span className="eyebrow">{label}</span>
        {slot}
      </div>
      {loading ? (
        <div className="mt-6 space-y-3">
          <div className="skeleton h-10 w-2/3 rounded-md" />
          <div className="skeleton h-3 w-full rounded-md" />
        </div>
      ) : (
        <div className="mt-6">
          <div className="kpi-value">{value}</div>
          {detail ? <div className="mt-3 max-w-[28ch] text-[12px] leading-relaxed text-ink-400">{detail}</div> : null}
        </div>
      )}
    </div>
  );
}
