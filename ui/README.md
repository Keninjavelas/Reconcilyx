# Reconcilyx Dashboard (ui/)

Enterprise FinOps console for the Reconcilyx operator. Dark-first editorial aesthetic — Vite, TypeScript, React 18, Tailwind, Zustand, Recharts.

## Quick start

```bash
cd ui
npm install

# local dev (proxies /api and /healthz to http://localhost:8082)
npm run dev

# typecheck
npm run typecheck

# production build → lands in ui/dist (embedded by ../ui/embed.go)
npm run build
```

## Configure backend

By default the dev server proxies to `http://localhost:8082` where the Go
`internal/api` server runs. Override with:

```bash
# Copy the example file and edit if needed
cp .env.example .env.local

# Or export directly
VITE_API_BASE=https://reconcilyx.example.com npm run dev
```

If the backend is protected by an API key, set:

```bash
VITE_API_KEY=<your-token>
```

For a GitHub Pages build, users can instead save an HTTPS endpoint under
**Settings -> Control Plane API URL**. The setting is stored in that browser's
local storage and overrides the build-time API URL. The remote API must allow
the GitHub Pages origin through CORS and must have its own authentication.

## Embedding into the operator

The Go binary ships the built UI as a static filesystem via `//go:embed all:dist`
in [../ui/embed.go](../ui/embed.go). Whenever you change the dashboard:

1. `npm run build` (emits to `ui/dist`)
2. `make docker-build` or `go build ./cmd/main.go` — the linker packs the dist dir.
3. The HTTP handler at `/` serves the SPA on the same port as the JSON API (8082).

## Layout

```
src/
 ├─ main.tsx            Hash router entry (works on static hosting)
 ├─ App.tsx             Shell — nav, footer, routes, toasts
 ├─ index.css           Grain overlay, ink-950 bg, panel/Kpi/btn primitives
 ├─ lib/
 │   ├─ types.ts        ProposalListItem, SavingsResponse, etc.
 │   ├─ api.ts          JSON fetch wrapper + 8 endpoint methods
 │   └─ useApi.ts       typed hook w/ loading/error/refetch
 ├─ store/app.ts        Zustand: cluster, ns filter, toasts, formatCurrency, formatCarbonGrams
 ├─ components/
 │   ├─ Toasts.tsx      4-kind notification stack
 │   ├─ KpiCard.tsx     Accent gradient header + delta chip
 │   ├─ Badges.tsx      RiskBadge, ConfidenceBadge, PhaseChip, ApprovalChip
 │   ├─ DataTable.tsx   Generic <T> row/col table w/ empty & click
 │   ├─ SavingsChart.tsx  Dual-axis USD+CO₂ Recharts line
 │   └─ BeforeAfterCard.tsx  Before/after resource delta rows with ✓ better / ⚠ worse chips
 └─ pages/
     ├─ Overview.tsx        4 Kpi + SavingsChart + top proposals
     ├─ Proposals.tsx       Filter chips + search + DataTable
     ├─ ProposalDetail.tsx  Summary KPIs, BeforeAfter, timeline, approve/reject/rollback
     ├─ Savings.tsx         4x KPIs + cluster/ns bars + full line chart
     └─ Policies.tsx        Policy table + 7-step Create wizard w/ YAML preview & carbon slider
```
