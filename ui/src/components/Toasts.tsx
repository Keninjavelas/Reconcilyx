import { useAppStore, type Toast as ToastT } from "@/store/app";
import clsx from "clsx";

const styles: Record<ToastT["kind"], { rule: string; label: string }> = {
  info: { rule: "bg-sky-500", label: "Info" },
  success: { rule: "bg-mint-500", label: "Done" },
  warn: { rule: "bg-amber-500", label: "Notice" },
  error: { rule: "bg-ember-500", label: "Error" },
};

export default function Toasts() {
  const toasts = useAppStore((s) => s.toasts);
  const removeToast = useAppStore((s) => s.removeToast);
  if (!toasts.length) return null;

  return (
    <div className="fixed bottom-5 right-5 z-[60] flex w-[min(380px,calc(100vw-2.5rem))] flex-col gap-2">
      {toasts.map((toast) => {
        const tone = styles[toast.kind];
        return (
          <div
            key={toast.id}
            className="relative flex items-start gap-3 overflow-hidden rounded-xl border border-white/[0.12] bg-ink-900/95 px-4 py-3.5 shadow-lift backdrop-blur animate-fade-in-up"
            role="status"
          >
            <span className={clsx("absolute inset-y-0 left-0 w-[2px]", tone.rule)} aria-hidden />
            <div className="min-w-0 flex-1 pl-1">
              <div className="flex items-baseline gap-2">
                <span className="font-mono text-[9px] uppercase tracking-[0.18em] text-ink-500">
                  {tone.label}
                </span>
                <span className="text-[13px] font-medium text-ink-50">{toast.title}</span>
              </div>
              {toast.message ? (
                <div className="mt-1 break-words text-[12px] leading-relaxed text-ink-400">
                  {toast.message}
                </div>
              ) : null}
            </div>
            <button
              aria-label="Dismiss notification"
              className="p-1 text-ink-500 transition hover:text-ink-100"
              onClick={() => removeToast(toast.id)}
            >
              <span aria-hidden>×</span>
            </button>
          </div>
        );
      })}
    </div>
  );
}
