import { useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import DataTable from "@/components/DataTable";
import { ApprovalChip, ConfidenceBadge, PhaseChip, RiskBadge } from "@/components/Badges";
import ProposalDetailDrawer from "@/components/ProposalDetailDrawer";
import { api } from "@/lib/api";
import { useApi } from "@/lib/useApi";
import { formatCurrency, useAppStore } from "@/store/app";
import type { ProposalListItem } from "@/lib/types";
import clsx from "clsx";

const FILTERS: { key: "all" | ProposalListItem["approvalState"]; label: string }[] = [
  { key: "all", label: "All" },
  { key: "Pending", label: "Pending approval" },
  { key: "Approved", label: "Approved" },
  { key: "Rejected", label: "Rejected" },
];

export default function Proposals() {
  const [params, setParams] = useSearchParams();
  const pushToast = useAppStore((s) => s.pushToast);
  const refresh = useAppStore((s) => s.refresh);
  const selectedProposals = useAppStore((s) => s.selectedProposals);
  const toggleProposalSelection = useAppStore((s) => s.toggleProposalSelection);
  const selectAllProposals = useAppStore((s) => s.selectAllProposals);
  const clearSelectedProposals = useAppStore((s) => s.clearSelectedProposals);

  const initialFilter = (params.get("approvalState") ?? "all") as (typeof FILTERS)[number]["key"];
  const [filter, setFilter] = useState<(typeof FILTERS)[number]["key"]>(initialFilter);
  const [q, setQ] = useState(params.get("q") ?? "");
  const [drawerProposal, setDrawerProposal] = useState<string | null>(null);
  const [bulkLoading, setBulkLoading] = useState(false);

  const { data } = useApi(api.proposals.list, { sortBy: "-createdAt" });
  const rows = data ?? [];

  const filtered = useMemo(() => {
    return rows.filter((r) => {
      if (filter !== "all" && r.approvalState !== filter) return false;
      if (!q) return true;
      const query = q.toLowerCase();
      return (
        r.workloadName.toLowerCase().includes(query) ||
        r.namespace.toLowerCase().includes(query) ||
        r.name.toLowerCase().includes(query)
      );
    });
  }, [rows, filter, q]);

  const allFilteredSelected =
    filtered.length > 0 && filtered.every((r) => selectedProposals.includes(r.name));

  function toggleSelectAll() {
    if (allFilteredSelected) {
      clearSelectedProposals();
    } else {
      selectAllProposals(filtered.map((r) => r.name));
    }
  }

  function setFilterLink(f: (typeof FILTERS)[number]["key"]) {
    setFilter(f);
    const next = new URLSearchParams(params);
    if (f === "all") next.delete("approvalState");
    else next.set("approvalState", f);
    setParams(next);
  }

  async function handleBulkApprove() {
    if (selectedProposals.length === 0) return;
    setBulkLoading(true);
    try {
      await api.proposals.bulkApprove(selectedProposals);
      pushToast({
        kind: "success",
        title: "Bulk Approved",
        message: `Successfully approved ${selectedProposals.length} proposal(s).`,
      });
      clearSelectedProposals();
      refresh();
    } catch (err: unknown) {
      pushToast({
        kind: "error",
        title: "Bulk Approval Failed",
        message: err instanceof Error ? err.message : String(err),
      });
    } finally {
      setBulkLoading(false);
    }
  }

  async function handleBulkReject() {
    if (selectedProposals.length === 0) return;
    setBulkLoading(true);
    try {
      await api.proposals.bulkReject(selectedProposals);
      pushToast({
        kind: "warn",
        title: "Bulk Rejected",
        message: `Rejected ${selectedProposals.length} proposal(s).`,
      });
      clearSelectedProposals();
      refresh();
    } catch (err: unknown) {
      pushToast({
        kind: "error",
        title: "Bulk Rejection Failed",
        message: err instanceof Error ? err.message : String(err),
      });
    } finally {
      setBulkLoading(false);
    }
  }

  async function approveRow(r: ProposalListItem) {
    try {
      await api.proposals.approve(r.name);
      pushToast({ kind: "success", title: "Approved proposal", message: r.name });
      refresh();
    } catch (e) {
      pushToast({
        kind: "error",
        title: "Couldn't approve proposal",
        message: e instanceof Error ? e.message : String(e),
      });
    }
  }

  async function rejectRow(r: ProposalListItem) {
    try {
      await api.proposals.reject(r.name);
      pushToast({ kind: "warn", title: "Rejected proposal", message: r.name });
      refresh();
    } catch (e) {
      pushToast({
        kind: "error",
        title: "Couldn't reject proposal",
        message: e instanceof Error ? e.message : String(e),
      });
    }
  }

  return (
    <div className="space-y-6 animate-fade-in-up">
      {/* Header */}
      <div className="flex flex-wrap items-end justify-between gap-4 border-b border-white/5 pb-4">
        <div>
          <div className="eyebrow">Governance &amp; Actuation</div>
          <h1 className="font-display text-4xl tracking-tight">Proposals Inventory</h1>
          <p className="mt-1 text-ink-100/70 max-w-2xl text-sm">
            Every optimization is an immutable proposal: workload deltas, risk/confidence, cost &amp; CO₂
            impact, approval gates, and zero-downtime Server-Side Apply actuation history.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <input
            className="input !w-64"
            placeholder="Search workload, namespace, id…"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
        </div>
      </div>

      {/* Filter Tabs & Sticky Bulk Action Bar */}
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div className="flex flex-wrap gap-2">
          {FILTERS.map((f) => {
            const count =
              f.key === "all"
                ? rows.length
                : rows.filter((r) => r.approvalState === f.key).length;
            return (
              <button
                key={f.key}
                onClick={() => setFilterLink(f.key)}
                className={clsx(
                  "rounded-xl border px-4 py-2 text-sm font-medium transition-all",
                  filter === f.key
                    ? "bg-white/[0.04] border-mint-500/30 text-mint-400"
                    : "border-white/5 text-ink-100/70 hover:bg-white/[0.03]",
                )}
              >
                {f.label}
                <span className="ml-2 text-xs text-ink-500">{count}</span>
              </button>
            );
          })}
        </div>

        {/* Bulk Action Bar */}
        {selectedProposals.length > 0 && (
          <div className="flex items-center gap-2 bg-mint-500/10 border border-mint-500/30 px-4 py-2 rounded-xl text-xs animate-fade-in">
            <span className="font-mono text-mint-300 font-bold">
              {selectedProposals.length} selected
            </span>
            <button
              onClick={handleBulkApprove}
              disabled={bulkLoading}
              className="btn-primary !py-1 !px-3 text-xs"
            >
              🚀 Bulk Approve SSA
            </button>
            <button
              onClick={handleBulkReject}
              disabled={bulkLoading}
              className="btn-ghost !py-1 !px-3 text-xs text-ember-400 border-ember-500/30 hover:bg-ember-500/10"
            >
              🚫 Bulk Reject
            </button>
            <button
              onClick={clearSelectedProposals}
              className="text-ink-400 hover:text-white px-2 py-1"
            >
              Clear
            </button>
          </div>
        )}
      </div>

      {/* Proposals Data Table */}
      <div className="panel">
        <DataTable<ProposalListItem>
          rows={filtered}
          rowKey={(r) => r.name}
          onRowClick={(r) => setDrawerProposal(r.name)}
          empty={
            <div className="py-12 text-center">
              <div className="font-medium text-ink-100">No proposals match your filters.</div>
              <div className="text-ink-500 mt-1 text-sm">
                Create an OptimizationPolicy targeting your workloads to generate recommendations.
              </div>
            </div>
          }
          columns={[
            {
              key: "select",
              title: (
                <input
                  type="checkbox"
                  checked={allFilteredSelected}
                  onChange={toggleSelectAll}
                  className="rounded border-white/20 bg-ink-800 text-mint-500 focus:ring-mint-500/40"
                  onClick={(e) => e.stopPropagation()}
                />
              ),
              render: (r) => (
                <div onClick={(e) => e.stopPropagation()}>
                  <input
                    type="checkbox"
                    checked={selectedProposals.includes(r.name)}
                    onChange={() => toggleProposalSelection(r.name)}
                    className="rounded border-white/20 bg-ink-800 text-mint-500 focus:ring-mint-500/40"
                  />
                </div>
              ),
            },
            {
              key: "workload",
              title: "Workload",
              render: (r) => (
                <div>
                  <div className="flex items-center gap-2">
                    <span className="font-medium text-sm">{r.namespace}/{r.workloadName}</span>
                    {r.cluster ? (
                      <span className="chip bg-white/[0.03] text-ink-500 border border-white/10">
                        {r.cluster}
                      </span>
                    ) : null}
                  </div>
                  <div className="font-mono text-xs text-ink-500 mt-0.5">
                    {r.workloadKind} · {r.name}
                  </div>
                </div>
              ),
            },
            { key: "recommendation", title: "Recommendation" },
            {
              key: "phase",
              title: "Status",
              render: (r) => (
                <div className="flex flex-wrap items-center gap-2">
                  <PhaseChip phase={r.phase} />
                  <ApprovalChip state={r.approvalState} />
                  {r.externalPRID ? (
                    <span className="chip bg-white/[0.03] text-ink-100/70 border border-white/10">
                      PR · {r.externalPRID}
                    </span>
                  ) : null}
                </div>
              ),
            },
            {
              key: "safety",
              title: "Safety",
              render: (r) => (
                <div className="flex flex-wrap items-center gap-2">
                  <ConfidenceBadge score={r.confidenceScore} />
                  <RiskBadge score={r.riskScore} />
                </div>
              ),
            },
            {
              key: "impact",
              title: "Estimated impact",
              className: "text-right",
              render: (r) => (
                <div className="text-right">
                  {r.estimatedSavings ? (
                    <div className="font-mono text-mint-400 font-semibold">
                      {formatCurrency(Number(r.estimatedSavings))}/mo
                    </div>
                  ) : null}
                  {r.carbonSavedHuman ? (
                    <div className="text-xs text-amber-400 mt-0.5">{r.carbonSavedHuman}/mo</div>
                  ) : null}
                </div>
              ),
            },
            {
              key: "actions",
              title: "",
              render: (r) => (
                <div
                  className="flex flex-wrap items-center justify-end gap-2"
                  onClick={(e) => e.stopPropagation()}
                >
                  {r.approvalState === "Pending" ? (
                    <>
                      <button
                        onClick={() => approveRow(r)}
                        className="btn-primary !py-1 !px-3 text-xs bg-mint-500/90 hover:bg-mint-500"
                      >
                        Approve
                      </button>
                      <button
                        onClick={() => rejectRow(r)}
                        className="btn-ghost !py-1 !px-3 text-xs hover:bg-ember-500/10 hover:text-ember-400 hover:border-ember-500/30"
                      >
                        Reject
                      </button>
                    </>
                  ) : r.phase === "Completed" || r.phase === "Stabilizing" ? (
                    <button
                      onClick={async () => {
                        try {
                          await api.proposals.rollback(r.name);
                          pushToast({ kind: "warn", title: "Rollback scheduled", message: r.name });
                          refresh();
                        } catch (e) {
                          pushToast({
                            kind: "error",
                            title: "Rollback failed",
                            message: e instanceof Error ? e.message : String(e),
                          });
                        }
                      }}
                      className="btn-ghost !py-1 !px-3 text-xs hover:bg-ember-500/10 hover:text-ember-400 hover:border-ember-500/30"
                    >
                      Rollback
                    </button>
                  ) : null}
                </div>
              ),
            },
          ]}
        />
      </div>

      {/* Proposal Detail Inspector Drawer */}
      <ProposalDetailDrawer
        proposalName={drawerProposal}
        onClose={() => setDrawerProposal(null)}
      />
    </div>
  );
}
