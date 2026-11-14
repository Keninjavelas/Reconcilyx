import { useEffect, useState } from "react";
import { Link, NavLink, Navigate, Route, Routes, useLocation, useNavigate } from "react-router-dom";
import Overview from "./pages/Overview";
import Proposals from "./pages/Proposals";
import ProposalDetail from "./pages/ProposalDetail";
import Forecast from "./pages/Forecast";
import Savings from "./pages/Savings";
import Policies from "./pages/Policies";
import Settings from "./pages/Settings";
import clsx from "clsx";
import Toasts from "./components/Toasts";
import AiChatDrawer from "./components/AiChatDrawer";
import KeyboardShortcutsModal from "./components/KeyboardShortcutsModal";
import HubConnectionModal from "./components/HubConnectionModal";
import { EVIDENCE_BANNER_TEXT } from "./lib/evidenceReplay";
import Icon, { type IconName } from "./components/Icon";
import { useAppStore } from "./store/app";
import { getApiBaseUrl, api } from "./lib/api";
import { useApi } from "./lib/useApi";

interface NavItem {
  label: string;
  to: string;
  end?: boolean;
  icon: IconName;
}

const navItems: NavItem[] = [
  { label: "Overview", to: "/", end: true, icon: "overview" },
  { label: "Proposals", to: "/proposals", icon: "proposals" },
  { label: "Forecast", to: "/forecast", icon: "forecast" },
  { label: "Savings", to: "/savings", icon: "savings" },
  { label: "Policies", to: "/policies", icon: "policies" },
  { label: "Settings", to: "/settings", icon: "settings" },
];

export default function App() {
  const [chatOpen, setChatOpen] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const [apiBaseUrl, setConfiguredApiBaseUrl] = useState(() => getApiBaseUrl());
  const navigate = useNavigate();
  const location = useLocation();

  const setShortcutsOpen = useAppStore((s) => s.setShortcutsOpen);
  const setHubGateOpen = useAppStore((s) => s.setHubGateOpen);
  const replayMode = useAppStore((s) => s.replayMode);
  const setReplayMode = useAppStore((s) => s.setReplayMode);
  const autoRefresh = useAppStore((s) => s.autoRefresh);
  const refresh = useAppStore((s) => s.refresh);

  const { data: health, error: healthError, fetchedAt } = useApi(api.healthz.get);
  const apiOnline = !replayMode && health?.status === "ok";
  const healthLabel = replayMode
    ? "Replay snapshot"
    : apiOnline
      ? "API online"
      : healthError
        ? "API unreachable"
        : "Checking API";

  useEffect(() => {
    const updateApiBaseUrl = () => setConfiguredApiBaseUrl(getApiBaseUrl());
    window.addEventListener("reconcilyx-api-url-changed", updateApiBaseUrl);
    return () => window.removeEventListener("reconcilyx-api-url-changed", updateApiBaseUrl);
  }, []);

  useEffect(() => {
    setMenuOpen(false);
  }, [location.pathname]);

  useEffect(() => {
    if (!autoRefresh) return;
    const timer = window.setInterval(() => refresh(), 30000);
    return () => window.clearInterval(timer);
  }, [autoRefresh, refresh]);

  useEffect(() => {
    let pendingG = false;

    const handleKeyDown = (event: KeyboardEvent) => {
      if (
        event.target instanceof HTMLInputElement ||
        event.target instanceof HTMLTextAreaElement ||
        event.target instanceof HTMLSelectElement
      ) {
        return;
      }

      if (event.key === "Escape") {
        setChatOpen(false);
        setShortcutsOpen(false);
        setHubGateOpen(false);
        setMenuOpen(false);
        pendingG = false;
        return;
      }

      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setChatOpen((prev) => !prev);
        return;
      }

      if (event.key === "?") {
        event.preventDefault();
        setShortcutsOpen(true);
        return;
      }

      if (event.key.toLowerCase() === "g") {
        pendingG = true;
        window.setTimeout(() => {
          pendingG = false;
        }, 1000);
        return;
      }

      if (!pendingG) return;
      pendingG = false;
      switch (event.key.toLowerCase()) {
        case "d":
          navigate("/");
          break;
        case "p":
          navigate("/proposals");
          break;
        case "f":
          navigate("/forecast");
          break;
        case "s":
          navigate("/savings");
          break;
        case "o":
          navigate("/policies");
          break;
        case "x":
          navigate("/settings");
          break;
        case "c":
          setChatOpen(true);
          break;
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [navigate, setShortcutsOpen, setHubGateOpen]);

  return (
    <div className="relative min-h-screen">
      <div className="bg-ambient" aria-hidden />
      <div className="bg-grid" aria-hidden />

      <header className="sticky top-0 z-40 border-b border-white/[0.09] bg-ink-950/85 backdrop-blur-xl">
        <div className="mx-auto flex h-16 max-w-[1440px] items-center justify-between gap-4 px-5 lg:px-10">
          <Link to="/" className="group flex items-center gap-3" aria-label="Reconcilyx overview">
            <span className="grid h-8 w-8 place-items-center rounded-[7px] bg-mint-500 font-display text-[15px] font-semibold leading-none text-ink-950 transition group-hover:bg-mint-400">
              R
            </span>
            <span className="leading-none">
              <span className="block font-display text-[14px] font-medium tracking-[0.18em] text-ink-50">
                RECONCILYX
              </span>
              <span className="mt-1.5 block font-mono text-[8px] uppercase tracking-[0.24em] text-ink-500">
                Operator console
              </span>
            </span>
          </Link>

          <nav className="hidden items-center lg:flex" aria-label="Primary">
            {navItems.map((item, index) => (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.end}
                className={({ isActive }) =>
                  clsx(
                    "group relative flex items-center gap-1.5 px-3 py-5 text-[12px] transition",
                    isActive ? "text-ink-50" : "text-ink-400 hover:text-ink-100",
                  )
                }
              >
                {({ isActive }) => (
                  <>
                    <span
                      className={clsx(
                        "font-mono text-[9px] tracking-[0.08em]",
                        isActive ? "text-mint-500" : "text-ink-600",
                      )}
                    >
                      {String(index + 1).padStart(2, "0")}
                    </span>
                    {item.label}
                    <span
                      className={clsx(
                        "absolute bottom-0 left-3 right-3 h-px transition",
                        isActive ? "bg-mint-500" : "bg-transparent group-hover:bg-white/20",
                      )}
                      aria-hidden
                    />
                  </>
                )}
              </NavLink>
            ))}
          </nav>

          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => setHubGateOpen(true)}
              className="hidden items-center gap-2 rounded-full border border-white/[0.12] px-3 py-1.5 font-mono text-[9px] uppercase tracking-[0.14em] text-ink-300 transition hover:border-mint-500/40 hover:text-ink-100 md:inline-flex"
              title={
                replayMode
                  ? EVIDENCE_BANNER_TEXT
                  : fetchedAt
                    ? `Last health check ${new Date(fetchedAt).toLocaleTimeString()} — click to configure Hub`
                    : "Click to configure Control Plane connection"
              }
            >
              <span
                className={clsx(
                  "status-dot",
                  replayMode
                    ? "bg-amber-400 text-amber-400"
                    : apiOnline
                      ? "bg-mint-500 text-mint-500"
                      : healthError
                        ? "bg-ember-500 text-ember-500"
                        : "bg-amber-500 text-amber-500",
                )}
                aria-hidden
              />
              {healthLabel}
            </button>
            <button
              onClick={() => setChatOpen(true)}
              className="btn-ghost hidden h-9 gap-2 px-3.5 text-[11px] sm:inline-flex"
              title="Ask the control plane (Ctrl+K)"
            >
              <Icon name="ask" size={14} />
              Ask
              <kbd className="ml-1 rounded border border-white/[0.12] px-1.5 py-0.5 font-mono text-[9px] text-ink-500">
                Ctrl K
              </kbd>
            </button>
            <button
              className="btn-icon lg:hidden"
              onClick={() => setMenuOpen((open) => !open)}
              aria-expanded={menuOpen}
              aria-label="Toggle navigation"
            >
              <Icon name={menuOpen ? "close" : "menu"} size={18} />
            </button>
          </div>
        </div>

        {menuOpen ? (
          <div className="border-t border-white/[0.08] bg-ink-950 px-5 py-4 lg:hidden">
            <nav className="grid gap-1" aria-label="Mobile">
              {navItems.map((item, index) => (
                <NavLink
                  key={item.to}
                  to={item.to}
                  end={item.end}
                  className={({ isActive }) =>
                    clsx(
                      "flex items-center justify-between rounded-lg px-3 py-3 text-[13px] transition",
                      isActive ? "bg-white/[0.05] text-ink-50" : "text-ink-300 hover:bg-white/[0.03]",
                    )
                  }
                >
                  <span className="flex items-center gap-3">
                    <Icon name={item.icon} size={15} />
                    {item.label}
                  </span>
                  <span className="font-mono text-[9px] text-ink-600">
                    {String(index + 1).padStart(2, "0")}
                  </span>
                </NavLink>
              ))}
            </nav>
            <div className="mt-3 flex items-center justify-between border-t border-white/[0.08] pt-3">
              <span className="flex items-center gap-2 font-mono text-[9px] uppercase tracking-[0.14em] text-ink-400">
                <span
                  className={clsx(
                    "status-dot",
                    apiOnline ? "bg-mint-500 text-mint-500" : healthError ? "bg-ember-500 text-ember-500" : "bg-amber-500 text-amber-500",
                  )}
                  aria-hidden
                />
                {healthLabel}
              </span>
              <button className="btn-ghost h-9 px-3 text-[11px]" onClick={() => setChatOpen(true)}>
                Ask the control plane
              </button>
            </div>
          </div>
        ) : null}
      </header>

      {replayMode ? (
        <div
          role="status"
          className="border-b border-amber-500/35 bg-amber-500/[0.08] px-5 py-2.5 text-[11px] text-amber-200 lg:px-10"
        >
          <div className="mx-auto flex max-w-[1440px] flex-wrap items-center justify-between gap-3">
            <div className="flex items-center gap-2 font-mono tracking-[0.04em]">
              <span className="status-dot bg-amber-400 text-amber-400" aria-hidden />
              <span>{EVIDENCE_BANNER_TEXT}</span>
            </div>
            <div className="flex items-center gap-2">
              <button
                type="button"
                className="rounded-md border border-amber-400/40 px-2.5 py-1 font-mono text-[10px] uppercase tracking-[0.1em] text-amber-100 hover:bg-amber-400/15"
                onClick={() => setHubGateOpen(true)}
              >
                Connect live Hub
              </button>
              <button
                type="button"
                className="rounded-md border border-white/15 px-2.5 py-1 font-mono text-[10px] uppercase tracking-[0.1em] text-ink-300 hover:text-ink-100"
                onClick={() => setReplayMode(false)}
              >
                Exit replay
              </button>
            </div>
          </div>
        </div>
      ) : !apiOnline && healthError ? (
        <div
          role="alert"
          className="border-b border-ember-500/35 bg-ember-500/[0.08] px-5 py-3 text-[12px] text-ember-200 lg:px-10"
        >
          <div className="mx-auto flex max-w-[1440px] flex-wrap items-center justify-between gap-3">
            <div>
              <span className="font-medium text-ember-100">Control plane unreachable: </span>
              <span className="text-ink-300">
                No telemetry is fabricated. Connect a live Reconcilyx Hub API or switch to read-only verified evidence replay.
              </span>
            </div>
            <div className="flex items-center gap-2">
              <button
                type="button"
                className="btn-primary h-8 px-3 text-[11px]"
                onClick={() => setHubGateOpen(true)}
              >
                Configure Hub connection
              </button>
              <button
                type="button"
                className="btn-ghost h-8 px-3 text-[11px]"
                onClick={() => setReplayMode(true)}
              >
                Inspect Run #20261026055631 Replay
              </button>
            </div>
          </div>
        </div>
      ) : null}

      <main className="mx-auto w-full max-w-[1440px] px-5 pb-20 pt-9 lg:px-10 lg:pt-14">
        <Routes>
          <Route path="/" element={<Overview />} />
          <Route path="/proposals" element={<Proposals />} />
          <Route path="/proposals/:name" element={<ProposalDetail />} />
          <Route path="/forecast" element={<Forecast />} />
          <Route path="/savings" element={<Savings />} />
          <Route path="/policies" element={<Policies />} />
          <Route path="/settings" element={<Settings />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </main>

      <footer className="border-t border-white/[0.08]">
        <div className="mx-auto grid max-w-[1440px] gap-8 px-5 py-10 md:grid-cols-[1.4fr_1fr_1fr] lg:px-10">
          <div>
            <div className="font-display text-[13px] font-medium tracking-[0.18em] text-ink-100">
              RECONCILYX
            </div>
            <p className="mt-3 max-w-[38ch] text-[12px] leading-relaxed text-ink-500">
              Cost and carbon optimization for Kubernetes, applied through proposal-gated in-place pod
              resizing.
            </p>
            <div className="mt-4 font-mono text-[9px] uppercase tracking-[0.16em] text-ink-600">
              Apache-2.0 · v4.1.0-rc1
            </div>
          </div>
          <div>
            <div className="eyebrow">Control plane</div>
            <div className="mt-3 space-y-2 font-mono text-[10px] text-ink-400">
              <div className="break-all">{apiBaseUrl || `${window.location.origin} (same origin)`}</div>
              <div>{healthLabel}</div>
              <div className="text-ink-600">
                {health?.time ? `Reported ${new Date(health.time).toLocaleString()}` : "No health report yet"}
              </div>
            </div>
          </div>
          <div>
            <div className="eyebrow">Reference</div>
            <div className="mt-3 flex flex-col items-start gap-2 text-[12px] text-ink-400">
              <button className="transition hover:text-mint-500" onClick={() => setShortcutsOpen(true)}>
                Keyboard shortcuts
              </button>
              <a
                className="transition hover:text-mint-500"
                href="https://github.com/Keninjavelas/Reconcilyx"
                target="_blank"
                rel="noreferrer"
              >
                Source repository ↗
              </a>
              <Link className="transition hover:text-mint-500" to="/settings">
                Connection settings
              </Link>
            </div>
          </div>
        </div>
      </footer>

      <Toasts />
      <AiChatDrawer isOpen={chatOpen} onClose={() => setChatOpen(false)} />
      <KeyboardShortcutsModal />
      <HubConnectionModal />
    </div>
  );
}
