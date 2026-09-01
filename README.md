# Reconcilyx v4.0.0-beta

Reconcilyx is an open-source Kubernetes optimization platform for evaluating cost, carbon, and workload right-sizing decisions.

## 🚀 Live Demo

- **Frontend UI:** [https://reconfigy.x.vercel.app](https://reconfigy.x.vercel.app)
  *(Configure your Control Plane API URL in **Settings** or connect to a running local/remote backend.)*

## Beta Status

Reconcilyx is in active beta hardening. It is not production-ready and should not mutate production workloads without an independent security review, a non-production rollout, and environment-specific validation.

| Category | Evidence-backed status |
|---|---|
| Build and automated tests | Locally verified; repeat in your environment |
| Dashboard | Buildable React UI; deployed on Vercel, embeddable with Hub, or hostable on GitHub Pages |
| API, CLI, controller, and integrations | Implemented areas remain beta until end-to-end production-like validation |
| eBPF and node consolidation | Explicitly deferred to v5.0 planning |

The conservative inventory is [FEATURE_MATRIX.md](FEATURE_MATRIX.md). [STATUS.md](STATUS.md) and [GITHUB_ISSUES.md](GITHUB_ISSUES.md) are the release and remediation sources of truth.

## Deployment Options

- **Live Hosted UI (Vercel):** Access the web console at [https://reconfigy.x.vercel.app](https://reconfigy.x.vercel.app). Point the UI to your backend via **Settings → Control Plane API URL**.
- **Production Backend (VPS / Cloud):** Deploy the backend 24/7 on a VPS (Linode, Vultr, DigitalOcean) or managed Kubernetes using Docker, systemd, or Helm. Use Caddy/Nginx for automatic TLS and configure `RECONCILYX_CORS_ORIGINS`.
- **Testing & Evaluation (Kind + ngrok):** Run a session-based local Kind cluster and tunnel port `8082` via ngrok for temporary evaluation and demos.
- **Kubernetes In-Cluster:** Deploy via Helm using [charts/reconcilyx](charts/reconcilyx) with custom RBAC and security contexts.

See [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) for step-by-step instructions for each deployment model.

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
