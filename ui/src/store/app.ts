import { create } from "zustand";

type ToastKind = "info" | "success" | "error" | "warn";

export interface Toast {
  id: string;
  kind: ToastKind;
  title: string;
  message?: string;
}

interface AppState {
  cluster: string;
  namespaceFilter?: string;
  refreshEpoch: number;
  toasts: Toast[];
  selectedProposals: string[];
  autoRefresh: boolean;
  shortcutsOpen: boolean;
  monthlySavingsGoalUSD: number;

  setCluster: (c: string) => void;
  setNamespaceFilter: (n?: string) => void;
  refresh: () => void;
  pushToast: (t: Omit<Toast, "id">) => void;
  removeToast: (id: string) => void;
  toggleProposalSelection: (name: string) => void;
  selectAllProposals: (names: string[]) => void;
  clearSelectedProposals: () => void;
  toggleAutoRefresh: () => void;
  setShortcutsOpen: (open: boolean) => void;
}

const uid = () => Math.random().toString(36).slice(2, 9);

export const useAppStore = create<AppState>((set) => ({
  cluster: "hub",
  namespaceFilter: undefined,
  refreshEpoch: 0,
  toasts: [],
  selectedProposals: [],
  autoRefresh: true,
  shortcutsOpen: false,
  monthlySavingsGoalUSD: 5000,
  setCluster: (c) => set({ cluster: c }),
  setNamespaceFilter: (n) => set({ namespaceFilter: n }),
  refresh: () => set((s) => ({ refreshEpoch: s.refreshEpoch + 1 })),
  pushToast: (t) => {
    const id = uid();
    const toast: Toast = { id, ...t };
    set((s) => ({ toasts: [...s.toasts, toast] }));
    window.setTimeout(() => {
      set((s) => ({ toasts: s.toasts.filter((x) => x.id !== id) }));
    }, 5000);
  },
  removeToast: (id) => set((s) => ({ toasts: s.toasts.filter((x) => x.id !== id) })),
  toggleProposalSelection: (name) =>
    set((s) => ({
      selectedProposals: s.selectedProposals.includes(name)
        ? s.selectedProposals.filter((x) => x !== name)
        : [...s.selectedProposals, name],
    })),
  selectAllProposals: (names) => set({ selectedProposals: names }),
  clearSelectedProposals: () => set({ selectedProposals: [] }),
  toggleAutoRefresh: () => set((s) => ({ autoRefresh: !s.autoRefresh })),
  setShortcutsOpen: (open) => set({ shortcutsOpen: open }),
}));

export function formatCurrency(n: number, digits = 2): string {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    maximumFractionDigits: digits,
  }).format(n);
}

export function formatCarbonGrams(g?: number): string {
  if (!g) return "0 g CO₂";
  const kg = 1000;
  const tonne = 1_000_000;
  if (g >= tonne) return `${(g / tonne).toFixed(2)} tonnes CO₂`;
  if (g >= kg) return `${(g / kg).toFixed(1)} kg CO₂`;
  return `${Math.round(g)} g CO₂`;
}
