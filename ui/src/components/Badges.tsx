import clsx from "clsx";

function humanize(value: string): string {
  return value
    .replace(/([a-z0-9])([A-Z])/g, "$1 $2")
    .replace(/([A-Z])([A-Z][a-z])/g, "$1 $2")
    .trim();
}

export function RiskBadge({ score }: { score: number }) {
  const tone =
    score >= 70
      ? "border-ember-500/35 bg-ember-500/10 text-ember-400"
      : score >= 40
        ? "border-amber-500/35 bg-amber-500/10 text-amber-400"
        : "border-white/10 bg-white/[0.03] text-ink-300";
  return (
    <span className={clsx("chip border", tone)} title={`Risk score ${score} of 100`}>
      Risk {score}
    </span>
  );
}

export function ConfidenceBadge({ score }: { score: number }) {
  const tone =
    score >= 80
      ? "border-mint-500/35 bg-mint-500/10 text-mint-500"
      : score >= 55
        ? "border-sky-500/35 bg-sky-500/10 text-sky-400"
        : "border-white/10 bg-white/[0.03] text-ink-300";
  return (
    <span className={clsx("chip border", tone)} title={`Confidence score ${score} of 100`}>
      Conf {score}
    </span>
  );
}

const phaseTones: Record<string, string> = {
  Detected: "border-white/10 bg-white/[0.03] text-ink-300",
  Evaluating: "border-sky-500/35 bg-sky-500/10 text-sky-400",
  Approved: "border-mint-500/35 bg-mint-500/10 text-mint-500",
  AwaitingExternalApproval: "border-amber-500/35 bg-amber-500/10 text-amber-400",
  Rejected: "border-white/10 bg-white/[0.02] text-ink-400",
  Expired: "border-white/10 bg-white/[0.02] text-ink-400",
  Scheduled: "border-sky-500/35 bg-sky-500/10 text-sky-400",
  Executing: "border-sky-500/35 bg-sky-500/10 text-sky-400",
  Verifying: "border-amber-500/35 bg-amber-500/10 text-amber-400",
  Stabilizing: "border-amber-500/35 bg-amber-500/10 text-amber-400",
  Completed: "border-mint-500/35 bg-mint-500/10 text-mint-500",
  Failed: "border-ember-500/35 bg-ember-500/10 text-ember-400",
  RollbackRequired: "border-ember-500/35 bg-ember-500/10 text-ember-400",
  RollingBack: "border-ember-500/35 bg-ember-500/10 text-ember-400",
  RollbackVerification: "border-amber-500/35 bg-amber-500/10 text-amber-400",
  RolledBack: "border-white/10 bg-white/[0.03] text-ink-300",
};

export function PhaseChip({ phase }: { phase: string }) {
  const label = phase ? humanize(phase) : "Unknown phase";
  return (
    <span className={clsx("chip border", phaseTones[phase] ?? "border-white/10 bg-white/[0.03] text-ink-300")}>
      {label}
    </span>
  );
}

export function ApprovalChip({ state }: { state: string }) {
  const tone =
    state === "Approved"
      ? "border-mint-500/35 bg-mint-500/10 text-mint-500"
      : state === "Rejected"
        ? "border-ember-500/35 bg-ember-500/10 text-ember-400"
        : state === "Pending"
          ? "border-amber-500/35 bg-amber-500/10 text-amber-400"
          : "border-white/10 bg-white/[0.03] text-ink-300";
  return <span className={clsx("chip border", tone)}>{state || "Not reported"}</span>;
}

export function RecommendationLabel({ value }: { value: string }) {
  return <span className="font-mono text-[12px] text-ink-100">{humanize(value)}</span>;
}
