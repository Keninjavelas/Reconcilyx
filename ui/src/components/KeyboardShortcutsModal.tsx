import { useAppStore } from "@/store/app";

const SHORTCUTS = [
  { key: "g d", desc: "Overview" },
  { key: "g p", desc: "Proposals" },
  { key: "g f", desc: "Forecast" },
  { key: "g s", desc: "Savings" },
  { key: "g o", desc: "Policies" },
  { key: "g x", desc: "Settings" },
  { key: "⌘ K", desc: "Open the assistant" },
  { key: "?", desc: "Show this reference" },
  { key: "Esc", desc: "Dismiss overlays" },
];

export default function KeyboardShortcutsModal() {
  const open = useAppStore((s) => s.shortcutsOpen);
  const setOpen = useAppStore((s) => s.setShortcutsOpen);

  if (!open) return null;

  return (
    <div
      className="overlay fixed inset-0 z-[60] flex items-center justify-center p-4"
      onClick={() => setOpen(false)}
      role="presentation"
    >
      <div
        className="panel w-full max-w-md animate-fade-in-up"
        onClick={(event) => event.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-label="Keyboard shortcuts"
      >
        <div className="panel-header">
          <div>
            <div className="eyebrow">Reference</div>
            <h2 className="panel-title mt-1">Keyboard shortcuts</h2>
          </div>
          <button className="btn-icon" onClick={() => setOpen(false)} aria-label="Close shortcuts">
            <span aria-hidden>×</span>
          </button>
        </div>
        <div className="px-5 py-2 sm:px-6">
          {SHORTCUTS.map((shortcut) => (
            <div
              key={shortcut.key}
              className="flex items-center justify-between border-b border-white/[0.055] py-3 last:border-0"
            >
              <span className="text-[13px] text-ink-300">{shortcut.desc}</span>
              <kbd className="rounded border border-white/[0.12] bg-white/[0.04] px-2 py-1 font-mono text-[10px] uppercase tracking-[0.1em] text-ink-100">
                {shortcut.key}
              </kbd>
            </div>
          ))}
        </div>
        <div className="border-t border-white/[0.08] px-5 py-3 text-[11px] text-ink-500 sm:px-6">
          Shortcuts are inactive while typing in a field.
        </div>
      </div>
    </div>
  );
}
