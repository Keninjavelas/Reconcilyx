# Product Requirements Document (PRD) — Reconcilyx 2.0 Operator Dashboard

| Field | Value |
| :--- | :--- |
| Product | Reconcilyx Operator Dashboard (Stage 7 Web UI) |
| Version | 2.0.0 — Stage 7 |
| Owner | Reconcilyx Maintainers |
| Audience | FinOps, SRE, Platform teams |
| Status | Approved in-principle by user, awaiting frontend build |

---

## 1. Problem Statement

Reconcilyx 2.0 has a technically robust ML-driven cost-optimization engine (RL Q-learning, Z-score anomaly, statistical forecasting, GitOps PR flow, in-place vertical scaling), but **it exposes zero human interface**. Engineers and FinOps teams cannot:
- Approve or reject risky proposals from a browser
- See a live savings dashboard or forecast graphs
- Preview an optimization's impact before applying it
- Switch clusters and namespaces for multi-cluster hub/spoke
- Create policies through a form instead of YAML

The result is a platform that is powerful but **invisible** to the people who have to trust it. The goal of the Stage 7 UI is to bridge the technical core with the business decision-makers, converting "dark" cost savings into signed-off, auditable optimizations.

---

## 2. Goals & Non-Goals

### 2.1 Goals
- **G1 Visibility**: Provide a single-pane dashboard for total savings, forecast trends, anomaly alerts, PR activity — per cluster / per namespace.
- **G2 Control**: Implement human-in-the-loop approve / reject / rollback with typed reasons and audit log.
- **G3 Safety**: A What-If simulator that runs the Analyzer on user-supplied CPU/Memory inputs and returns cost + risk impact **without writing to the cluster**.
- **G4 GitOps**: Display PR / MR links, status badges, merge status alongside the proposal that generated them.
- **G5 Adoption**: Embed into the existing Go binary so `make deploy` ships UI + API + controller together — no extra services for quick start.

### 2.2 Non-Goals
- Not a replacement for Grafana (Grafana dashboard JSON still lives under `hack/`, UI embeds critical panels only).
- Not a YAML editor for arbitrary Kubernetes objects — policy form maps only to `OptimizationPolicySpec`.
- No first-class RBAC in the UI (rely on an API key or kubeconfig-backed server-side auth; production users put OIDC proxy in front).
- No live websocket push — the UI polls `/api/*` endpoints at a configurable cadence.

---

## 3. User Personas

| Persona | Primary Job | Typical Action |
| :--- | :--- | :--- |
| **FinOps Lead** | Keep monthly cloud spend under budget | Review Savings dashboard, export CSV, approve high-value proposals |
| **SRE / Platform Engineer** | Keep clusters reliable + optimized | Approve/reject mid-risk proposals, use What-If, trigger rollback |
| **Application Engineer** | Keep their app performant | Inspect forecast for their namespace, contest a proposal |
| **Manager / Reviewer** | Sign off on large optimization sweeps | Audit log, scheduled batch status |

---

## 4. Core User Journeys & Flows

### J1 — "I want to approve a single risky proposal"
1. Login → open Dashboard → see "Pending Approvals" count KPI.
2. Navigate to **Proposals** tab, filter `approvalState=Pending`.
3. Open proposal detail page → review:
   - Before / after CPU + Memory side-by-side
   - Risk score, confidence score, rationale
   - Estimated monthly savings
   - Historical forecast graph (actual vs predicted CPU/memory)
   - Anomaly events for the workload in the last 72h
   - Linked Git PR URL (if GitOps is enabled for the policy)
4. Click **Approve** → type comment → confirms proposal enters `Approved`.

### J2 — "I want to preview a 20% CPU cut on a workload without touching the cluster"
1. Navigate to **What-If** page.
2. Select: cluster, namespace, workload (Deployment selector auto-populated).
3. Current CPU/Memory requests are pre-filled.
4. Tweak CPU from `2000m → 1600m` (slider + numeric input). Keep memory the same.
5. Click **Simulate**.
6. Returned:
   - Engine confidence score, risk score, whether the change is within Policy's SafetyConstraints
   - Estimated monthly USD savings
   - Diff panel (green additions / red removals)
   - Plain-language explanation from the RuleRegistry that fired

### J3 — "A rollback is needed — do it now"
1. Proposals → Detail → Rollback button (enabled if phase ∈ {Verified, Completed, Failed, RollbackRequired}).
2. Confirm with reason.
3. Server writes `Phase=RollingBack` → controller runs the ExecutionSnapshot restore → UI updates to `PhaseRolledBack` when complete.

### J4 — "Create a new policy via form (not YAML)"
1. Policies tab → "Create Policy".
2. Step wizard:
   - (1) Name, namespace, Mode (Observe / Propose / AutoApply), Objective (Cost / Performance / Efficiency)
   - (2) Target selectors (namespace labels, workload labels `app:X`)
   - (3) Resource constraints (min/max CPU + memory)
   - (4) Safety: Min replicas, max latency increase, cooldown, RiskTolerance / ConfidenceRequirement sliders
   - (5) Rollback: error-rate threshold %, OOMKilled window
   - (6) GitOps (toggle, provider, owner/repo, base branch, auto-merge)
3. Review preview of the rendered YAML → click Apply.

### J5 — "Switch between spoke clusters in a Hub + Spoke setup"
1. Top-left cluster selector listing all `ClusterRegistration` CRs.
2. Defaults to `hub` (local).
3. Every `/api/proposals` query is scoped by `targetClusterName`.

---

## 5. Feature Requirements

### F1 Dashboard
- **KPIs**: Total savings (USD / month, 30-day trailing), workloads under management, proposals pending review, active anomalies.
- **Savings Chart**: Recharts area chart of 30-day savings per cluster.
- **Top 10 Savings Opportunities**: ranked table.
- **Recent Anomalies Feed**: severity badges (green/yellow/orange/red).

### F2 Proposals
- Filterable/sortable list view (status, cluster, namespace, risk score, savings, rec type).
- Detail view with:
  - Phase timeline (Detected → Evaluating → Approved → Scheduled → Executing → Verifying → Stabilizing → Completed / RolledBack).
  - Resources before/after cards.
  - Forecast panel (actual vs predicted CPU & memory).
  - Decision rationale + rule hits panel.
  - Approve / Reject / Rollback actions with mandatory reason fields.
  - GitOps PR card (link, status, mergedBy if known).

### F3 Policies
- List of all `OptimizationPolicy` CRs (status, mode, objective, matched workload count).
- Detail view: matched workload list, allowed recommendations, safety sliders.
- Create / edit wizard (§J4).

### F4 What-If Simulator
- Workload selector + CPU/memory sliders.
- POST `/api/what-if` returns: engine Analyzer output (confidence, risk) + cost impact + per-rule diff.

### F5 Multi-Cluster Switcher
- Populated from `ClusterRegistration` CRs.
- Every top-level query (`/api/proposals`, `/api/metrics/savings`) accepts `?cluster=`.

### F6 Anomaly Alerts
- Recent anomaly table (severity, workload, metric CPU/mem, timestamp, z-score).
- Click-through to workload forecast.

### F7 GitOps View
- List cards for open PRs (provider, repo, head branch, status, related proposal link).

### F8 Natural Language (optional v2.1)
- Ollama-style prompt box: "What happens if I cut my `nginx-demo` CPU by 20%?"
- Calls the What-If endpoint and wraps output in a plain-English summary.
- Deferred to v2.1 unless the quick local test is trivial.

### F9 Audit Log
- Table: timestamp, actor, proposal (name/ns), action (approve/reject/rollback), reason, client IP, user agent.
- Server stores in-memory ring buffer + optional append to ConfigMap for persistence across restarts.

### F10 Notifications
- Slack webhook: posts proposals, approvals, rollbacks to `#reconcilyx`.
- Email (SMTP): subscription list for rollbacks + high-risk approvals.
- Configured via env: `SLACK_WEBHOOK_URL`, `SMTP_HOST`, etc.

---

## 6. UI & Aesthetic Direction

**Tone**: *"Calm confidence"* — Refined, industrial-finance aesthetic that reads as a reliable cost-control tool, not a flashy ML toy. Engineers must trust it with their production.

**Design Choices**:
- **Theme**: Dark-mode first (operators live in dark terminals), light-mode toggle.
- **Typography**: Editorial display font for KPI numerals paired with a crisp mono for numbers/resource quantities. Avoid Inter. Prefer **"Instrument Serif / Sora / JetBrains Mono"** pairing or comparable.
- **Color palette**:
  - Base: `#0B0F17` / `#111827` dark surfaces, `#F8FAFC` text.
  - Accents: saving-green `#22C55E`, risk-red `#EF4444`, anomaly-orange `#F97316`, forecast-teal `#06B6D4`, brand-indigo `#6366F1`.
  - Always use semantic gradient fills on area charts (not generic purple-on-white).
- **Motion**: Subtle staggered mount reveals on dashboard KPI cards, gentle hover lift on cards, phase timeline dots fill sequentially.
- **Spatial composition**: Left rail navigation (icons + labels), top bar cluster switcher + user/notifications, body grid of 12-cols with asymmetric 8+4 split for primary-content + side panel.
- **Background texture**: Soft `noise.png` grain at 3% opacity overlay + subtle brand gradient mesh on dashboard hero.
- **KPI Cards**: Use 3px top-border colored by category (green=save, red=risk, teal=forecast).

**Differentiating "remember this" element**: On the Dashboard hero, show a **live animated liquid-fill "Savings Reservoir" gauge** that fills up as cumulative savings increases, with monthly target line and callout.

---

## 7. Scope & Release Criteria

| Release | Contents | Entry Criteria |
| :--- | :--- | :--- |
| **v2.0.0 Stage 7** | F1-F7, F9-F10, embed in Go binary, docs/ui.md, build-ui + run-api targets | Unit tests pass, go build passes, npm build passes, end-to-end approval flow works against the Kind cluster demo |
| **v2.1+** | F8 natural language (Ollama), cost-budget-aware scheduler, Jira/Linear ticket creator, OIDC auth | Stage 7 shipped + adoption signals |

### Exit criteria for Stage 7 UI build
- ✅ All F1-F7 pages render without errors against mock API responses.
- ✅ Approve/Reject/Rollback POSTs correctly mutate `OptimizationProposal.status.approvalState` and write audit log.
- ✅ `make build-ui` produces `ui/dist/` that is embedded and served by the Go binary at `/`.
- ✅ docs/ui.md documents: launch commands, configuration, permission model, screenshots.
- ✅ README.md updated with QuickStart including the UI.

---

## 8. Open Questions / Resolutions

| Question | Resolution |
| :--- | :--- |
| Authentication model? | Server verifies API key header `X-API-Key` against env `RECONCILYX_API_KEY`. Put OIDC in front for production. |
| History persistence? | In-memory ring buffer of 10,000 audit events + optional append to a labeled ConfigMap on each proposal mutation. |
| React-only vs SSR? | Vite + React SPA served from Go embed. No SSR, simpler binary. |
| Forecast chart data source? | `/api/forecast/:workload` calls forecasting.Predictor + cached Prometheus range query (TTL 30s). |
| What-If runs which engine? | Direct call to `engine.Analyzer.Analyze()` with the user-supplied Resources, against cached telemetry — no K8s writes. |
