import { useAppStore } from "@/store/app";

const SHORTCUTS = [
  { key: "g d", desc: "Go to Overview Dashboard" },
  { key: "g p", desc: "Go to Proposals Inventory" },
  { key: "g f", desc: "Go to Forecast & What-If" },
  { key: "g o", desc: "Go to Policies" },
  { key: "g s", desc: "Go to Settings" },
  { key: "g c  /  Ctrl+K", desc: "Open AI Assistant Drawer" },
  { key: "?", desc: "Show Keyboard Shortcuts Help" },
  { key: "Esc", desc: "Close Modals / Clear Selection" },
];

export default function KeyboardShortcutsModal() {
  const open = useAppStore((s) => s.shortcutsOpen);
  const setOpen = useAppStore((s) => s.setShortcutsOpen);

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 overflow-y-auto bg-black/60 backdrop-blur-sm flex items-center justify-center p-4 animate-fade-in"
      onClick={() => setOpen(false)}
    >
      <div
        className="w-full max-w-md bg-ink-950 border border-white/10 rounded-2xl p-6 shadow-2xl space-y-6"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-white/10 pb-4">
          <div className="flex items-center gap-2">
            <span className="text-lg">⌨️</span>
            <h3 className="font-display text-lg text-white">Keyboard Shortcuts</h3>
          </div>
          <button
            onClick={() => setOpen(false)}
            className="text-ink-400 hover:text-white p-1 rounded-lg hover:bg-white/5"
          >
            ✕
          </button>
        </div>

        <div className="space-y-2">
          {SHORTCUTS.map((s) => (
            <div
              key={s.key}
              className="flex items-center justify-between py-2 border-b border-white/5 text-sm"
            >
              <span className="text-ink-300">{s.desc}</span>
              <kbd className="px-2.5 py-1 rounded bg-white/10 border border-white/10 text-mint-400 font-mono text-xs font-semibold">
                {s.key}
              </kbd>
            </div>
          ))}
        </div>

        <div className="text-xs text-ink-500 text-center">
          Press <kbd className="px-1.5 py-0.5 rounded bg-white/5 border border-white/10">Esc</kbd> or click anywhere outside to close.
        </div>
      </div>
    </div>
  );
}
