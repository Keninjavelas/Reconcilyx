import { Link, useParams, useSearchParams } from "react-router-dom";
import { useApi } from "@/lib/useApi";
import { api } from "@/lib/api";
import type { ProposalDetail } from "@/lib/types";
import BeforeAfterCard from "@/components/BeforeAfterCard";
import KpiCard from "@/components/KpiCard";
import Icon from "@/components/Icon";
import { ApprovalChip, ConfidenceBadge, PhaseChip, RiskBadge } from "@/components/Badges";
import { formatCarbonGrams, formatCurrency, useAppStore } from "@/store/app";
import clsx from "clsx";

const ROLLBACK_PHASES = new Set([
  "Verifying",
  "Stabilizing",
  "Completed",
  "Failed",
  "RollbackRequired",
  "RollbackVerification",
]);

export default function ProposalDetailPage() {
  const { name } = useParams();
  const [searchParams] = useSearchParams();
  const namespace = searchParams.get("namespace") ?? "default";
  const pushToast = useAppStore((s) => s.pushToast);
  const refresh = useAppStore((s) => s.refresh);

  const { data, loading, error, refetch } = useApi(
    (proposalName: string, proposalNamespace: string) =>
      api.proposals.detail(proposalName, proposalNamespace),
    name ?? "",
    namespace,
  );

  if (loading && !data) {
    return (
      <div className="space-y-6">
        <div className="skeleton h-4 w-40 rounded" />
        <div className="skeleton h-16 w-2/3 rounded-xl" />
        <div className="skeleton h-44 w-full rounded-2xl" />
        <div className="grid gap-4 xl:grid-cols-3">
          <div className="skeleton h-80 rounded-2xl xl:col-span-2" />
          <div className="skeleton h-80 rounded-2xl" />
        </div>
      </div>
    );
  }

  if (error || !data) {
    return (
      <div className="space-y-6">
        <Link
          to="/proposals"
          className="link-arrow font-mono text-[10px] uppercase tracking-[0.14em] text-ink-500 hover:text-mint-500"
        >
          <Icon name="arrow-left" size={12} />
          All proposals
        </Link>
        <div className="panel">
          <div className="px-6 py-16 text-center">
            <div className="font-display text-[22px] tracking-[-0.03em] text-ink-50">
              Proposal unavailable
            </div>
            <p className="mx-auto mt-2 max-w-[52ch] text-[13px] leading-relaxed text-ink-400">
              {error ?? `No proposal named ${name ?? ""} exists in namespace ${namespace}.`}
            </p>
            <div className="mt-5 flex justify-center gap-2">
              <button className="btn-ghost" onClick={refetch}>
                Retry
              </button>
              <Link to="/proposals" className="btn-primary">
                Back to proposals
              </Link>
            </div>
          </div>
        </div>
      </div>
    );
  }

  const detail: ProposalDetail = data;
  const summary = detail.summary;
  const phase = summary.phase;
  const approval = summary.approvalState;
  const canReview = approval === "Pending";
  const canRollback = ROLLBACK_PHASES.has(phase);
  const hasSnapshot = Boolean(detail.raw.status?.executionSnapshot);
  const savings = detail.expected.savingsUSD;
  const carbon = detail.expected.carbonSavedGrams;
  const resourceDelta = detail.expected.resourceDelta ?? {};
  const conditions = detail.raw.status?.conditions ?? [];

  async function approve() {
    try {
      await api.proposals.approve({ name: summary.name, namespace: summary.namespace });
      pushToast({ kind: "success", title: "Proposal approved", message: summary.name });
      refresh();
      refetch();
    } catch (err) {
      pushToast({
        kind: "error",
        title: "Approval failed",
        message: err instanceof Error ? err.message : String(err),
      });
    }
  }

  async function reject() {
    try {
      await api.proposals.reject({ name: summary.name, namespace: summary.namespace });
      pushToast({ kind: "warn", title: "Proposal rejected", message: summary.name });
      refresh();
      refetch();
    } catch (err) {
      pushToast({
        kind: "error",
        title: "Rejection failed",
        message: err instanceof Error ? err.message : String(err),
      });
    }
  }

  async function rollback() {
    try {
      await api.proposals.rollback({ name: summary.name, namespace: summary.namespace });
      pushToast({ kind: "warn", title: "Rollback requested", message: summary.name });
      refresh();
      refetch();
    } catch (err) {
      pushToast({
        kind: "error",
        title: "Rollback failed",
        message: err instanceof Error ? err.message : String(err),
      });
    }
  }

  return (
    <div className="space-y-8">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Link
          to="/proposals"
          className="link-arrow font-mono text-[10px] uppercase tracking-[0.14em] text-ink-500 hover:text-mint-500"
        >
          <Icon name="arrow-left" size={12} />
          All proposals
        </Link>
        <span className="font-mono text-[9px] uppercase tracking-[0.14em] text-ink-600">
          {namespace} / {summary.name}
        </span>
      </div>

      <header className="border-b border-white/[0.1] pb-8">
        <div className="flex flex-wrap items-start justify-between gap-6">
          <div className="min-w-0 max-w-3xl">
            <div className="flex items-center gap-3">
              <span className="font-mono text-[10px] tracking-[0.2em] text-mint-500">02 / PROPOSAL</span>
              <span className="h-px w-8 bg-white/20" aria-hidden />
              <span className="eyebrow">{summary.workloadKind}</span>
            </div>
            <h1 className="mt-4 break-words font-display text-[clamp(2rem,4.2vw,3.75rem)] font-medium leading-[0.95] tracking-[-0.055em] text-ink-50">
              {summary.namespace}/<span className="editorial italic text-mint-500">{summary.workloadName}</span>
            </h1>
            <div className="mt-4 flex flex-wrap items-center gap-2">
              <PhaseChip phase={phase} />
              <ApprovalChip state={approval} />
              <ConfidenceBadge score={summary.confidenceScore} />
              <RiskBadge score={summary.riskScore} />
            </div>
            <div className="mt-4 font-mono text-[10px] uppercase tracking-[0.12em] text-ink-500">
              Created {new Date(summary.createdAt).toLocaleString()}
              {detail.raw.status?.policyApplied ? ` · policy ${detail.raw.status.policyApplied}` : ""}
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {canReview ? (
              <>
                <button className="btn-primary" onClick={approve}>
                  Approve proposal
                </button>
                <button className="btn-danger" onClick={reject}>
                  Reject
                </button>
              </>
            ) : null}
            {canRollback ? (
              <button
                className="btn-ghost"
                onClick={rollback}
                disabled={!hasSnapshot}
                title={hasSnapshot ? "Restore the pre-actuation snapshot" : "No execution snapshot is available"}
              >
                Roll back
              </button>
            ) : null}
            {!canReview && !canRollback ? (
              <span className="font-mono text-[10px] uppercase tracking-[0.12em] text-ink-500">
                No actions in this phase
              </span>
            ) : null}
          </div>
        </div>
      </header>

      <section className="grid grid-cols-1 gap-px overflow-hidden rounded-2xl border border-white/[0.09] bg-white/[0.07] sm:grid-cols-2 xl:grid-cols-4">
        <KpiCard
          className="bg-ink-900"
          label="Monthly savings"
          value={savings ? formatCurrency(Number(savings)) : "—"}
          detail="Estimated by the analyzer from observed resource usage."
        />
        <KpiCard
          className="bg-ink-900"
          label="Carbon abatement"
          value={
            summary.carbonSavedHuman ??
            (carbon !== undefined ? formatCarbonGrams(carbon) : "—")
          }
          detail="Grid-intensity-adjusted estimate for the same window."
        />
        <KpiCard
          className="bg-ink-900"
          label="Confidence"
          value={`${summary.confidenceScore}`}
          detail="Analyzer confidence out of 100."
        />
        <KpiCard
          className="bg-ink-900"
          label="Risk"
          value={`${summary.riskScore}`}
          detail={
            summary.riskScore >= 70
              ? "High risk — review the rationale before approval."
              : summary.riskScore >= 40
                ? "Moderate risk — the execution gate applies extra checks."
                : "Low risk under the active policy constraints."
          }
        />
      </section>

      <section className="grid gap-4 xl:grid-cols-3">
        <div className="space-y-4 xl:col-span-2">
          <BeforeAfterCard
            eyebrow="In-place pod resize"
            title="Requested resource change"
            rows={[
              {
                label: "CPU request",
                before: summary.cpuBefore ?? "—",
                after: summary.cpuAfter ?? "—",
                note: "Observed current request vs. proposed request",
              },
              {
                label: "Memory request",
                before: summary.memoryBefore ?? "—",
                after: summary.memoryAfter ?? "—",
                note: "Observed current request vs. proposed request",
              },
            ]}
          />

          <div className="panel">
            <div className="panel-header">
              <div>
                <div className="eyebrow">Analyzer rationale</div>
                <div className="panel-title mt-1">Why this proposal exists.</div>
              </div>
            </div>
            <div className="px-5 py-5 sm:px-6">
              <p className="text-[13px] leading-relaxed text-ink-300">
                {detail.rationale || "The analyzer did not record a rationale for this proposal."}
              </p>
              {summary.rationale && summary.rationale !== detail.rationale ? (
                <p className="mt-3 border-t border-white/[0.07] pt-3 text-[12px] leading-relaxed text-ink-500">
                  {summary.rationale}
                </p>
              ) : null}
            </div>
          </div>

          <div className="panel">
            <div className="panel-header">
              <div>
                <div className="eyebrow">Event timeline</div>
                <div className="panel-title mt-1">What the control plane recorded.</div>
              </div>
              <span className="font-mono text-[9px] uppercase tracking-[0.14em] text-ink-500">
                {detail.events.length} events
              </span>
            </div>
            {detail.events.length === 0 ? (
              <div className="px-5 py-10 text-center text-[12px] leading-relaxed text-ink-500">
                No Kubernetes events recorded for this proposal yet.
              </div>
            ) : (
              <ol className="px-5 py-2 sm:px-6">
                {detail.events.map((event, index) => (
                  <li
                    key={`${event.at}-${index}`}
                    className="flex gap-3 border-b border-white/[0.055] py-3.5 last:border-0"
                  >
                    <span
                      className={clsx(
                        "mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full",
                        event.type === "Warning" ? "bg-ember-500" : "bg-mint-500",
                      )}
                      aria-hidden
                    />
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-baseline gap-2">
                        <span className="text-[12px] text-ink-200">{event.reason}</span>
                        <span className="font-mono text-[9px] text-ink-500">
                          {new Date(event.at).toLocaleString()}
                        </span>
                        {event.count && event.count > 1 ? (
                          <span className="font-mono text-[9px] text-ink-500">×{event.count}</span>
                        ) : null}
                      </div>
                      <p className="mt-1 break-words text-[12px] leading-relaxed text-ink-400">
                        {event.message}
                      </p>
                    </div>
                  </li>
                ))}
              </ol>
            )}
          </div>
        </div>

        <div className="space-y-4">
          <div className="panel">
            <div className="panel-header">
              <div>
                <div className="eyebrow">Telemetry baseline</div>
                <div className="panel-title mt-1">Observed before approval.</div>
              </div>
            </div>
            <dl className="divide-y divide-white/[0.055]">
              <MetricRow label="Observed CPU request" value={detail.observed.cpu ?? "—"} />
              <MetricRow label="Observed memory request" value={detail.observed.memory ?? "—"} />
              <MetricRow label="P95 CPU usage" value={detail.observed.p95Cpu ?? "not reported"} />
              <MetricRow label="P95 memory usage" value={detail.observed.p95Memory ?? "not reported"} />
              <MetricRow label="Proposed CPU" value={detail.proposed.cpu ?? "—"} />
              <MetricRow label="Proposed memory" value={detail.proposed.memory ?? "—"} />
            </dl>
          </div>

          <div className="panel">
            <div className="panel-header">
              <div>
                <div className="eyebrow">Expected impact</div>
                <div className="panel-title mt-1">Cost and carbon delta.</div>
              </div>
            </div>
            <dl className="divide-y divide-white/[0.055]">
              <MetricRow label="Monthly savings" value={savings ? formatCurrency(Number(savings)) : "—"} />
              <MetricRow
                label="Carbon saved"
                value={summary.carbonSavedHuman ?? (carbon !== undefined ? formatCarbonGrams(carbon) : "—")}
              />
              {Object.entries(resourceDelta).map(([key, value]) => (
                <MetricRow key={key} label={`Δ ${key}`} value={value} />
              ))}
              <MetricRow label="GitOps PR" value={detail.raw.status?.externalPRID ?? "None"} />
            </dl>
          </div>

          <div className="panel">
            <div className="panel-header">
              <div>
                <div className="eyebrow">Execution record</div>
                <div className="panel-title mt-1">Actuation evidence.</div>
              </div>
            </div>
            <dl className="divide-y divide-white/[0.055]">
              <MetricRow label="Execution state" value={detail.raw.status?.executionState || "not reported"} />
              <MetricRow label="Policy" value={detail.raw.status?.policyApplied ?? "not recorded"} />
              <MetricRow label="Field manager" value={detail.raw.status?.executionMetadata?.fieldManager ?? "not recorded"} />
              <MetricRow label="Execution ID" value={detail.raw.status?.executionMetadata?.executionID ?? "not recorded"} />
              <MetricRow
                label="Snapshot"
                value={hasSnapshot ? "captured before actuation" : "not recorded"}
              />
              <MetricRow label="Yield count" value={String(detail.raw.status?.yieldCount ?? 0)} />
              <MetricRow
                label="Expires"
                value={
                  detail.raw.status?.expirationTime
                    ? new Date(detail.raw.status.expirationTime).toLocaleString()
                    : "no expiry reported"
                }
              />
            </dl>
          </div>

          {conditions.length > 0 ? (
            <div className="panel">
              <div className="panel-header">
                <div>
                  <div className="eyebrow">Conditions</div>
                  <div className="panel-title mt-1">Controller reports.</div>
                </div>
              </div>
              <ul className="divide-y divide-white/[0.055]">
                {conditions.map((condition) => (
                  <li key={condition.type} className="px-5 py-3.5">
                    <div className="flex items-center justify-between gap-3">
                      <span className="text-[12px] text-ink-200">{condition.type}</span>
                      <span
                        className={clsx(
                          "font-mono text-[9px] uppercase tracking-[0.12em]",
                          condition.status === "True" ? "text-mint-500" : "text-ink-500",
                        )}
                      >
                        {condition.status}
                      </span>
                    </div>
                    {condition.message ? (
                      <p className="mt-1 text-[11px] leading-relaxed text-ink-500">{condition.message}</p>
                    ) : null}
                  </li>
                ))}
              </ul>
            </div>
          ) : null}

          {detail.migrationPlan ? (
            <div className="panel">
              <div className="panel-header">
                <div>
                  <div className="eyebrow">Migration plan</div>
                  <div className="panel-title mt-1">Cross-cloud target.</div>
                </div>
              </div>
              <dl className="divide-y divide-white/[0.055]">
                <MetricRow label="Source cluster" value={detail.migrationPlan.sourceCluster ?? "—"} />
                <MetricRow label="Target" value={`${detail.migrationPlan.targetCloud} / ${detail.migrationPlan.targetRegion}`} />
                <MetricRow label="Instance" value={detail.migrationPlan.targetInstance} />
                <MetricRow label="Cost delta" value={formatCurrency(detail.migrationPlan.costDeltaUSD)} />
                <MetricRow
                  label="Carbon delta"
                  value={formatCarbonGrams(detail.migrationPlan.carbonSavedGrams)}
                />
              </dl>
              {detail.migrationPlan.migrationScript ? (
                <pre className="mx-5 mb-5 mt-1 max-h-52 overflow-auto rounded-lg border border-white/[0.08] bg-black/30 p-3 font-mono text-[10px] leading-relaxed text-ink-300">
                  {detail.migrationPlan.migrationScript}
                </pre>
              ) : null}
            </div>
          ) : null}
        </div>
      </section>
    </div>
  );
}

function MetricRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-start justify-between gap-4 px-5 py-3">
      <dt className="text-[11px] text-ink-500">{label}</dt>
      <dd className="max-w-[60%] break-all text-right font-mono text-[11px] text-ink-200">{value}</dd>
    </div>
  );
}
