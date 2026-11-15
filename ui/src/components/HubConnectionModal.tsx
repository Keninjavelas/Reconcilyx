import { useState } from "react";
import { getApiBaseUrl, getApiKey, setApiBaseUrl, setApiKey, api } from "@/lib/api";
import { EVIDENCE_RUN_ID, EVIDENCE_VERIFIED_RUNS } from "@/lib/evidenceReplay";
import { useAppStore } from "@/store/app";
import Icon from "@/components/Icon";

export default function HubConnectionModal() {
  const open = useAppStore((s) => s.hubGateOpen);
  const setOpen = useAppStore((s) => s.setHubGateOpen);
  const replayMode = useAppStore((s) => s.replayMode);
  const setReplayMode = useAppStore((s) => s.setReplayMode);
  const pushToast = useAppStore((s) => s.pushToast);
  const refresh = useAppStore((s) => s.refresh);

  const [url, setUrl] = useState(() => getApiBaseUrl());
  const [key, setKey] = useState(() => getApiKey());
  const [testing, setTesting] = useState(false);
  const [diagnostic, setDiagnostic] = useState<{ ok: boolean; text: string } | null>(null);

  if (!open) return null;

  async function handleConnectLive(event: React.FormEvent) {
    event.preventDefault();
    setTesting(true);
    setDiagnostic(null);
    setApiBaseUrl(url.trim());
    setApiKey(key.trim());
    setReplayMode(false);
    try {
      const health = await api.healthz.get();
      setDiagnostic({
        ok: true,
        text: `Connected (${health.status}${health.version ? ` · ${health.version}` : ""}). Telemetry live.`,
      });
      refresh();
      pushToast({
        kind: "success",
        title: "Connected to live Hub",
        message: url.trim() || window.location.origin,
      });
      setOpen(false);
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      setDiagnostic({
        ok: false,
        text: `${msg} — Verify RECONCILYX_CORS_ORIGINS allows ${window.location.origin}, or install cluster CA via RECONCILYX_CA_BUNDLE.`,
      });
    } finally {
      setTesting(false);
    }
  }

  function handleEnterReplay() {
    setReplayMode(true);
    setOpen(false);
    pushToast({
      kind: "warn",
      title: "Read-Only Evidence Replay enabled",
      message: `Viewing verified Kind TLS snapshot #${EVIDENCE_RUN_ID}. Cluster mutations are disabled.`,
    });
  }

  return (
    <div
      className="overlay fixed inset-0 z-[70] flex items-center justify-center p-4"
      onClick={() => setOpen(false)}
      role="presentation"
    >
      <div
        className="panel w-full max-w-lg animate-fade-in-up"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-label="Control plane connection gate"
      >
        <div className="panel-header">
          <div>
            <div className="eyebrow">Control plane connection</div>
            <h2 className="panel-title mt-1">Connect Hub or inspect verified evidence</h2>
          </div>
          <button className="btn-icon" onClick={() => setOpen(false)} aria-label="Close connection gate">
            <span aria-hidden>×</span>
          </button>
        </div>

        <form onSubmit={handleConnectLive} className="space-y-4 p-5 sm:p-6">
          <div>
            <label className="field-label" htmlFor="gate-hub-url">
              Hub API Origin
            </label>
            <input
              id="gate-hub-url"
              type="url"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder="Leave blank for same-origin or enter https://hub.example.com:8082"
              className="input font-mono text-[12px]"
            />
          </div>

          <div>
            <label className="field-label" htmlFor="gate-api-key">
              API Key (X-API-Key)
            </label>
            <input
              id="gate-api-key"
              type="password"
              value={key}
              onChange={(e) => setKey(e.target.value)}
              placeholder="RECONCILYX_API_KEY"
              autoComplete="off"
              className="input font-mono text-[12px]"
            />
          </div>

          {diagnostic ? (
            <div
              className={
                diagnostic.ok
                  ? "rounded-xl border border-mint-500/35 bg-mint-500/[0.08] px-4 py-3 text-[11px] text-mint-400"
                  : "rounded-xl border border-ember-500/35 bg-ember-500/[0.08] px-4 py-3 text-[11px] text-ember-300"
              }
            >
              {diagnostic.text}
            </div>
          ) : null}

          <div className="flex flex-wrap items-center gap-2 pt-1">
            <button type="submit" className="btn-primary" disabled={testing}>
              <Icon name="refresh" size={13} />
              {testing ? "Testing Hub…" : "Connect Live Hub"}
            </button>
            {!replayMode ? (
              <button type="button" className="btn-ghost" onClick={handleEnterReplay}>
                Open Read-Only Evidence Replay
              </button>
            ) : (
              <button
                type="button"
                className="btn-ghost"
                onClick={() => {
                  setReplayMode(false);
                  setOpen(false);
                }}
              >
                Exit Replay Mode
              </button>
            )}
          </div>

          <div className="rounded-xl border border-white/[0.08] bg-white/[0.02] px-4 py-3 text-[11px] leading-relaxed text-ink-400">
            <span className="font-medium text-ink-200">Zero-fabrication guarantee:</span> Reconcilyx never generates synthetic live cluster metrics. When no live Hub is reachable, you can explicitly inspect immutable snapshots captured from verified Kind TLS run <span className="font-mono text-ink-200">#{EVIDENCE_RUN_ID}</span> and {EVIDENCE_VERIFIED_RUNS.verifiedDate} CI E2E (<span className="font-mono text-ink-200">#{EVIDENCE_VERIFIED_RUNS.ciE2ERunId}</span>), 2-cluster federation (<span className="font-mono text-ink-200">#{EVIDENCE_VERIFIED_RUNS.federationRunId}</span>), and GitOps PR #4 in read-only Evidence Replay Mode.
          </div>
        </form>
      </div>
    </div>
  );
}
