import { useState, useEffect } from "react";
import { api } from "@/lib/api";
import type { ProposalDetail, ProposalListItem } from "@/lib/types";
import { ApprovalChip, ConfidenceBadge, PhaseChip, RiskBadge } from "./Badges";
import { formatCurrency, useAppStore } from "@/store/app";

interface ProposalDetailDrawerProps {
  proposalName: string | null;
  onClose: () => void;
}

export default function ProposalDetailDrawer({ proposalName, onClose }: ProposalDetailDrawerProps) {
  const [detail, setDetail] = useState<ProposalDetail | null>(null);
  const [loading, setLoading] = useState(false);
  const [actionLoading, setActionLoading] = useState(false);
  const pushToast = useAppStore((s) => s.pushToast);
  const refresh = useAppStore((s) => s.refresh);

  useEffect(() => {
    if (!proposalName) return;
    setLoading(true);
    api.proposals
      .detail(proposalName)
      .then((res) => setDetail(res))
      .catch((err) =>
        pushToast({ kind: "error", title: "Failed to load proposal", message: String(err) }),
      )
      .finally(() => setLoading(false));
  }, [proposalName, pushToast]);

  if (!proposalName) return null;

  const handleApprove = async () => {
    setActionLoading(true);
    try {
      await api.proposals.approve(proposalName, "Approved via Detail Inspector Drawer");
      pushToast({ kind: "success", title: "Proposal Approved", message: `Proposal ${proposalName} approved successfully.` });
      refresh();
      onClose();
    } catch (err: unknown) {
      pushToast({ kind: "error", title: "Approval Failed", message: err instanceof Error ? err.message : String(err) });
    } finally {
      setActionLoading(false);
    }
  };

  const handleReject = async () => {
    setActionLoading(true);
    try {
      await api.proposals.reject(proposalName, "Rejected via Detail Inspector Drawer");
      pushToast({ kind: "warn", title: "Proposal Rejected", message: `Proposal ${proposalName} rejected.` });
      refresh();
      onClose();
    } catch (err: unknown) {
      pushToast({ kind: "error", title: "Rejection Failed", message: err instanceof Error ? err.message : String(err) });
    } finally {
      setActionLoading(false);
    }
  };

  const handleRollback = async () => {
    setActionLoading(true);
    try {
      await api.proposals.rollback(proposalName, "Rollback requested via Detail Inspector Drawer");
      pushToast({ kind: "warn", title: "Rollback Initiated", message: `Rollback triggered for ${proposalName}.` });
      refresh();
      onClose();
    } catch (err: unknown) {
      pushToast({ kind: "error", title: "Rollback Failed", message: err instanceof Error ? err.message : String(err) });
    } finally {
      setActionLoading(false);
    }
  };

  const summary: Partial<ProposalListItem> = detail?.summary ?? { name: proposalName };

  return (
    <div
      className="fixed inset-0 z-50 overflow-hidden bg-black/60 backdrop-blur-sm flex justify-end animate-fade-in"
      onClick={onClose}
    >
      <div
        className="w-full max-w-2xl bg-ink-950 border-l border-white/10 flex flex-col h-full shadow-2xl animate-slide-left overflow-y-auto"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="px-6 py-5 border-b border-white/10 flex items-center justify-between bg-ink-900/50 sticky top-0 backdrop-blur z-10">
          <div>
            <div className="eyebrow">Proposal Inspector</div>
            <h2 className="font-display text-xl text-white font-medium mt-0.5">
              {summary.namespace}/{summary.workloadName}
            </h2>
            <div className="text-xs text-ink-500 font-mono mt-0.5">{summary.name}</div>
          </div>
          <button
            onClick={onClose}
            className="text-ink-400 hover:text-white p-2 rounded-lg hover:bg-white/5"
          >
            ✕
          </button>
        </div>

        {loading ? (
          <div className="p-12 text-center text-sm text-mint-400 animate-pulse">
            Loading proposal inspection details...
          </div>
        ) : (
          <div className="p-6 space-y-6 text-sm">
            {/* Status & Safety Badges */}
            <div className="panel p-4 flex flex-wrap items-center justify-between gap-3 bg-white/[0.02]">
              <div className="flex items-center gap-2">
                <PhaseChip phase={summary.phase ?? "Detected"} />
                <ApprovalChip state={summary.approvalState ?? "Pending"} />
              </div>
              <div className="flex items-center gap-2">
                <ConfidenceBadge score={summary.confidenceScore ?? 80} />
                <RiskBadge score={summary.riskScore ?? 15} />
              </div>
            </div>

            {/* In-Place Pod Resizing Resource Diff */}
            <div className="panel p-5 space-y-4">
              <div className="eyebrow">In-Place Pod Resizing Diff</div>
              <div className="grid grid-cols-2 gap-4 text-xs">
                <div className="p-3 rounded-xl bg-ember-500/10 border border-ember-500/20 space-y-2">
                  <div className="font-semibold text-ember-400 text-xs">BEFORE (Observed Baseline)</div>
                  <div>CPU Request: <span className="font-mono text-white font-bold">{summary.cpuBefore ?? "—"}</span></div>
                  <div>Memory Request: <span className="font-mono text-white font-bold">{summary.memoryBefore ?? "—"}</span></div>
                </div>

                <div className="p-3 rounded-xl bg-mint-500/10 border border-mint-500/20 space-y-2">
                  <div className="font-semibold text-mint-400 text-xs">PROPOSED (After In-Place SSA)</div>
                  <div>CPU Request: <span className="font-mono text-mint-300 font-bold">{summary.cpuAfter ?? "—"}</span></div>
                  <div>Memory Request: <span className="font-mono text-mint-300 font-bold">{summary.memoryAfter ?? "—"}</span></div>
                </div>
              </div>
            </div>

            {/* Impact Calculation */}
            <div className="grid grid-cols-2 gap-4">
              <div className="panel p-4 border-mint-500/20">
                <div className="eyebrow text-mint-400">Projected Monthly Savings</div>
                <div className="text-2xl font-mono font-bold text-mint-400 mt-1">
                  {summary.estimatedSavings ? formatCurrency(Number(summary.estimatedSavings)) : "—"} / mo
                </div>
              </div>

              <div className="panel p-4 border-amber-500/20">
                <div className="eyebrow text-amber-400">Carbon Abatement</div>
                <div className="text-2xl font-mono font-bold text-amber-400 mt-1">
                  {summary.carbonSavedHuman ?? "—"} / mo
                </div>
              </div>
            </div>

            {/* Rationale & Rules */}
            <div className="panel p-5 space-y-2">
              <div className="eyebrow">Analyzer Rationale</div>
              <p className="text-ink-200 text-xs leading-relaxed">
                {summary.rationale ?? detail?.rationale ?? "Target workload analyzed via Prometheus 7-day usage trends and rule evaluation engine."}
              </p>
            </div>

            {/* Execution Events */}
            {detail?.execution?.events && detail.execution.events.length > 0 && (
              <div className="panel p-5 space-y-3">
                <div className="eyebrow">Actuation Events Timeline</div>
                <div className="space-y-2">
                  {detail.execution.events.map((evt, idx) => (
                    <div key={idx} className="flex items-center justify-between text-xs py-1 border-b border-white/5">
                      <span className="text-ink-300">{evt.message}</span>
                      <span className="text-ink-500 font-mono text-[10px]">{evt.at}</span>
                    </div>
                  ))}
                </div>
              </div>
            )}

            {/* Actions Bar */}
            <div className="pt-4 border-t border-white/10 flex flex-wrap items-center gap-3">
              {summary.approvalState === "Pending" && (
                <>
                  <button
                    onClick={handleApprove}
                    disabled={actionLoading}
                    className="btn-primary text-xs"
                  >
                    🚀 Approve &amp; Apply SSA In-Place
                  </button>
                  <button
                    onClick={handleReject}
                    disabled={actionLoading}
                    className="btn-danger text-xs"
                  >
                    🚫 Reject Proposal
                  </button>
                </>
              )}

              {(summary.phase === "Completed" || summary.phase === "Stabilizing") && (
                <button
                  onClick={handleRollback}
                  disabled={actionLoading}
                  className="btn-ghost text-amber-400 border-amber-500/30 hover:bg-amber-500/10 text-xs"
                >
                  ↺ Rollback to Pre-Actuation Baseline
                </button>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
