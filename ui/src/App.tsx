import { useState, useEffect } from "react";
import { NavLink, Navigate, Route, Routes, useNavigate } from "react-router-dom";
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
import { useAppStore } from "./store/app";
import { getApiBaseUrl } from "./lib/api";

const navItems: { label: string; to: string; end?: boolean; badge?: string }[] = [
  { label: "Overview", to: "/", end: true },
  { label: "Proposals", to: "/proposals" },
  { label: "Forecast", to: "/forecast", badge: "What-If" },
  { label: "Savings", to: "/savings", badge: "USD · CO₂" },
  { label: "Policies", to: "/policies" },
  { label: "Settings", to: "/settings" },
];

export default function App() {
  const [chatOpen, setChatOpen] = useState(false);
  const [apiBaseUrl, setConfiguredApiBaseUrl] = useState(() => getApiBaseUrl());
  const navigate = useNavigate();

  const setShortcutsOpen = useAppStore((s) => s.setShortcutsOpen);
  const autoRefresh = useAppStore((s) => s.autoRefresh);
  const refresh = useAppStore((s) => s.refresh);

  useEffect(() => {
    const updateApiBaseUrl = () => setConfiguredApiBaseUrl(getApiBaseUrl());
    window.addEventListener("reconcilyx-api-url-changed", updateApiBaseUrl);
    return () => window.removeEventListener("reconcilyx-api-url-changed", updateApiBaseUrl);
  }, []);

  // Background auto-refresh polling (30s)
  useEffect(() => {
    if (!autoRefresh) return;
    const timer = setInterval(() => {
      refresh();
    }, 30000);
    return () => clearInterval(timer);
  }, [autoRefresh, refresh]);

  // Global Keyboard Shortcuts
  useEffect(() => {
    let pendingG = false;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (
        e.target instanceof HTMLInputElement ||
        e.target instanceof HTMLTextAreaElement ||
        e.target instanceof HTMLSelectElement
      ) {
        return;
      }

      if (e.key === "Escape") {
        setChatOpen(false);
        setShortcutsOpen(false);
        pendingG = false;
        return;
      }

      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setChatOpen((prev) => !prev);
        return;
      }

      if (e.key === "?") {
        e.preventDefault();
        setShortcutsOpen(true);
        return;
      }

      if (e.key.toLowerCase() === "g") {
        pendingG = true;
        setTimeout(() => {
          pendingG = false;
        }, 1000);
        return;
      }

      if (pendingG) {
        pendingG = false;
        switch (e.key.toLowerCase()) {
          case "d":
            navigate("/");
            break;
          case "p":
            navigate("/proposals");
            break;
          case "f":
            navigate("/forecast");
            break;
          case "o":
            navigate("/policies");
            break;
          case "s":
            navigate("/settings");
            break;
          case "c":
            setChatOpen(true);
            break;
        }
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [navigate, setShortcutsOpen]);

  return (
    <div className="min-h-screen relative">
      <div className="fixed inset-0 -z-10 pointer-events-none bg-grain" aria-hidden />
      <header className="sticky top-0 z-30 border-b border-white/5 bg-ink-950/70 backdrop-blur">
        <div className="mx-auto max-w-7xl px-6 py-4 flex items-center justify-between">
          <div className="flex items-center gap-3 cursor-pointer" onClick={() => navigate("/")}>
            <div className="w-9 h-9 rounded-xl bg-gradient-to-br from-mint-400 via-mint-500 to-sky-500 shadow-glow flex items-center justify-center text-ink-950 font-display text-xl font-bold">
              R
            </div>
            <div>
              <div className="font-display text-xl tracking-tight text-white">Reconcilyx</div>
              <div className="text-xs text-ink-500 -mt-0.5">
                Cost + Carbon Intelligence Fabric for Kubernetes
              </div>
            </div>
          </div>
          <nav className="hidden md:flex items-center gap-1">
            {navItems.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.end}
                className={({ isActive }) =>
                  clsx(
                    "relative px-3.5 py-2 text-sm font-medium rounded-lg transition-all",
                    isActive
                      ? "text-mint-400 bg-white/[0.04]"
                      : "text-ink-100/80 hover:text-ink-100 hover:bg-white/[0.03]",
                  )
                }
              >
                <span className="inline-flex items-center gap-1.5">
                  {item.label}
                  {item.badge && (
                    <span className="text-[9px] uppercase tracking-widest text-ink-500 border border-white/10 px-1 py-0.5 rounded font-mono">
                      {item.badge}
                    </span>
                  )}
                </span>
              </NavLink>
            ))}
          </nav>
          <div className="flex items-center gap-3">
            <button
              onClick={() => setChatOpen(true)}
              className="inline-flex items-center gap-2 rounded-lg border border-mint-500/30 bg-mint-500/10 px-3 py-1.5 text-xs text-mint-300 hover:bg-mint-500/20 transition font-medium"
              title="AI Assistant (Ctrl+K)"
            >
              🤖 AI Assistant
            </button>
            <span className="hidden sm:inline-flex items-center gap-2 rounded-full border border-white/10 bg-white/[0.02] px-3 py-1.5 text-xs text-ink-100/80">
              <span className="w-1.5 h-1.5 rounded-full bg-mint-400 animate-pulse" aria-hidden />
              {apiBaseUrl ? "Remote API configured" : "Same-origin API"}
            </span>
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-7xl px-6 py-8">
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

      <footer className="mx-auto max-w-7xl px-6 py-10 text-xs text-ink-500 flex flex-wrap gap-4 justify-between border-t border-white/5">
        <div>© Reconcilyx · Apache 2.0 · v4.0.0-beta</div>
        <div className="flex flex-wrap gap-4">
          <a className="hover:text-ink-100" href={`${apiBaseUrl}/healthz`} target="_blank" rel="noreferrer">
            /healthz
          </a>
          <a className="hover:text-ink-100" href={`${apiBaseUrl}/metrics`} target="_blank" rel="noreferrer">
            /metrics
          </a>
          <a className="hover:text-ink-100" href="https://github.com/Keninjavelas/Reconcilyx" target="_blank" rel="noreferrer">
            GitHub
          </a>
        </div>
      </footer>

      <Toasts />
      <AiChatDrawer isOpen={chatOpen} onClose={() => setChatOpen(false)} />
      <KeyboardShortcutsModal />
    </div>
  );
}
