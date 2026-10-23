import type { ReactNode } from "react";

export default function PageHeader({
  index,
  eyebrow,
  title,
  description,
  actions,
  meta,
}: {
  index: string;
  eyebrow: string;
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  meta?: ReactNode;
}) {
  return (
    <header className="border-b border-white/10 pb-8 lg:pb-10">
      <div className="flex flex-wrap items-start justify-between gap-6">
        <div className="max-w-3xl">
          <div className="flex items-center gap-3">
            <span className="font-mono text-[10px] tracking-[0.2em] text-mint-400">{index}</span>
            <span className="h-px w-8 bg-white/20" aria-hidden />
            <span className="eyebrow">{eyebrow}</span>
          </div>
          <h1 className="mt-4 font-display text-[clamp(2.35rem,4.6vw,4.15rem)] font-medium leading-[0.95] tracking-[-0.055em] text-ink-50">
            {title}
          </h1>
          {description ? (
            <p className="mt-4 max-w-2xl text-[15px] leading-relaxed text-ink-300">{description}</p>
          ) : null}
        </div>
        {actions ? <div className="flex flex-wrap items-center gap-2">{actions}</div> : null}
      </div>
      {meta ? (
        <div className="mt-6 flex flex-wrap items-center gap-x-5 gap-y-2 font-mono text-[10px] uppercase tracking-[0.14em] text-ink-500">
          {meta}
        </div>
      ) : null}
    </header>
  );
}
