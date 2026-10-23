# Reconcilyx Dashboard (ui/)

Operator console for the Reconcilyx control plane. Vite + React 18 + TypeScript + Tailwind + Zustand + Recharts, with a dark editorial design system.

## Quick start

```bash
cd ui
npm ci

# local dev (proxies /api and /healthz to http://localhost:8082)
npm run dev

# typecheck only
npm run typecheck

# production build -> ui/dist, embedded by ../ui/embed.go
npm run build
```

## Configure backend

By default the dev server proxies to `http://localhost:8082` where the Go API runs. Override with:

```bash
# Copy the example file and edit if needed
cp .env.example .env.local

# Or export directly
VITE_API_BASE=https://reconcilyx.example.com npm run dev
```

If the backend is protected by an API key, set `VITE_API_KEY=<your-token>` at build time, or save it at runtime under **Settings → Control plane API**. Runtime values are stored in that browser and override the build-time origin.

A static build can be hosted separately: set the Hub URL in Settings, allow that origin in `RECONCILYX_CORS_ORIGINS`, and configure the API key. No public hosted instance is operated.

## Embedding into the operator

The Go binary ships the built UI as a static filesystem via `//go:embed all:dist` in [../ui/embed.go](../ui/embed.go). Whenever you change the console:

1. `npm run build` (emits to `ui/dist`)
2. `make docker-build` or `go build ./cmd/main.go` — the linker packs the dist directory
3. The HTTP handler at `/` serves the SPA on the same port as the JSON API (8082)

## Layout

```
src/
 ├─ main.tsx              Hash router entry (works on static hosting)
 ├─ App.tsx               Shell — nav, footer, routes, toasts, shortcuts, assistant
 ├─ index.css             Ink canvas, grain, panel/button/type primitives
 ├─ lib/
 │   ├─ types.ts          API contracts and display types
 │   ├─ api.ts            Fetch wrapper + proposal/policy/metrics/chat endpoints
 │   ├─ llm.ts            BYOK provider/key stored in this browser
 │   └─ useApi.ts         Typed hook with loading/error/refetch
 ├─ store/app.ts          Zustand: cluster, namespace, selection, toasts, goal, auto-refresh
 ├─ components/
 │   ├─ PageHeader.tsx    Editorial page opener with metrics and actions
 │   ├─ Icon.tsx          Inline SVG icon set
 │   ├─ Toasts.tsx        Notification stack
 │   ├─ KpiCard.tsx       Oversized metric card
 │   ├─ Badges.tsx        Risk, confidence, phase, and approval chips
 │   ├─ DataTable.tsx     Generic table with empty and row-click states
 │   ├─ SavingsChart.tsx  USD area + CO₂ line (ComposedChart)
 │   ├─ ProposalDetailDrawer.tsx  Proposal inspector and actions
 │   ├─ BeforeAfterCard.tsx       Resource delta rows
 │   └─ AiChatDrawer.tsx / KeyboardShortcutsModal.tsx
 └─ pages/
     ├─ Overview.tsx        KPI band, savings trend, proposal queue, anomalies
     ├─ Proposals.tsx       Filters, search, bulk actions, mobile card queue, drawer
     ├─ ProposalDetail.tsx  Full-page proposal view from API detail
     ├─ Savings.tsx         Daily ledger, cluster/namespace breakdowns, line items
     ├─ Forecast.tsx        Forecast chart, what-if, observed vs proposed resources
     ├─ Policies.tsx        Policy catalogue and manifest-accurate create sheet
     └─ Settings.tsx        Connection test, assistant key, preferences, server env
```

The console was exercised against the real response shapes in `internal/api` using a local mock control plane. Cluster-side behaviour still requires a running operator; see [../STATUS.md](../STATUS.md).
