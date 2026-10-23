import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import type { ProposalDetail, ProposalRef } from "@/lib/types";
import { ApprovalChip, ConfidenceBadge, PhaseChip, RiskBadge } from "./Badges";
import { formatCurrency, useAppStore } from "@/store/app";
import BeforeAfterCard from "./BeforeAfterCard";
import { useApi } from "@/lib/useApi";

const ROLLBACK_PHASES = new Set([
  "Verifying",
  "Stabilizing",
  "Completed",
  "Failed",
  "RollbackRequired",
  "RollbackVerification",
]);

interface ProposalDetailDrawerProps {
  proposal: ProposalRef | null;
  onClose: () => void;
}

export default function ProposalDetailDrawer({ proposal, onClose }: ProposalDetailDrawerProps) {
  if (!proposal) return null;
  return <DrawerBody proposal={proposal} onClose={onClose} />;
}

function DrawerBody({ proposal, onClose }: { proposal: ProposalRef; onClose: () => void }) {
  const pushToast = useAppStore((s) => s.pushToast);
  const refresh = useAppStore((s) => s.refresh);
  const { data, loading, error, refetch } = useApi(
    (ref: ProposalRef) => api.proposals.detail(ref.name, ref.namespace),
    proposal,
  );
  const [acting, setActing] = useState(false);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const detail: ProposalDetail | undefined = data;
  const summary = detail?.summary;
  const phase = summary?.phase ?? "";
  const approval = summary?.approvalState ?? "";
  const canReview = approval === "Pending";
  const canRollback = ROLLBACK_PHASES.has(phase);
  const hasSnapshot = Boolean(detail?.raw.status?.executionSnapshot);

  async function approve() {
    setActing(true);
    try {
      await api.proposals.approve(proposal, "Approved via proposal inspector");
      pushToast({ kind: "success", title: "Proposal approved", message: proposal.name });
      refresh();
      refetch();
    } catch (err) {
      pushToast({
        kind: "error",
        title: "Approval failed",
        message: err instanceof Error ? err.message : String(err),
      });
    } finally {
      setActing(false);
    }
  }

  async function reject() {
    setActing(true);
    try {
      await api.proposals.reject(proposal, "Rejected via proposal inspector");
      pushToast({ kind: "warn", title: "Proposal rejected", message: proposal.name });
      refresh();
      refetch();
    } catch (err) {
      pushToast({
        kind: "error",
        title: "Rejection failed",
        message: err instanceof Error ? err.message : String(err),
      });
    } finally {
      setActing(false);
    }
  }

  async function rollback() {
    setActing(true);
    try {
      await api.proposals.rollback(proposal, "Rollback requested via proposal inspector");
      pushToast({
        kind: "warn",
        title: "Rollback requested",
        message: "The controller will restore the pre-actuation snapshot.",
      });
      refresh();
      refetch();
    } catch (err) {
      pushToast({
        kind: "error",
        title: "Rollback failed",
        message: err instanceof Error ? err.message : String(err),
      });
    } finally {
      setActing(false);
    }
  }

  return (
    <div className="overlay fixed inset-0 z-50 flex justify-end" onClick={onClose} role="presentation">
      <aside
        className="flex h-full w-full max-w-2xl animate-slide-left flex-col border-l border-white/[0.12] bg-ink-950"
        onClick={(event) => event.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-label={`Proposal ${proposal.name}`}
      >
        <header className="sticky top-0 z-10 flex items-start justify-between gap-4 border-b border-white/[0.1] bg-ink-950/95 px-5 py-5 backdrop-blur sm:px-7">
          <div className="min-w-0">
            <div className="eyebrow">Proposal inspector</div>
            <h2 className="mt-1 truncate font-display text-[22px] tracking-[-0.03em] text-ink-50">
              {summary ? `${summary.namespace}/${summary.workloadName}` : proposal.name}
            </h2>
            <div className="mt-1 truncate font-mono text-[10px] uppercase tracking-[0.12em] text-ink-500">
              {summary?.workloadKind ?? "Workload"} · {proposal.name}
            </div>
          </div>
          <button className="btn-icon" onClick={onClose} aria-label="Close proposal inspector">
            <span aria-hidden>×</span>
          </button>
        </header>

        <div className="flex-1 overflow-y-auto scrollbar-thin">
          {loading && !detail ? (
            <div className="space-y-4 p-5 sm:p-7">
              <div className="skeleton h-9 w-2/3 rounded-lg" />
              <div className="skeleton h-40 w-full rounded-xl" />
              <div className="skeleton h-24 w-full rounded-xl" />
            </div>
          ) : error ? (
            <div className="p-5 sm:p-7">
              <div className="rounded-xl border border-ember-500/30 bg-ember-500/[0.06] p-4">
                <div className="text-[13px] font-medium text-ember-400">Could not load proposal</div>
                <div className="mt-1 break-words text-[12px] leading-relaxed text-ink-400">{error}</div>
                <button className="btn-ghost mt-4" onClick={refetch}>
                  Retry
                </button>
              </div>
            </div>
          ) : detail && summary ? (
            <div className="space-y-5 p-5 sm:p-7">
              <div className="flex flex-wrap items-center gap-2">
                <PhaseChip phase={phase} />
                <ApprovalChip state={approval} />
                <ConfidenceBadge score={summary.confidenceScore} />
                <RiskBadge score={summary.riskScore} />
              </div>

              <BeforeAfterCard
                eyebrow="In-place pod resize"
                title="Resource request delta"
                rows={[
                  {
                    label: "CPU request",
                    before: summary.cpuBefore ?? "—",
                    after: summary.cpuAfter ?? "—",
                    tone: "neutral",
                  },
                  {
                    label: "Memory request",
                    before: summary.memoryBefore ?? "—",
                    after: summary.memoryAfter ?? "—",
                    tone: "neutral",
                  },
                  {
                    label: "P95 CPU usage",
                    before: detail.observed.p95Cpu ?? "not reported",
                    after: "—",
                    note: "Seven-day observed window, supplied by the analyzer",
                    tone: "neutral",
                  },
                  {
                    label: "P95 memory usage",
                    before: detail.observed.p95Memory ?? "not reported",
                    after: "—",
                    tone: "neutral",
                  },
                ]}
              />

              <div className="grid gap-3 sm:grid-cols-2">
                <div className="panel p-4">
                  <div className="eyebrow">Monthly savings</div>
                  <div className="num mt-2 text-[24px] tracking-[-0.04em] text-mint-500">
                    {summary.estimatedSavings ? formatCurrency(Number(summary.estimatedSavings)) : "—"}
                  </div>
                </div>
                <div className="panel p-4">
                  <div className="eyebrow">Carbon abatement</div>
                  <div className="num mt-2 text-[24px] tracking-[-0.04em] text-amber-400">
                    {summary.carbonSavedHuman ?? "—"}
                  </div>
                </div>
              </div>

              <section className="panel p-5">
                <div className="eyebrow">Analyzer rationale</div>
                <p className="mt-2 text-[13px] leading-relaxed text-ink-300">
                  {detail.rationale || "The analyzer did not record a rationale for this proposal."}
                </p>
                {summary.rationale && summary.rationale !== detail.rationale ? (
                  <p className="mt-2 text-[12px] leading-relaxed text-ink-500">{summary.rationale}</p>
                ) : null}
              </section>

              <section className="panel">
                <div className="panel-header">
                  <div className="panel-title text-[15px]">Execution record</div>
                </div>
                <dl className="divide-y divide-white/[0.055] text-[12px]">
                  <Row label="Policy applied" value={detail.raw.status?.policyApplied ?? "Not recorded"} />
                  <Row label="Execution state" value={detail.raw.status?.executionState ?? "Not reported"} />
                  <Row label="Field manager" value={detail.raw.status?.executionMetadata?.fieldManager ?? "Not recorded"} />
                  <Row label="Execution ID" value={detail.raw.status?.executionMetadata?.executionID ?? "Not recorded"} />
                  <Row label="Yield count" value={String(detail.raw.status?.yieldCount ?? 0)} />
                  <Row label="Snapshot" value={hasSnapshot ? "Captured before actuation" : "No snapshot recorded"} />
                  <Row label="GitOps PR" value={detail.raw.status?.externalPRID ?? "None"} />
                </dl>
              </section>

              <section className="panel">
                <div className="panel-header">
                  <div className="panel-title text-[15px]">Event timeline</div>
                  <span className="font-mono text-[10px] uppercase tracking-[0.14em] text-ink-500">
                    {detail.events.length} events
                  </span>
                </div>
                {detail.events.length === 0 ? (
                  <div className="px-5 py-8 text-center text-[12px] text-ink-400">
                    No Kubernetes events recorded for this proposal yet.
                  </div>
                ) : (
                  <ol className="px-5 py-2 sm:px-6">
                    {detail.events.map((event, index) => (
                      <li
                        key={`${event.at}-${index}`}
                        className="flex gap-3 border-b border-white/[0.055] py-3 last:border-0"
                      >
                        <span
                          className={`mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full ${
                            event.type === "Warning" ? "bg-ember-500" : "bg-mint-500"
                          }`}
                          aria-hidden
                        />
                        <div className="min-w-0">
                          <div className="flex flex-wrap items-baseline gap-2">
                            <span className="text-[12px] text-ink-200">{event.reason}</span>
                            <span className="font-mono text-[10px] text-ink-500">
                              {new Date(event.at).toLocaleString()}
                            </span>
                          </div>
                          <p className="mt-1 break-words text-[12px] leading-relaxed text-ink-400">
                            {event.message}
                          </p>
                        </div>
                      </li>
                    ))}
                  </ol>
                )}
              </section>
            </div>
          ) : null}
        </div>

        <footer className="flex flex-wrap items-center gap-2 border-t border-white/[0.1] bg-ink-950 px-5 py-4 sm:px-7">
          {canReview ? (
            <>
              <button className="btn-primary" onClick={approve} disabled={acting}>
                Approve proposal
              </button>
              <button className="btn-danger" onClick={reject} disabled={acting}>
                Reject
              </button>
            </>
          ) : null}
          {canRollback ? (
            <button
              className="btn-ghost"
              onClick={rollback}
              disabled={acting || !hasSnapshot}
              title={hasSnapshot ? "Restore the pre-actuation snapshot" : "No execution snapshot is available"}
            >
              Roll back
            </button>
          ) : null}
          {!canReview && !canRollback ? (
            <span className="text-[12px] text-ink-500">
              No actions available in the current phase.
            </span>
          ) : null}
          <button className="btn-ghost ml-auto" onClick={onClose} disabled={acting}>
            Close
          </button>
        </footer>
      </aside>
    </div>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-start justify-between gap-5 px-5 py-3 sm:px-6">
      <dt className="text-ink-500">{label}</dt>
      <dd className="max-w-[60%] break-all text-right font-mono text-[11px] text-ink-300">{value}</dd>
    </div>
  );
}
