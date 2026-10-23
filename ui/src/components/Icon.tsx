import clsx from "clsx";
import type { ReactNode } from "react";

export type IconName =
  | "overview"
  | "proposals"
  | "forecast"
  | "savings"
  | "policies"
  | "settings"
  | "ask"
  | "arrow-up-right"
  | "arrow-right"
  | "arrow-left"
  | "close"
  | "menu"
  | "check"
  | "alert"
  | "search"
  | "plus"
  | "refresh"
  | "copy"
  | "external"
  | "cpu"
  | "memory"
  | "leaf"
  | "dollar"
  | "target"
  | "clock"
  | "chevron-down"
  | "command"
  | "lock"
  | "branch"
  | "terminal";

const paths: Record<IconName, ReactNode> = {
  overview: (
    <>
      <rect x="3" y="3" width="7" height="7" rx="1.2" />
      <rect x="14" y="3" width="7" height="7" rx="1.2" />
      <rect x="3" y="14" width="7" height="7" rx="1.2" />
      <rect x="14" y="14" width="7" height="7" rx="1.2" />
    </>
  ),
  proposals: (
    <>
      <path d="M4 6h16M4 12h16M4 18h10" />
      <circle cx="19" cy="18" r="2" />
    </>
  ),
  forecast: (
    <>
      <path d="M3 17l5-6 4 3 5-8 4 4" />
      <path d="M3 21h18" />
    </>
  ),
  savings: (
    <>
      <circle cx="12" cy="12" r="8.5" />
      <path d="M15 9.2c-.6-.8-1.7-1.2-3-1.2-1.7 0-3 .9-3 2.2 0 3.5 6 1.8 6 5 0 1.4-1.4 2.3-3.1 2.3-1.4 0-2.6-.5-3.2-1.4M12 6.4v11.2" />
    </>
  ),
  policies: (
    <>
      <path d="M12 3l7 3v5.2c0 4.4-2.9 8-7 9.8-4.1-1.8-7-5.4-7-9.8V6l7-3z" />
      <path d="M9.2 12.2l1.9 1.9 3.9-4" />
    </>
  ),
  settings: (
    <>
      <path d="M4 7h10M18 7h2M4 17h2M10 17h10" />
      <circle cx="16" cy="7" r="2" />
      <circle cx="8" cy="17" r="2" />
    </>
  ),
  ask: (
    <>
      <path d="M12 3l1.7 5.1L19 10l-5.3 1.9L12 17l-1.7-5.1L5 10l5.3-1.9L12 3z" />
      <path d="M18.5 15.5l.8 2.2 2.2.8-2.2.8-.8 2.2-.8-2.2-2.2-.8 2.2-.8.8-2.2z" />
    </>
  ),
  "arrow-up-right": (
    <>
      <path d="M7 17L17 7M9 7h8v8" />
    </>
  ),
  "arrow-right": (
    <>
      <path d="M4 12h16M14 6l6 6-6 6" />
    </>
  ),
  "arrow-left": (
    <>
      <path d="M20 12H4M10 6l-6 6 6 6" />
    </>
  ),
  close: <path d="M6 6l12 12M18 6L6 18" />,
  menu: <path d="M3 7h18M3 12h18M3 17h18" />,
  check: <path d="M4.5 12.5l5 5 10-11" />,
  alert: (
    <>
      <path d="M12 3l9.5 17H2.5L12 3z" />
      <path d="M12 9v5M12 17.2v.1" />
    </>
  ),
  search: (
    <>
      <circle cx="11" cy="11" r="7" />
      <path d="M20 20l-4-4" />
    </>
  ),
  plus: <path d="M12 4v16M4 12h16" />,
  refresh: (
    <>
      <path d="M20 11a8 8 0 10-2.3 6.3" />
      <path d="M20 5v6h-6" />
    </>
  ),
  copy: (
    <>
      <rect x="9" y="9" width="12" height="12" rx="2" />
      <path d="M15 9V5a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2h4" />
    </>
  ),
  external: (
    <>
      <path d="M14 4h6v6M20 4l-9 9" />
      <path d="M18 14v5a1 1 0 01-1 1H5a1 1 0 01-1-1V7a1 1 0 011-1h5" />
    </>
  ),
  cpu: (
    <>
      <rect x="6" y="6" width="12" height="12" rx="2" />
      <rect x="10" y="10" width="4" height="4" rx="0.6" />
      <path d="M9 2v4M15 2v4M9 18v4M15 18v4M2 9h4M2 15h4M18 9h4M18 15h4" />
    </>
  ),
  memory: (
    <>
      <rect x="3" y="7" width="18" height="10" rx="2" />
      <path d="M7 11v2M11 11v2M15 11v2M19 11v2M7 4v3M17 4v3M7 17v3M17 17v3" />
    </>
  ),
  leaf: (
    <>
      <path d="M20 4C10 4 5 8.5 5 15c0 2.5 1 4.5 2 5 2-8 7-10 13-10" />
      <path d="M4 21c1-5 4-8 8-10" />
    </>
  ),
  dollar: (
    <>
      <path d="M12 2v20M17 6.5c0-1.9-2.2-3-5-3s-5 1.1-5 3 1.7 2.7 5 3.4c3.3.7 5 1.5 5 3.6s-2.2 3.5-5 3.5-5-1.3-5-3.2" />
    </>
  ),
  target: (
    <>
      <circle cx="12" cy="12" r="8.5" />
      <circle cx="12" cy="12" r="4.5" />
      <circle cx="12" cy="12" r="1" />
    </>
  ),
  clock: (
    <>
      <circle cx="12" cy="12" r="8.5" />
      <path d="M12 7v5l3.5 2" />
    </>
  ),
  "chevron-down": <path d="M6 9l6 6 6-6" />,
  command: (
    <>
      <path d="M9 6a3 3 0 10-3 3h12a3 3 0 10-3-3v12a3 3 0 103-3H6a3 3 0 103 3V6z" />
    </>
  ),
  lock: (
    <>
      <rect x="5" y="10" width="14" height="11" rx="2" />
      <path d="M8 10V7a4 4 0 018 0v3" />
    </>
  ),
  branch: (
    <>
      <circle cx="6" cy="5" r="2.2" />
      <circle cx="18" cy="7" r="2.2" />
      <circle cx="6" cy="19" r="2.2" />
      <path d="M6 7.2v9.6M8.2 5H13a5 5 0 015 5v-1" />
    </>
  ),
  terminal: (
    <>
      <rect x="3" y="4" width="18" height="16" rx="2" />
      <path d="M7 9l3 3-3 3M13 15h4" />
    </>
  ),
};

export default function Icon({
  name,
  size = 16,
  className,
  strokeWidth = 1.6,
}: {
  name: IconName;
  size?: number;
  className?: string;
  strokeWidth?: number;
}) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={strokeWidth}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      className={clsx("shrink-0", className)}
    >
      {paths[name]}
    </svg>
  );
}
