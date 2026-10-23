# Reconcilyx v4.0.0-beta

Reconcilyx is an open-source Kubernetes optimization platform for evaluating cost, carbon, and workload right-sizing decisions.

## Console

The React operator console is embedded in the Hub and served on the same port as the API (`http://<hub-host>:8082/` by default). No public demo instance is operated. A static build of `ui/dist` can be hosted separately and pointed at a Hub from **Settings → Control Plane API**; see [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) for the CORS and API-key requirements.

## Beta Status

Reconcilyx is in active beta hardening. It is not production-ready and should not mutate production workloads without an independent security review, a non-production rollout, and environment-specific validation.

| Category | Evidence-backed status |
|---|---|
| Build and automated tests | Locally verified; repeat in your environment |
| Dashboard | React console embedded in the Hub; optional static hosting pointed at a Hub URL |
| API, CLI, controller, and integrations | Implemented areas remain beta until end-to-end production-like validation |
| Actuation safety | Kill switch, health and TTL gates, and HPA/VPA conflict rejection before mutation; in-place resizes are ephemeral, GitOps is the persistence path |
| eBPF and node consolidation | Explicitly deferred to v5.0 planning |

The conservative inventory is [FEATURE_MATRIX.md](FEATURE_MATRIX.md). [STATUS.md](STATUS.md) and [GITHUB_ISSUES.md](GITHUB_ISSUES.md) are the release and remediation sources of truth.

## Deployment Options

- **Embedded console (default):** Open `http://<hub-host>:8082/` after deploying the Hub. The UI calls the API on the same origin, so no CORS setup is required.
- **Static UI (optional):** Host `ui/dist` on any HTTPS origin, set the Hub URL under **Settings → Control Plane API**, allow that origin in `RECONCILYX_CORS_ORIGINS`, and configure the API key. No public hosted instance is claimed.
- **Production Backend (VPS / Cloud):** Deploy the backend 24/7 on a VPS (Linode, Vultr, DigitalOcean) or managed Kubernetes using Docker, systemd, or Helm. Use Caddy/Nginx for automatic TLS and configure `RECONCILYX_CORS_ORIGINS`.
- **Testing & Evaluation (Kind + ngrok):** Run a session-based local Kind cluster and tunnel port `8082` via ngrok for temporary evaluation and demos.
- **Kubernetes In-Cluster:** Deploy via Helm using [charts/reconcilyx](charts/reconcilyx) with custom RBAC and security contexts.

See [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) for step-by-step instructions for each deployment model.

## v5.0 Planned

The following are out of scope for `v4.0.0-beta` and have no committed release date:

- eBPF telemetry via Cilium, Pixie, or Tetragon. The v4 beta uses Prometheus and warns when the eBPF stub is invoked.
- Node consolidation and Karpenter actuation.

## Commercial Use

Reconcilyx is licensed under the [Apache License 2.0](LICENSE). The license permits commercial use, modification, distribution, and sublicensing subject to its terms and notice requirements.

The project does not provide a hosted control plane, an SLA, a security certification, or a warranty. Do not interpret the license as a claim of production suitability or compliance certification.

## Development

The Go binary embeds `ui/dist`, so build the console before building Go:

```powershell
Set-Location ui
npm ci
npm run build
Set-Location ..
go build ./...
make test   # downloads envtest binaries, then runs controller and unit tests
```

For UI development, `npm run dev` proxies API requests to `http://localhost:8082` by default. Override that proxy with `VITE_API_BASE` or `RECONCILYX_API_URL`. See [ui/README.md](ui/README.md) for the console layout and configuration.
