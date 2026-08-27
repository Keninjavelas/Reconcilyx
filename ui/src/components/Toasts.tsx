import { useAppStore, type Toast as ToastT } from "@/store/app";
import clsx from "clsx";

const styles: Record<ToastT["kind"], { chip: string; ring: string; icon: string }> = {
  info:    { chip: "bg-sky-500/20 text-sky-400", ring: "border-sky-500/20", icon: "i" },
  success: { chip: "bg-mint-500/20 text-mint-400", ring: "border-mint-500/30", icon: "✓" },
  warn:    { chip: "bg-amber-500/20 text-amber-400", ring: "border-amber-500/20", icon: "!" },
  error:   { chip: "bg-ember-500/20 text-ember-400", ring: "border-ember-500/20", icon: "✕" },
};

export default function Toasts() {
  const toasts = useAppStore((s) => s.toasts);
  const removeToast = useAppStore((s) => s.removeToast);
  if (!toasts.length) return null;
  return (
    <div className="fixed bottom-6 right-6 z-50 flex flex-col gap-3 w-[min(360px,calc(100vw-3rem))]">
      {toasts.map((t) => {
        const s = styles[t.kind];
        return (
          <div
            key={t.id}
            className={clsx(
              "relative rounded-xl border backdrop-blur bg-ink-900/80 p-4 shadow-xl animate-fade-in-up",
              s.ring,
            )}
          >
            <div className="flex items-start gap-3">
              <div className={clsx("w-7 h-7 rounded-full grid place-items-center text-sm font-bold shrink-0", s.chip)}>
                {s.icon}
              </div>
              <div className="min-w-0 flex-1">
                <div className="font-semibold text-ink-100 text-sm">{t.title}</div>
                {t.message ? (
                  <div className="mt-0.5 text-sm text-ink-100/70 break-words">{t.message}</div>
                ) : null}
              </div>
              <button
                aria-label="Dismiss"
                className="text-ink-500 hover:text-ink-100"
                onClick={() => removeToast(t.id)}
              >
                ✕
              </button>
            </div>
          </div>
        );
      })}
    </div>
  );
}
