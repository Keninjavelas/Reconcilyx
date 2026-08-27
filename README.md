# Reconcilyx v4.0.0-beta

Reconcilyx is an open-source Kubernetes optimization project for evaluating cost, carbon, and workload right-sizing decisions.

## Beta Status

Reconcilyx is in active beta hardening. It is not production-ready and should not mutate production workloads without an independent security review, a non-production rollout, and environment-specific validation.

| Category | Evidence-backed status |
|---|---|
| Build and automated tests | Locally verified; repeat in your environment |
| Dashboard | Buildable React UI; can be embedded with the Hub or served from GitHub Pages |
| API, CLI, controller, and integrations | Implemented areas remain beta until end-to-end production-like validation |
| eBPF and node consolidation | Explicitly deferred to v5.0 planning |

The conservative inventory is [FEATURE_MATRIX.md](FEATURE_MATRIX.md). It records `0 Verified`, `72 Partial`, and `30 Missing/Stubbed` at the audit baseline. [STATUS.md](STATUS.md) and [GITHUB_ISSUES.md](GITHUB_ISSUES.md) are the release and remediation sources of truth.

## Deployment Options

- **Local development:** run the UI against a locally running Hub at `http://localhost:8082`.
- **Kubernetes:** use the Helm chart in [charts/reconcilyx](charts/reconcilyx) after reviewing its RBAC and deployment values.
- **Static UI:** GitHub Pages can host only the browser dashboard. Operators deploy and secure their own API/control plane, then configure its URL in **Settings**.
- **Kind + ngrok:** an evaluation-only option that tunnels a locally port-forwarded Hub over temporary HTTPS. See the deployment guide; do not use it as a production architecture.

See [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) for beta deployment guidance, including the CORS and authentication requirements for a hosted UI.

## Commercial Use

Reconcilyx is licensed under the [Apache License 2.0](LICENSE). The license permits commercial use, modification, distribution, and sublicensing subject to its terms and notice requirements.

The project does not provide a hosted control plane, an SLA, a security certification, or a warranty. Do not interpret the license as a claim of production suitability or compliance certification.

## Development

```powershell
go build ./...
go test ./...
Set-Location ui
npm ci
npm run build
```

For UI development, `npm run dev` proxies API requests to `http://localhost:8082` by default. Override that proxy with `VITE_API_BASE` or `RECONCILYX_API_URL`.
