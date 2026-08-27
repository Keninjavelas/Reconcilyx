import clsx from "clsx";

export function RiskBadge({ score, label }: { score: number; label?: string }) {
  const level =
    score >= 70 ? "high" : score >= 40 ? "med" : score >= 15 ? "low" : "none";
  const style = {
    high: "bg-ember-500/15 text-ember-400 border-ember-500/30",
    med:  "bg-amber-500/15 text-amber-400 border-amber-500/30",
    low:  "bg-sky-500/15 text-sky-400 border-sky-500/30",
    none: "bg-white/[0.03] text-ink-100/70 border-white/10",
  }[level];
  return (
    <span className={clsx("chip border", style)}>
      {label ?? "Risk"} · {score}
    </span>
  );
}

export function ConfidenceBadge({ score }: { score: number }) {
  const level =
    score >= 85 ? "high" : score >= 60 ? "med" : score >= 35 ? "low" : "low";
  const style = {
    high: "bg-mint-500/15 text-mint-400 border-mint-500/30",
    med:  "bg-sky-500/15 text-sky-400 border-sky-500/30",
    low:  "bg-white/[0.03] text-ink-100/70 border-white/10",
  }[level];
  return (
    <span className={clsx("chip border", style)}>
      Conf · {score}
    </span>
  );
}

export function PhaseChip({ phase }: { phase: string }) {
  const map: Record<string, string> = {
    Detected: "bg-white/[0.03] text-ink-100/70 border-white/10",
    Evaluating: "bg-sky-500/10 text-sky-400 border-sky-500/30",
    Approved: "bg-mint-500/10 text-mint-400 border-mint-500/30",
    AwaitingExternalApproval: "bg-amber-500/10 text-amber-400 border-amber-500/30",
    AwaitingHumanApproval: "bg-amber-500/10 text-amber-400 border-amber-500/30",
    Scheduled: "bg-sky-500/10 text-sky-400 border-sky-500/30",
    Executing: "bg-sky-500/10 text-sky-400 border-sky-500/30",
    Verifying: "bg-amber-500/10 text-amber-400 border-amber-500/30",
    Stabilizing: "bg-amber-500/10 text-amber-400 border-amber-500/30",
    Completed: "bg-mint-500/10 text-mint-400 border-mint-500/30",
    Rollback: "bg-ember-500/10 text-ember-400 border-ember-500/30",
    Rejected: "bg-white/[0.03] text-ink-100/50 border-white/10",
  };
  return (
    <span className={clsx("chip border", map[phase] ?? "bg-white/[0.03] text-ink-100/70 border-white/10")}>
      {phase}
    </span>
  );
}

export function ApprovalChip({ state }: { state: string }) {
  const style =
    state === "Approved" || state === "AutoApproved"
      ? "bg-mint-500/10 text-mint-400 border-mint-500/30"
      : state === "Rejected"
        ? "bg-ember-500/10 text-ember-400 border-ember-500/30"
        : "bg-amber-500/10 text-amber-400 border-amber-500/30";
  return <span className={clsx("chip border", style)}>{state}</span>;
}
