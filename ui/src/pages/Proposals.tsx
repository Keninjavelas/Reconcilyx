import { useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import DataTable from "@/components/DataTable";
import PageHeader from "@/components/PageHeader";
import Icon from "@/components/Icon";
import {
  ApprovalChip,
  ConfidenceBadge,
  PhaseChip,
  RecommendationLabel,
  RiskBadge,
} from "@/components/Badges";
import ProposalDetailDrawer from "@/components/ProposalDetailDrawer";
import { api } from "@/lib/api";
import { useApi } from "@/lib/useApi";
import { formatCurrency, useAppStore } from "@/store/app";
import type { ProposalListItem, ProposalRef } from "@/lib/types";
import clsx from "clsx";

type FilterKey = "all" | "Pending" | "Approved" | "Rejected";

const FILTERS: { key: FilterKey; label: string }[] = [
  { key: "all", label: "All" },
  { key: "Pending", label: "Pending" },
  { key: "Approved", label: "Approved" },
  { key: "Rejected", label: "Rejected" },
];

const ROLLBACK_PHASES = new Set([
  "Verifying",
  "Stabilizing",
  "Completed",
  "Failed",
  "RollbackRequired",
  "RollbackVerification",
]);

function proposalKey(proposal: ProposalListItem): string {
  return `${proposal.namespace}/${proposal.name}`;
}

export default function Proposals() {
  const [searchParams, setSearchParams] = useSearchParams();
  const pushToast = useAppStore((s) => s.pushToast);
  const refresh = useAppStore((s) => s.refresh);
  const selectedProposals = useAppStore((s) => s.selectedProposals);
  const toggleProposalSelection = useAppStore((s) => s.toggleProposalSelection);
  const selectAllProposals = useAppStore((s) => s.selectAllProposals);
  const clearSelectedProposals = useAppStore((s) => s.clearSelectedProposals);

  const paramFilter = searchParams.get("approvalState");
  const initialFilter: FilterKey =
    paramFilter === "Pending" || paramFilter === "Approved" || paramFilter === "Rejected"
      ? paramFilter
      : "all";

  const [filter, setFilter] = useState<FilterKey>(initialFilter);
  const [query, setQuery] = useState(searchParams.get("q") ?? "");
  const [inspected, setInspected] = useState<ProposalRef | null>(null);
  const [bulkLoading, setBulkLoading] = useState(false);
  const headerCheckbox = useRef<HTMLInputElement>(null);

  const { data, loading, error, refetch } = useApi(api.proposals.list, { sortBy: "-createdAt" });
  const rows = useMemo(() => data ?? [], [data]);

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return rows.filter((row) => {
      if (filter !== "all" && row.approvalState !== filter) return false;
      if (!needle) return true;
      return (
        row.workloadName.toLowerCase().includes(needle) ||
        row.namespace.toLowerCase().includes(needle) ||
        row.name.toLowerCase().includes(needle) ||
        row.workloadKind.toLowerCase().includes(needle)
      );
    });
  }, [rows, filter, query]);

  const selectable = filtered.filter((row) => row.approvalState === "Pending");
  const selectedRows = rows.filter((row) => selectedProposals.includes(proposalKey(row)));
  const selectedPending = selectedRows.filter((row) => row.approvalState === "Pending");
  const allFilteredSelected =
    selectable.length > 0 && selectable.every((row) => selectedProposals.includes(proposalKey(row)));
  const someSelected = selectable.some((row) => selectedProposals.includes(proposalKey(row)));

  useEffect(() => {
    if (headerCheckbox.current) {
      headerCheckbox.current.indeterminate = someSelected && !allFilteredSelected;
    }
  }, [someSelected, allFilteredSelected]);

  function setFilterLink(next: FilterKey) {
    setFilter(next);
    const params = new URLSearchParams(searchParams);
    if (next === "all") params.delete("approvalState");
    else params.set("approvalState", next);
    setSearchParams(params, { replace: true });
  }

  function toggleSelectAll() {
    if (allFilteredSelected) {
      clearSelectedProposals();
    } else {
      selectAllProposals(selectable.map(proposalKey));
    }
  }

  async function bulk(action: "approve" | "reject") {
    if (selectedPending.length === 0) return;
    setBulkLoading(true);
    const refs = selectedPending.map((row) => ({ name: row.name, namespace: row.namespace }));
    try {
      if (action === "approve") {
        await api.proposals.bulkApprove(refs);
        pushToast({
          kind: "success",
          title: "Proposals approved",
          message: `${refs.length} proposal${refs.length === 1 ? "" : "s"} approved.`,
        });
      } else {
        await api.proposals.bulkReject(refs);
        pushToast({
          kind: "warn",
          title: "Proposals rejected",
          message: `${refs.length} proposal${refs.length === 1 ? "" : "s"} rejected.`,
        });
      }
      clearSelectedProposals();
      refresh();
      refetch();
    } catch (err) {
      pushToast({
        kind: "error",
        title: `Bulk ${action} failed`,
        message: err instanceof Error ? err.message : String(err),
      });
    } finally {
      setBulkLoading(false);
    }
  }

  async function act(action: "approve" | "reject" | "rollback", row: ProposalListItem) {
    const ref = { name: row.name, namespace: row.namespace };
    try {
      if (action === "approve") {
        await api.proposals.approve(ref);
        pushToast({ kind: "success", title: "Proposal approved", message: row.name });
      } else if (action === "reject") {
        await api.proposals.reject(ref);
        pushToast({ kind: "warn", title: "Proposal rejected", message: row.name });
      } else {
        await api.proposals.rollback(ref);
        pushToast({ kind: "warn", title: "Rollback requested", message: row.name });
      }
      refresh();
      refetch();
    } catch (err) {
      pushToast({
        kind: "error",
        title: `Could not ${action}`,
        message: err instanceof Error ? err.message : String(err),
      });
    }
  }

  const counts: Record<FilterKey, number> = {
    all: rows.length,
    Pending: rows.filter((row) => row.approvalState === "Pending").length,
    Approved: rows.filter((row) => row.approvalState === "Approved").length,
    Rejected: rows.filter((row) => row.approvalState === "Rejected").length,
  };

  return (
    <div className="space-y-8">
      <PageHeader
        index="02"
        eyebrow="Governance & actuation"
        title={
          <>
            Every change starts as a{" "}
            <span className="editorial text-mint-500">proposal.</span>
          </>
        }
        description="Proposals carry the observed baseline, the proposed request, the confidence and risk scores, and the expected cost and carbon impact. Nothing reaches a workload without an approval path."
        actions={
          <>
            <button className="btn-ghost" onClick={() => { refresh(); refetch(); }}>
              <Icon name="refresh" size={14} />
              Refresh
            </button>
          </>
        }
        meta={
          <>
            <span>{rows.length} proposals</span>
            <span>{counts.Pending} awaiting review</span>
            <span>{counts.Approved} approved</span>
            <span>{counts.Rejected} rejected</span>
          </>
        }
      />

      {error ? (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-ember-500/30 bg-ember-500/[0.06] px-4 py-3 text-[12px] text-ember-300">
          <span className="break-words">Could not load proposals. {error}</span>
          <button className="btn-ghost h-8 px-3 text-[11px]" onClick={refetch}>
            Retry
          </button>
        </div>
      ) : null}

      <div className="flex flex-wrap items-center justify-between gap-3 border-y border-white/[0.08] py-3">
        <div className="flex flex-wrap gap-1 rounded-full border border-white/[0.1] p-1">
          {FILTERS.map((item) => (
            <button
              key={item.key}
              onClick={() => setFilterLink(item.key)}
              className={clsx(
                "flex items-center gap-2 rounded-full px-3 py-1.5 text-[11px] transition",
                filter === item.key
                  ? "bg-white/[0.07] text-ink-50"
                  : "text-ink-400 hover:text-ink-100",
              )}
              aria-pressed={filter === item.key}
            >
              {item.label}
              <span className="font-mono text-[9px] text-ink-500">{counts[item.key]}</span>
            </button>
          ))}
        </div>

        <div className="flex items-center gap-2">
          <label className="relative">
            <span className="sr-only">Search proposals</span>
            <Icon
              name="search"
              size={14}
              className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-ink-500"
            />
            <input
              className="input h-9 w-56 pl-9 text-[12px] sm:w-64"
              placeholder="Workload, namespace, id"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
            />
          </label>
        </div>
      </div>

      {selectedProposals.length > 0 ? (
        <div className="flex flex-wrap items-center gap-3 rounded-xl border border-mint-500/25 bg-mint-500/[0.05] px-4 py-2.5">
          <span className="font-mono text-[10px] uppercase tracking-[0.14em] text-mint-500">
            {selectedProposals.length} selected
          </span>
          <button
            className="btn-primary h-8 px-3 text-[11px]"
            disabled={bulkLoading || selectedPending.length === 0}
            onClick={() => bulk("approve")}
            title={
              selectedPending.length === 0
                ? "Only pending proposals can be approved"
                : undefined
            }
          >
            Approve pending ({selectedPending.length})
          </button>
          <button
            className="btn-danger h-8 px-3 text-[11px]"
            disabled={bulkLoading || selectedPending.length === 0}
            onClick={() => bulk("reject")}
          >
            Reject pending ({selectedPending.length})
          </button>
          <button className="text-[11px] text-ink-400 transition hover:text-ink-100" onClick={clearSelectedProposals}>
            Clear
          </button>
        </div>
      ) : null}

      <div className="panel lg:hidden">
        <div className="flex items-center justify-between gap-3 border-b border-white/[0.08] px-4 py-3">
          <span className="eyebrow">Proposal queue</span>
          <button
            className="font-mono text-[9px] uppercase tracking-[0.12em] text-ink-400 transition hover:text-mint-500 disabled:opacity-40"
            onClick={toggleSelectAll}
            disabled={selectable.length === 0}
          >
            {allFilteredSelected ? "Deselect pending" : "Select all pending"}
          </button>
        </div>
        {filtered.length === 0 ? (
          <div className="px-5 py-14 text-center text-[13px] leading-relaxed text-ink-400">
            {loading && !data
              ? "Loading proposals…"
              : query || filter !== "all"
                ? "No proposals match these filters."
                : "No proposals yet. Create an OptimizationPolicy to start evaluating workloads."}
          </div>
        ) : (
          <ul className="divide-y divide-white/[0.055]">
            {filtered.map((row) => {
              const key = proposalKey(row);
              const checked = selectedProposals.includes(key);
              return (
                <li
                  key={key}
                  className="cursor-pointer p-4 transition-colors hover:bg-mint-500/[0.025]"
                  onClick={() => setInspected({ name: row.name, namespace: row.namespace })}
                >
                  <div className="flex items-start gap-3">
                    <div onClick={(event) => event.stopPropagation()} className="pt-0.5">
                      <input
                        type="checkbox"
                        checked={checked}
                        onChange={() => toggleProposalSelection(key)}
                        disabled={row.approvalState !== "Pending"}
                        aria-label={`Select ${row.name}`}
                        className="h-4 w-4 accent-mint-500 disabled:opacity-25"
                      />
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
                        <span className="text-[13px] text-ink-100">
                          {row.namespace}/{row.workloadName}
                        </span>
                        {row.cluster ? (
                          <span className="rounded border border-white/[0.1] px-1.5 py-0.5 font-mono text-[8px] uppercase tracking-[0.1em] text-ink-500">
                            {row.cluster}
                          </span>
                        ) : null}
                      </div>
                      <div className="mt-0.5 truncate font-mono text-[9px] uppercase tracking-[0.1em] text-ink-500">
                        {row.workloadKind} · {row.name}
                      </div>
                    </div>
                    <span className="shrink-0 text-right">
                      <span className="num block text-[12px] text-mint-500">
                        {row.estimatedSavings ? `${formatCurrency(Number(row.estimatedSavings))}/mo` : "—"}
                      </span>
                      <span className="num mt-0.5 block text-[9px] text-ink-500">
                        {row.carbonSavedHuman ?? "carbon not reported"}
                      </span>
                    </span>
                  </div>

                  <div className="mt-3 flex flex-wrap items-center gap-1.5 pl-7">
                    <RecommendationLabel value={row.recommendation} />
                    <PhaseChip phase={row.phase} />
                    <ApprovalChip state={row.approvalState} />
                    <ConfidenceBadge score={row.confidenceScore} />
                    <RiskBadge score={row.riskScore} />
                    {row.externalPRID ? (
                      <span className="chip border border-white/10 bg-white/[0.03] text-ink-300">
                        PR {row.externalPRID}
                      </span>
                    ) : null}
                  </div>

                  <div
                    className="mt-3 flex items-center justify-end gap-2 pl-7"
                    onClick={(event) => event.stopPropagation()}
                  >
                    {row.approvalState === "Pending" ? (
                      <>
                        <button
                          onClick={() => act("approve", row)}
                          aria-label={`Approve ${row.name}`}
                          className="rounded-full border border-mint-500/40 bg-mint-500/10 px-3 py-1.5 font-mono text-[9px] uppercase tracking-[0.08em] text-mint-500 transition hover:bg-mint-500/20"
                        >
                          Approve
                        </button>
                        <button
                          onClick={() => act("reject", row)}
                          aria-label={`Reject ${row.name}`}
                          className="rounded-full border border-white/[0.12] px-3 py-1.5 font-mono text-[9px] uppercase tracking-[0.08em] text-ink-400 transition hover:border-ember-500/40 hover:text-ember-400"
                        >
                          Reject
                        </button>
                      </>
                    ) : ROLLBACK_PHASES.has(row.phase) ? (
                      <button
                        onClick={() => act("rollback", row)}
                        aria-label={`Roll back ${row.name}`}
                        className="rounded-full border border-white/[0.12] px-3 py-1.5 font-mono text-[9px] uppercase tracking-[0.08em] text-ink-400 transition hover:border-ember-500/40 hover:text-ember-400"
                      >
                        Roll back
                      </button>
                    ) : (
                      <span className="font-mono text-[9px] text-ink-600">no action available</span>
                    )}
                  </div>
                </li>
              );
            })}
          </ul>
        )}
      </div>

      <div className="panel hidden lg:block">
        <DataTable<ProposalListItem>
          rows={filtered}
          rowKey={(row) => proposalKey(row)}
          onRowClick={(row) => setInspected({ name: row.name, namespace: row.namespace })}
          empty={
            loading && !data ? (
              "Loading proposals…"
            ) : query || filter !== "all" ? (
              "No proposals match these filters."
            ) : (
              "No proposals yet. Create an OptimizationPolicy to start evaluating workloads."
            )
          }
          columns={[
            {
              key: "select",
              title: (
                <input
                  ref={headerCheckbox}
                  type="checkbox"
                  checked={allFilteredSelected}
                  onChange={toggleSelectAll}
                  disabled={selectable.length === 0}
                  aria-label="Select all pending proposals"
                  className="h-3.5 w-3.5 accent-mint-500"
                  onClick={(event) => event.stopPropagation()}
                />
              ),
              render: (row) => (
                <div onClick={(event) => event.stopPropagation()}>
                  <input
                    type="checkbox"
                    checked={selectedProposals.includes(proposalKey(row))}
                    onChange={() => toggleProposalSelection(proposalKey(row))}
                    disabled={row.approvalState !== "Pending"}
                    aria-label={`Select ${row.name}`}
                    className="h-3.5 w-3.5 accent-mint-500 disabled:opacity-30"
                  />
                </div>
              ),
            },
            {
              key: "workload",
              title: "Workload",
              render: (row) => (
                <div className="min-w-[190px]">
                  <div className="flex items-center gap-2">
                    <span className="text-[13px] text-ink-100">
                      {row.namespace}/{row.workloadName}
                    </span>
                    {row.cluster ? (
                      <span className="rounded border border-white/[0.1] px-1.5 py-0.5 font-mono text-[8px] uppercase tracking-[0.1em] text-ink-500">
                        {row.cluster}
                      </span>
                    ) : null}
                  </div>
                  <div className="mt-0.5 font-mono text-[9px] uppercase tracking-[0.1em] text-ink-500">
                    {row.workloadKind} · {row.name}
                  </div>
                </div>
              ),
            },
            {
              key: "recommendation",
              title: "Recommendation",
              render: (row) => <RecommendationLabel value={row.recommendation} />,
            },
            {
              key: "status",
              title: "Status",
              render: (row) => (
                <div className="flex flex-wrap gap-1.5">
                  <PhaseChip phase={row.phase} />
                  <ApprovalChip state={row.approvalState} />
                  {row.externalPRID ? (
                    <span className="chip border border-white/10 bg-white/[0.03] text-ink-300">
                      PR {row.externalPRID}
                    </span>
                  ) : null}
                </div>
              ),
            },
            {
              key: "safety",
              title: "Safety",
              render: (row) => (
                <div className="flex flex-wrap gap-1.5">
                  <ConfidenceBadge score={row.confidenceScore} />
                  <RiskBadge score={row.riskScore} />
                </div>
              ),
            },
            {
              key: "impact",
              title: "Est. impact",
              className: "text-right",
              render: (row) => (
                <div className="whitespace-nowrap text-right">
                  <div className="num text-[12px] text-mint-500">
                    {row.estimatedSavings ? `${formatCurrency(Number(row.estimatedSavings))}/mo` : "—"}
                  </div>
                  <div className="num mt-0.5 text-[10px] text-ink-500">
                    {row.carbonSavedHuman ?? "carbon not reported"}
                  </div>
                </div>
              ),
            },
            {
              key: "actions",
              title: "",
              render: (row) => (
                <div
                  className="flex items-center justify-end gap-1.5"
                  onClick={(event) => event.stopPropagation()}
                >
                  {row.approvalState === "Pending" ? (
                    <>
                      <button
                        onClick={() => act("approve", row)}
                        aria-label={`Approve ${row.name}`}
                        className="rounded-full border border-mint-500/40 bg-mint-500/10 px-2.5 py-1 font-mono text-[9px] uppercase tracking-[0.08em] text-mint-500 transition hover:bg-mint-500/20"
                      >
                        Approve
                      </button>
                      <button
                        onClick={() => act("reject", row)}
                        aria-label={`Reject ${row.name}`}
                        className="rounded-full border border-white/[0.12] px-2.5 py-1 font-mono text-[9px] uppercase tracking-[0.08em] text-ink-400 transition hover:border-ember-500/40 hover:text-ember-400"
                      >
                        Reject
                      </button>
                    </>
                  ) : ROLLBACK_PHASES.has(row.phase) ? (
                    <button
                      onClick={() => act("rollback", row)}
                      aria-label={`Roll back ${row.name}`}
                      className="rounded-full border border-white/[0.12] px-2.5 py-1 font-mono text-[9px] uppercase tracking-[0.08em] text-ink-400 transition hover:border-ember-500/40 hover:text-ember-400"
                    >
                      Roll back
                    </button>
                  ) : (
                    <span className="font-mono text-[9px] text-ink-600">—</span>
                  )}
                </div>
              ),
            },
          ]}
        />
      </div>

      <ProposalDetailDrawer proposal={inspected} onClose={() => setInspected(null)} />
    </div>
  );
}
