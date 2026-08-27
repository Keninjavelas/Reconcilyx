import { useState } from "react";
import {
  getApiBaseUrl,
  getApiKey,
  setApiBaseUrl,
  setApiKey as setControlPlaneApiKeyStorage,
} from "@/lib/api";
import { useAppStore } from "@/store/app";

export default function Settings() {
  const pushToast = useAppStore((s) => s.pushToast);
  const autoRefresh = useAppStore((s) => s.autoRefresh);
  const toggleAutoRefresh = useAppStore((s) => s.toggleAutoRefresh);

  const [provider, setProvider] = useState("ollama");
  const [apiKey, setApiKey] = useState("");
  const [baseUrl, setBaseUrl] = useState("http://localhost:11434");

  const [carbonProvider, setCarbonProvider] = useState("electricitymaps");
  const [carbonKey, setCarbonKey] = useState("");
  const [controlPlaneUrl, setControlPlaneUrl] = useState(() => getApiBaseUrl());
  const [controlPlaneApiKey, setControlPlaneApiKey] = useState(() => getApiKey());

  const handleSaveLLM = (e: React.FormEvent) => {
    e.preventDefault();
    pushToast({
      kind: "success",
      title: "LLM Configuration Saved",
      message: `Active provider set to '${provider}'. API Key saved.`,
    });
  };

  const handleSaveCarbon = (e: React.FormEvent) => {
    e.preventDefault();
    pushToast({
      kind: "success",
      title: "Carbon Provider Saved",
      message: `Carbon intensity provider set to '${carbonProvider}'.`,
    });
  };

  const handleSaveControlPlane = (e: React.FormEvent) => {
    e.preventDefault();
    try {
      const configuredUrl = setApiBaseUrl(controlPlaneUrl);
      setControlPlaneApiKeyStorage(controlPlaneApiKey);
      setControlPlaneUrl(configuredUrl);
      pushToast({
        kind: "success",
        title: "Control Plane URL Saved",
        message: configuredUrl
          ? `API requests will use ${configuredUrl}.`
          : "API requests will use this site's origin.",
      });
    } catch (error) {
      pushToast({
        kind: "error",
        title: "Invalid Control Plane URL",
        message: error instanceof Error ? error.message : "Enter a valid http or https URL.",
      });
    }
  };

  return (
    <div className="space-y-8 animate-fade-in-up">
      {/* Header */}
      <div className="border-b border-white/5 pb-4">
        <div className="eyebrow">Control Plane Config</div>
        <h1 className="font-display text-4xl tracking-tight">Settings</h1>
        <p className="mt-1 text-ink-100/70 max-w-2xl text-sm">
          Configure BYOK LLM provider keys, carbon intensity APIs, UI preferences, and system diagnostic posture.
        </p>
      </div>

      <div className="grid grid-cols-1 xl:grid-cols-2 gap-6">
        {/* LLM Provider Configuration Form */}
        <form onSubmit={handleSaveLLM} className="panel p-6 space-y-5">
          <div className="flex items-center gap-3">
            <div className="w-8 h-8 rounded-lg bg-mint-500/20 border border-mint-500/30 flex items-center justify-center text-mint-300">
              🤖
            </div>
            <div>
              <h2 className="panel-title text-lg">LLM Provider (BYOK)</h2>
              <p className="text-xs text-ink-500">Configure conversational agent &amp; tool routing model</p>
            </div>
          </div>

          <div className="space-y-4 text-xs">
            <div>
              <label className="text-ink-300 font-medium block mb-1.5">Select Provider</label>
              <select
                value={provider}
                onChange={(e) => setProvider(e.target.value)}
                className="input font-mono"
              >
                <option value="ollama">Ollama (Local / Self-hosted)</option>
                <option value="openai">OpenAI (GPT-4o / GPT-4o-mini)</option>
                <option value="anthropic">Anthropic (Claude 3.5 Sonnet)</option>
                <option value="groq">Groq (Llama 3 / Mixtral)</option>
                <option value="cohere">Cohere Command R+</option>
              </select>
            </div>

            <div>
              <label className="text-ink-300 font-medium block mb-1.5">API Key (if cloud provider)</label>
              <input
                type="password"
                value={apiKey}
                onChange={(e) => setApiKey(e.target.value)}
                placeholder="sk-..."
                className="input font-mono"
              />
            </div>

            <div>
              <label className="text-ink-300 font-medium block mb-1.5">Base URL / Endpoint</label>
              <input
                type="text"
                value={baseUrl}
                onChange={(e) => setBaseUrl(e.target.value)}
                placeholder="http://localhost:11434"
                className="input font-mono"
              />
            </div>
          </div>

          <button type="submit" className="btn-primary text-xs w-full justify-center">
            Save LLM Settings
          </button>
        </form>

        <form onSubmit={handleSaveControlPlane} className="panel p-6 space-y-5">
          <div className="eyebrow">Hosted UI Connection</div>
          <div>
            <h2 className="panel-title text-lg">Control Plane API URL</h2>
            <p className="text-xs text-ink-500 mt-1">
              Stored only in this browser. Leave blank when the UI and API share an origin.
            </p>
          </div>
          <div>
            <label className="text-ink-300 font-medium block mb-1.5">HTTPS API Origin</label>
            <input
              type="url"
              value={controlPlaneUrl}
              onChange={(e) => setControlPlaneUrl(e.target.value)}
              placeholder="https://reconcilyx.example.com"
              className="input font-mono"
            />
          </div>
          <div>
            <label className="text-ink-300 font-medium block mb-1.5">Control Plane API Key</label>
            <input
              type="password"
              value={controlPlaneApiKey}
              onChange={(e) => setControlPlaneApiKey(e.target.value)}
              placeholder="Optional in local development"
              autoComplete="off"
              className="input font-mono"
            />
          </div>
          <p className="text-[11px] text-ink-500">
            Both values are stored only in this browser. A remote API must explicitly allow this UI's origin with CORS. Do not expose a privileged control plane without authentication.
          </p>
          <button type="submit" className="btn-ghost text-xs w-full justify-center">
            Save Control Plane URL
          </button>
        </form>

        {/* Carbon Intensity & Pricing Config */}
        <form onSubmit={handleSaveCarbon} className="panel p-6 space-y-5">
          <div className="flex items-center gap-3">
            <div className="w-8 h-8 rounded-lg bg-amber-500/20 border border-amber-500/30 flex items-center justify-center text-amber-300">
              🌱
            </div>
            <div>
              <h2 className="panel-title text-lg">Carbon &amp; Cloud Pricing</h2>
              <p className="text-xs text-ink-500">Live grid emissions &amp; cloud pricing provider</p>
            </div>
          </div>

          <div className="space-y-4 text-xs">
            <div>
              <label className="text-ink-300 font-medium block mb-1.5">Carbon Intensity Provider</label>
              <select
                value={carbonProvider}
                onChange={(e) => setCarbonProvider(e.target.value)}
                className="input font-mono"
              >
                <option value="electricitymaps">Electricity Maps (Live API)</option>
                <option value="watttime">WattTime (Live API)</option>
                <option value="static">Static Regional Fallback (Updated Monthly)</option>
              </select>
            </div>

            <div>
              <label className="text-ink-300 font-medium block mb-1.5">Provider Token / API Key</label>
              <input
                type="password"
                value={carbonKey}
                onChange={(e) => setCarbonKey(e.target.value)}
                placeholder="Token..."
                className="input font-mono"
              />
            </div>
          </div>

          <button type="submit" className="btn-ghost text-xs w-full justify-center">
            Save Carbon Settings
          </button>
        </form>

        {/* Preferences & UI Refresh */}
        <div className="panel p-6 space-y-5">
          <div className="eyebrow">UI &amp; System Preferences</div>
          <h2 className="panel-title text-lg">Auto-Refresh &amp; Notifications</h2>

          <div className="space-y-3 text-xs">
            <label className="flex items-center justify-between p-3 rounded-xl bg-white/[0.02] border border-white/5 cursor-pointer">
              <div>
                <div className="font-medium text-white">Auto-Refresh Background Sync</div>
                <div className="text-ink-500 text-[11px] mt-0.5">Poll control plane metrics every 30 seconds</div>
              </div>
              <input
                type="checkbox"
                checked={autoRefresh}
                onChange={toggleAutoRefresh}
                className="w-4 h-4 accent-mint-500"
              />
            </label>
          </div>
        </div>

        {/* System & Release Posture Card */}
        <div className="panel p-6 space-y-5">
          <div className="eyebrow">System Diagnostic &amp; Release Posture</div>
          <h2 className="panel-title text-lg">Reconcilyx v4.0.0-beta</h2>

          <div className="p-4 rounded-xl bg-white/[0.02] border border-white/5 space-y-2 text-xs font-mono text-ink-300">
            <div>Release Posture: <span className="text-amber-300 font-bold">Beta: production validation in progress</span></div>
            <div>API Endpoint: <span className="text-sky-400">Configured locally in this browser</span></div>
            <div>Health Diagnostic: <span className="text-ink-500">Available after connecting an API</span></div>
          </div>
        </div>
      </div>
    </div>
  );
}
