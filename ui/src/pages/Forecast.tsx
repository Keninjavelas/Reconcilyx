import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { Area, CartesianGrid, ComposedChart, Line, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import PageHeader from "@/components/PageHeader";
import Icon from "@/components/Icon";
import { api } from "@/lib/api";
import { useApi } from "@/lib/useApi";
import type { ForecastPoint, WhatIfResponse } from "@/lib/types";
import { formatCurrency, useAppStore } from "@/store/app";
import clsx from "clsx";

interface WorkloadOption {
  key: string;
  name: string;
  namespace: string;
  kind: string;
  cpuBefore: string;
  memoryBefore: string;
  cpuAfter?: string;
  memoryAfter?: string;
}

function parseCpu(value?: string): number | undefined {
  if (!value) return undefined;
  if (value.endsWith("m")) {
    const milli = Number(value.slice(0, -1));
    return Number.isFinite(milli) ? milli : undefined;
  }
  const cores = Number(value);
  return Number.isFinite(cores) ? cores * 1000 : undefined;
}

function parseMemoryMi(value?: string): number | undefined {
  if (!value) return undefined;
  const match = value.match(/^([0-9.]+)([KMGTP]i?)?$/);
  if (!match) return undefined;
  const amount = Number(match[1]);
  const unit = match[2] ?? "";
  const factors: Record<string, number> = {
    "": 1 / (1024 * 1024),
    Ki: 1 / 1024,
    Mi: 1,
    Gi: 1024,
    Ti: 1024 * 1024,
    K: 1e3 / (1024 * 1024),
    M: 1e6 / (1024 * 1024),
    G: 1e9 / (1024 * 1024),
    T: 1e12 / (1024 * 1024),
  };
  const factor = factors[unit];
  if (factor === undefined || !Number.isFinite(amount)) return undefined;
  return Math.max(1, Math.round(amount * factor));
}

function cpuQuantity(milli: number): string {
  return milli % 1000 === 0 ? String(milli / 1000) : `${Math.round(milli)}m`;
}

function defaultCpu(base: number): number {
  return Math.max(10, Math.min(base, Math.round((base * 0.7) / 10) * 10));
}

function defaultMemory(base: number): number {
  return Math.max(32, Math.min(base, Math.round((base * 0.75) / 32) * 32));
}

function timeLabel(epoch: number): string {
  return new Date(epoch * 1000).toLocaleTimeString([], {
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
}

export default function Forecast() {
  const pushToast = useAppStore((s) => s.pushToast);
  const proposalsQuery = useApi(api.proposals.list, { limit: 200, sortBy: "-createdAt" });

  const options = useMemo<WorkloadOption[]>(() => {
    const seen = new Map<string, WorkloadOption>();
    for (const proposal of proposalsQuery.data ?? []) {
      if (!proposal.cpuBefore || !proposal.memoryBefore) continue;
      const key = `${proposal.namespace}/${proposal.workloadName}`;
      if (seen.has(key)) continue;
      seen.set(key, {
        key,
        name: proposal.workloadName,
        namespace: proposal.namespace,
        kind: proposal.workloadKind,
        cpuBefore: proposal.cpuBefore,
        memoryBefore: proposal.memoryBefore,
        cpuAfter: proposal.cpuAfter,
        memoryAfter: proposal.memoryAfter,
      });
    }
    return [...seen.values()];
  }, [proposalsQuery.data]);

  const [override, setOverride] = useState<{ key: string; cpu: number; mem: number } | null>(null);
  const selectedKey =
    override && options.some((option) => option.key === override.key)
      ? override.key
      : (options[0]?.key ?? "");
  const selected = options.find((option) => option.key === selectedKey) ?? null;

  const cpuBase = parseCpu(selected?.cpuBefore) ?? 0;
  const memBase = parseMemoryMi(selected?.memoryBefore) ?? 0;
  const cpuTarget = override?.key === selectedKey ? override.cpu : defaultCpu(cpuBase || 100);
  const memTarget = override?.key === selectedKey ? override.mem : defaultMemory(memBase || 128);

  const forecastQuery = useApi(
    (workload: string, namespace: string) =>
      workload ? api.forecast.get(workload, namespace) : Promise.resolve<ForecastPoint[]>([]),
    selected?.name ?? "",
    selected?.namespace ?? "",
  );

  const [whatIf, setWhatIf] = useState<WhatIfResponse | null>(null);
  const [whatIfLoading, setWhatIfLoading] = useState(false);
  const [whatIfError, setWhatIfError] = useState<string | undefined>(undefined);

  const workloadName = selected?.name ?? "";
  const namespace = selected?.namespace ?? "";
  const workloadKind = selected?.kind ?? "";
  const currentCPU = selected?.cpuBefore ?? "";
  const currentMemory = selected?.memoryBefore ?? "";

  useEffect(() => {
    if (!selected || cpuBase <= 0 || memBase <= 0) {
      setWhatIf(null);
      setWhatIfError(undefined);
      return;
    }
    let active = true;
    setWhatIfLoading(true);
    setWhatIfError(undefined);
    const timer = window.setTimeout(() => {
      api.forecast
        .whatIf({
          namespace,
          workloadName,
          workloadKind,
          currentCPU,
          currentMemory,
          proposedCPU: cpuQuantity(cpuTarget),
          proposedMemory: `${Math.round(memTarget)}Mi`,
        })
        .then((response) => {
          if (active) {
            setWhatIf(response);
            setWhatIfError(undefined);
          }
        })
        .catch((error: unknown) => {
          if (active) {
            setWhatIf(null);
            setWhatIfError(error instanceof Error ? error.message : String(error));
          }
        })
        .finally(() => {
          if (active) setWhatIfLoading(false);
        });
    }, 350);

    return () => {
      active = false;
      window.clearTimeout(timer);
    };
  }, [selectedKey, cpuTarget, memTarget, namespace, workloadName, workloadKind, currentCPU, currentMemory, cpuBase, memBase, selected]);

  const chartData = (forecastQuery.data ?? []).map((point) => ({
    time: timeLabel(point.t),
    actual: point.actualCpu ? Math.round(point.actualCpu * 1000) : null,
    proposed: point.predCpu ? Math.round(point.predCpu * 1000) : null,
  }));
  const hasTelemetry = chartData.some((point) => point.actual !== null || point.proposed !== null);

  function selectWorkload(key: string) {
    const option = options.find((item) => item.key === key);
    if (!option) return;
    const cpu = parseCpu(option.cpuBefore) ?? 100;
    const mem = parseMemoryMi(option.memoryBefore) ?? 128;
    setOverride({ key, cpu: defaultCpu(cpu), mem: defaultMemory(mem) });
  }

  function setCpu(value: number) {
    if (!selected) return;
    setOverride({ key: selected.key, cpu: value, mem: memTarget });
  }

  function setMemory(value: number) {
    if (!selected) return;
    setOverride({ key: selected.key, cpu: cpuTarget, mem: value });
  }

  async function copyScenario() {
    if (!selected) return;
    const scenario = {
      namespace,
      workloadName,
      workloadKind,
      currentCPU,
      currentMemory,
      proposedCPU: cpuQuantity(cpuTarget),
      proposedMemory: `${Math.round(memTarget)}Mi`,
    };
    try {
      await navigator.clipboard.writeText(JSON.stringify(scenario, null, 2));
      pushToast({
        kind: "success",
        title: "Scenario copied",
        message: "The what-if request is on your clipboard.",
      });
    } catch {
      pushToast({
        kind: "error",
        title: "Clipboard unavailable",
        message: JSON.stringify(scenario),
      });
    }
  }

  return (
    <div className="space-y-8">
      <PageHeader
        index="03"
        eyebrow="Forecast & simulation"
        title={
          <>
            Model the next request,{" "}
            <span className="editorial text-mint-500">before you make it.</span>
          </>
        }
        description="The playground reads workloads with recorded requests from the proposal ledger. Adjust CPU and memory, then ask the control plane how the change scores against its own safety model."
        actions={
          <Link to="/proposals" className="btn-ghost">
            Open proposals
            <Icon name="arrow-up-right" size={14} />
          </Link>
        }
        meta={
          <>
            <span>{options.length} workloads with recorded requests</span>
            <span>Telemetry from the control plane</span>
            <span>In-place resize requires Kubernetes 1.27+</span>
          </>
        }
      />

      {options.length === 0 ? (
        <div className="panel px-6 py-16 text-center">
          <div className="font-display text-[24px] tracking-[-0.04em] text-ink-50">
            No workloads with recorded requests.
          </div>
          <p className="mx-auto mt-3 max-w-[56ch] text-[13px] leading-relaxed text-ink-400">
            The simulator only lists workloads that already have an OptimizationProposal with an
            observed CPU and memory request. Generate proposals first, then return here to model a
            different request.
          </p>
          {proposalsQuery.error ? (
            <p className="mx-auto mt-3 max-w-[56ch] break-words text-[12px] text-ember-400">
              {proposalsQuery.error}
            </p>
          ) : null}
          <div className="mt-5 flex flex-wrap justify-center gap-2">
            {proposalsQuery.error ? (
              <button className="btn-ghost" onClick={proposalsQuery.refetch}>
                Retry
              </button>
            ) : null}
            <Link to="/policies" className="btn-primary">
              Review policies
            </Link>
          </div>
        </div>
      ) : (
        <>
          <div className="flex flex-wrap items-end justify-between gap-4 border-y border-white/[0.08] py-4">
            <div>
              <label className="field-label" htmlFor="workload-select">
                Workload
              </label>
              <select
                id="workload-select"
                value={selectedKey}
                onChange={(event) => selectWorkload(event.target.value)}
                className="input h-10 w-[min(420px,80vw)] font-mono text-[12px]"
              >
                {options.map((option) => (
                  <option key={option.key} value={option.key}>
                    {option.namespace}/{option.name} · {option.kind}
                  </option>
                ))}
              </select>
            </div>
            <div className="flex flex-wrap gap-5 font-mono text-[10px] uppercase tracking-[0.12em] text-ink-500">
              <span>
                Observed CPU <span className="normal-case text-ink-200">{selected?.cpuBefore ?? "—"}</span>
              </span>
              <span>
                Observed memory <span className="normal-case text-ink-200">{selected?.memoryBefore ?? "—"}</span>
              </span>
              {selected?.cpuAfter || selected?.memoryAfter ? (
                <span>
                  Proposal{" "}
                  <span className="normal-case text-mint-500">
                    {selected.cpuAfter ?? "—"} / {selected.memoryAfter ?? "—"}
                  </span>
                </span>
              ) : null}
            </div>
          </div>

          <section className="grid gap-4 xl:grid-cols-3">
            <div className="panel self-start">
              <div className="panel-header">
                <div>
                  <div className="eyebrow">Scenario controls</div>
                  <div className="panel-title mt-1">Change the request.</div>
                </div>
              </div>
              <div className="space-y-7 p-5 sm:p-6">
                <div>
                  <div className="flex items-baseline justify-between">
                    <label className="field-label" htmlFor="cpu-range">
                      CPU request
                    </label>
                    <span className="num text-[13px] text-mint-500">{cpuQuantity(cpuTarget)}</span>
                  </div>
                  <input
                    id="cpu-range"
                    type="range"
                    min={10}
                    max={Math.max(2000, Math.ceil(cpuBase / 500) * 1000 + 1000)}
                    step={10}
                    value={cpuTarget}
                    onChange={(event) => setCpu(Number(event.target.value))}
                    className="range"
                  />
                  <div className="flex justify-between font-mono text-[9px] text-ink-600">
                    <span>10m</span>
                    <span>observed {selected?.cpuBefore ?? "—"}</span>
                    <span>{cpuQuantity(Math.max(2000, Math.ceil(cpuBase / 500) * 1000 + 1000))}</span>
                  </div>
                </div>

                <div>
                  <div className="flex items-baseline justify-between">
                    <label className="field-label" htmlFor="memory-range">
                      Memory request
                    </label>
                    <span className="num text-[13px] text-mint-500">{Math.round(memTarget)}Mi</span>
                  </div>
                  <input
                    id="memory-range"
                    type="range"
                    min={32}
                    max={Math.max(4096, Math.ceil(memBase / 1024) * 2048)}
                    step={32}
                    value={memTarget}
                    onChange={(event) => setMemory(Number(event.target.value))}
                    className="range"
                  />
                  <div className="flex justify-between font-mono text-[9px] text-ink-600">
                    <span>32Mi</span>
                    <span>observed {selected?.memoryBefore ?? "—"}</span>
                    <span>{Math.max(4096, Math.ceil(memBase / 1024) * 2048)}Mi</span>
                  </div>
                </div>

                <div className="flex flex-wrap gap-2">
                  <button
                    className="btn-ghost h-8 px-3 text-[10px]"
                    onClick={() => {
                      if (!selected) return;
                      setOverride({
                        key: selected.key,
                        cpu: Math.max(10, cpuBase),
                        mem: Math.max(32, memBase),
                      });
                    }}
                  >
                    Use observed
                  </button>
                  {selected?.cpuAfter || selected?.memoryAfter ? (
                    <button
                      className="btn-ghost h-8 px-3 text-[10px]"
                      onClick={() => {
                        if (!selected) return;
                        setOverride({
                          key: selected.key,
                          cpu: parseCpu(selected.cpuAfter) ?? cpuTarget,
                          mem: parseMemoryMi(selected.memoryAfter) ?? memTarget,
                        });
                      }}
                    >
                      Use proposal
                    </button>
                  ) : null}
                  <button className="btn-ghost ml-auto h-8 px-3 text-[10px]" onClick={copyScenario}>
                    <Icon name="copy" size={12} />
                    Copy scenario
                  </button>
                </div>

                <div className="rounded-xl border border-white/[0.09] bg-white/[0.025] p-4">
                  <div className="font-mono text-[9px] uppercase tracking-[0.16em] text-ink-500">
                    Request payload
                  </div>
                  <pre className="mt-2 overflow-x-auto font-mono text-[10px] leading-relaxed text-ink-300">
{`current:  ${currentCPU} / ${currentMemory}
proposed: ${cpuQuantity(cpuTarget)} / ${Math.round(memTarget)}Mi`}
                  </pre>
                </div>
              </div>
            </div>

            <div className="space-y-4 xl:col-span-2">
              <div className="panel">
                <div className="panel-header">
                  <div>
                    <div className="eyebrow">Recorded telemetry</div>
                    <div className="panel-title mt-1">Observed CPU against the proposal.</div>
                  </div>
                  <div className="flex items-center gap-4 font-mono text-[9px] uppercase tracking-[0.12em] text-ink-500">
                    <span className="flex items-center gap-2">
                      <span className="h-px w-4 bg-mint-500" aria-hidden />
                      Observed
                    </span>
                    <span className="flex items-center gap-2">
                      <span className="h-px w-4 border-t border-dashed border-sky-500" aria-hidden />
                      Proposal
                    </span>
                  </div>
                </div>
                <div className="px-3 pb-4 pt-5 sm:px-5">
                  {forecastQuery.error ? (
                    <div className="flex h-[280px] flex-col items-center justify-center gap-3 rounded-xl border border-dashed border-ember-500/30 text-center">
                      <p className="max-w-[48ch] break-words text-[12px] leading-relaxed text-ember-400">
                        Forecast telemetry could not be loaded. {forecastQuery.error}
                      </p>
                      <button className="btn-ghost" onClick={forecastQuery.refetch}>
                        Retry
                      </button>
                    </div>
                  ) : forecastQuery.loading && !forecastQuery.data ? (
                    <div className="flex h-[280px] items-center justify-center font-mono text-[10px] uppercase tracking-[0.14em] text-ink-500">
                      Loading telemetry…
                    </div>
                  ) : !hasTelemetry ? (
                    <div className="flex h-[280px] flex-col items-center justify-center gap-2 rounded-xl border border-dashed border-white/10 text-center">
                      <span className="font-mono text-[10px] uppercase tracking-[0.16em] text-ink-500">
                        No telemetry samples
                      </span>
                      <span className="max-w-[44ch] text-[12px] leading-relaxed text-ink-400">
                        The control plane returned no recorded usage for this workload. The
                        what-if score below still uses the request values you supplied.
                      </span>
                    </div>
                  ) : (
                    <div className="h-[280px]">
                      <ResponsiveContainer width="100%" height="100%">
                        <ComposedChart data={chartData} margin={{ top: 8, right: 4, left: 0, bottom: 0 }}>
                          <CartesianGrid stroke="rgba(244,241,234,0.07)" vertical={false} />
                          <XAxis
                            dataKey="time"
                            stroke="transparent"
                            tick={{ fill: "#6D7368", fontSize: 10, fontFamily: "IBM Plex Mono" }}
                            axisLine={false}
                            tickLine={false}
                            minTickGap={44}
                          />
                          <YAxis
                            stroke="transparent"
                            tick={{ fill: "#6D7368", fontSize: 10, fontFamily: "IBM Plex Mono" }}
                            axisLine={false}
                            tickLine={false}
                            tickFormatter={(value) => `${value}m`}
                            width={56}
                          />
                          <Tooltip
                            contentStyle={{
                              background: "#0D0F0C",
                              border: "1px solid rgba(244,241,234,0.14)",
                              borderRadius: 10,
                              color: "#F4F1EA",
                              fontSize: 12,
                            }}
                            labelStyle={{
                              color: "#8E9488",
                              fontFamily: "IBM Plex Mono",
                              fontSize: 10,
                              letterSpacing: "0.08em",
                              textTransform: "uppercase",
                            }}
                            formatter={(value, name) => [
                              value === null ? "no sample" : `${Number(value).toFixed(0)}m`,
                              name === "actual" ? "Observed CPU" : "Proposal request",
                            ]}
                          />
                          <Area
                            type="monotone"
                            dataKey="actual"
                            stroke="#C9EF4A"
                            strokeWidth={1.6}
                            fill="rgba(201,239,74,0.09)"
                            connectNulls={false}
                            isAnimationActive={false}
                            activeDot={{ r: 3, fill: "#C9EF4A", stroke: "#070806", strokeWidth: 2 }}
                          />
                          <Line
                            type="monotone"
                            dataKey="proposed"
                            stroke="#8FB4B2"
                            strokeWidth={1.3}
                            strokeDasharray="4 4"
                            dot={false}
                            connectNulls={false}
                            isAnimationActive={false}
                          />
                        </ComposedChart>
                      </ResponsiveContainer>
                    </div>
                  )}
                </div>
              </div>

              <div className="panel">
                <div className="panel-header">
                  <div>
                    <div className="eyebrow">What-if score</div>
                    <div className="panel-title mt-1">The control plane&apos;s read.</div>
                  </div>
                  {whatIf ? (
                    <span
                      className={clsx(
                        "chip border",
                        whatIf.allowed
                          ? "border-mint-500/35 bg-mint-500/10 text-mint-500"
                          : "border-ember-500/35 bg-ember-500/10 text-ember-400",
                      )}
                    >
                      {whatIf.allowed ? "Within constraints" : "Flagged"}
                    </span>
                  ) : null}
                </div>
                <div className="p-5 sm:p-6">
                  {whatIfLoading ? (
                    <div className="space-y-3">
                      <div className="skeleton h-10 w-40 rounded" />
                      <div className="skeleton h-4 w-full rounded" />
                      <div className="skeleton h-4 w-4/5 rounded" />
                    </div>
                  ) : whatIfError ? (
                    <div className="rounded-xl border border-ember-500/30 bg-ember-500/[0.06] p-4">
                      <div className="text-[12px] font-medium text-ember-400">Simulation failed</div>
                      <p className="mt-1 break-words text-[12px] leading-relaxed text-ink-400">
                        {whatIfError}
                      </p>
                    </div>
                  ) : whatIf ? (
                    <div className="grid gap-5 sm:grid-cols-[1.2fr_1fr]">
                      <div>
                        <div className="eyebrow">Estimated monthly savings</div>
                        <div className="kpi-value mt-2 text-mint-500">
                          {formatCurrency(Number(whatIf.savingsUSDPerMonth))}
                        </div>
                        <p className="mt-3 text-[12px] leading-relaxed text-ink-400">
                          {whatIf.rationale}
                        </p>
                      </div>
                      <dl className="grid grid-cols-2 gap-px overflow-hidden rounded-xl border border-white/[0.08] bg-white/[0.07] sm:grid-cols-2">
                        <ScoreCell label="Risk" value={`${whatIf.riskScore}`} />
                        <ScoreCell label="Confidence" value={`${whatIf.confidenceScore}`} />
                        <ScoreCell label="CPU delta" value={whatIf.cpuDelta} />
                        <ScoreCell label="Memory delta" value={whatIf.memoryDelta} />
                      </dl>
                    </div>
                  ) : (
                    <div className="text-[12px] text-ink-500">Adjust the request to run a simulation.</div>
                  )}
                </div>
                <div className="border-t border-white/[0.08] px-5 py-3 font-mono text-[9px] uppercase leading-relaxed tracking-[0.12em] text-ink-600 sm:px-6">
                  Estimates come from the control plane&apos;s pricing model. They are not a billing statement.
                </div>
              </div>
            </div>
          </section>
        </>
      )}
    </div>
  );
}

function ScoreCell({ label, value }: { label: string; value: string }) {
  return (
    <div className="bg-ink-900 px-3 py-3">
      <div className="font-mono text-[8px] uppercase tracking-[0.14em] text-ink-500">{label}</div>
      <div className="num mt-1.5 text-[15px] text-ink-100">{value}</div>
    </div>
  );
}
