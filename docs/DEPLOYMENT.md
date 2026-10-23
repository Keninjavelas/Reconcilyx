# Reconcilyx Deployment Guide

## Overview

Reconcilyx ships as a single Go control plane with the React dashboard embedded in the binary. The same process serves:

- the Kubernetes controllers (reconciliation, telemetry, analysis, execution),
- the REST API on `:8082` (configurable with `RECONCILYX_API_PORT`),
- the dashboard at `/` on that same port.

Because the UI is embedded, no separate hosting is required. A static build of `ui/` can still be hosted separately and pointed at a Hub URL from the in-app **Settings** page.

---

## 1. Dashboard Hosting Model

| Mode | How it works |
|---|---|
| **Embedded (default)** | Open `http://<hub-host>:8082/`. The UI calls the same origin; no CORS configuration is needed. |
| **Static (optional)** | Build `ui/` with `npm run build`, host `ui/dist/` anywhere, and set **Settings → Control Plane API URL** to the Hub URL. Add that origin to `RECONCILYX_CORS_ORIGINS`. |

No public hosted dashboard is operated. Treat embedded or self-hosted deployment as the supported path. The optional GitHub Pages workflow (`.github/workflows/deploy-ui.yml`) is unverified and has no claimed deployment URL.

---

## 2. Production Backend Deployment (VPS: Linode, Vultr, DigitalOcean)

For 24/7 availability, run the Hub container on a VPS with Docker and terminate TLS with Caddy or Nginx.

### Step 1: Provision the VPS and install Docker

Provision an Ubuntu 22.04/24.04 or Debian VPS (2 vCPU / 4 GB RAM is sufficient for a small number of clusters).

```bash
sudo apt update && sudo apt install -y docker.io docker-compose-plugin
sudo systemctl enable --now docker
```

### Step 2: Place the cluster kubeconfig

The container runs as a non-root user (UID 65532), so mount the kubeconfig at a readable path rather than `/root/.kube/config`.

```bash
mkdir -p ~/reconcilyx
cp ~/.kube/config ~/reconcilyx/kubeconfig
chmod 644 ~/reconcilyx/kubeconfig
```

Use a dedicated service account with the RBAC from `charts/reconcilyx/templates/rbac.yaml` instead of an admin kubeconfig for production.

### Step 3: Run the Hub container

Published images are produced by `.github/workflows/publish-image.yml` on `v*` tags:

```bash
docker run -d \
  --name reconcilyx-hub \
  --restart unless-stopped \
  -p 127.0.0.1:8082:8082 \
  -v ~/reconcilyx/kubeconfig:/kube/config:ro \
  -e KUBECONFIG=/kube/config \
  -e RECONCILYX_CORS_ORIGINS="https://app.yourdomain.com" \
  -e RECONCILYX_API_KEY="your-secure-production-api-key" \
  -e RECONCILYX_CARBON_PROVIDER="static" \
  ghcr.io/keninjavelas/reconcilyx:v4.0.0-beta
```

- `RECONCILYX_API_KEY` is optional. When set, every API request, including those from the UI, must send `X-API-Key`.
- `RECONCILYX_CORS_ORIGINS` is only needed when the dashboard is hosted on a different origin. Leave it unset for the embedded UI.
- The image is distroless and runs with a read-only root filesystem compatible configuration; do not mount volumes other than the kubeconfig.

### Step 4: Configure automatic HTTPS with Caddy

```bash
sudo apt install -y debian-keyring debian-archive-keyring apt-transport-https curl
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | sudo gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | sudo tee /etc/apt/sources.list.d/caddy-stable.list
sudo apt update && sudo apt install -y caddy
```

Edit `/etc/caddy/Caddyfile`:

```caddy
api.yourdomain.com {
    reverse_proxy 127.0.0.1:8082
}
```

Reload and verify:

```bash
sudo systemctl reload caddy
curl -fsS https://api.yourdomain.com/healthz
curl -fsS -H "X-API-Key: your-secure-production-api-key" https://api.yourdomain.com/api/proposals
```

The dashboard is then available at `https://api.yourdomain.com/` with the embedded UI.

---

## 3. Local Evaluation and Demos (Kind + ngrok)

> Kind + ngrok is a session-based demo setup for local exploration. It stops when the cluster or tunnel is closed.

### Prerequisites

- Docker Desktop running
- [Kind](https://kind.sigs.k8s.io/) and [Helm](https://helm.sh/) installed
- Optional: [ngrok](https://ngrok.com/) for sharing the local Hub

### Step 1: Create the Kind cluster

```bash
kind create cluster --name reconcilyx
kubectl config use-context kind-reconcilyx
```

The Helm chart bundles CRDs from `charts/reconcilyx/crds/`, so no separate `make install` is required for a chart install.

### Step 2: Build and load the image

The Dockerfile builds the UI and the Go binary in one pass, so no local Node toolchain is required:

```bash
make docker-build IMG=ghcr.io/keninjavelas/reconcilyx:local
kind load docker-image ghcr.io/keninjavelas/reconcilyx:local --name reconcilyx
```

### Step 3: Install the Helm release

```bash
helm upgrade --install reconcilyx ./charts/reconcilyx \
  --set image.tag=local \
  --set image.pullPolicy=Never \
  --set carbon.provider=static
```

### Step 4: Open the dashboard

```bash
kubectl port-forward service/reconcilyx 8082:8082
```

Open `http://localhost:8082/`. The embedded UI uses the same origin automatically.

To share the session, expose the port in a second terminal:

```bash
ngrok http 8082
```

If you use the separately hosted static UI instead of the embedded one, paste the generated `https://<subdomain>.ngrok-free.app` URL into **Settings → Control Plane API URL** and add that origin to `api.corsOrigins` (or `RECONCILYX_CORS_ORIGINS`).

---

## 4. In-Cluster Kubernetes Deployment (Helm)

```bash
helm upgrade --install reconcilyx ./charts/reconcilyx \
  --namespace reconcilyx-system \
  --create-namespace \
  --set carbon.provider=static
```

Notes:

- **CRDs:** `charts/reconcilyx/crds/` contains the generated CRDs and Helm installs them automatically. Helm does not upgrade files in `crds/` on `helm upgrade`; after regenerating manifests, run `make helm-crds` and apply the CRDs explicitly before upgrading the release:
  ```bash
  kubectl apply -f charts/reconcilyx/crds/
  ```
- **Image:** `image.tag` defaults to `.Chart.AppVersion` (`v4.0.0-beta`). Override `image.repository` / `image.tag` to use a different registry.
- **API key:** set `api.key` to require `X-API-Key` on every request. Leave it empty only for development.
- **CORS:** not needed for the embedded UI. Set `api.corsOrigins` only when hosting the static UI on a different origin, for example `https://keninjavelas.github.io`.
- **Webhooks:** the chart starts the manager with `ENABLE_WEBHOOKS=false`, so no cert-manager or webhook service is required. CRD validation markers still apply server-side through the API server's structural schema.
- **Probes:** liveness `/healthz` and readiness `/readyz` are wired to port `8081`; metrics remain on `8080` and the API/UI on `8082`.

Verify the rollout:

```bash
kubectl -n reconcilyx-system rollout status deployment/reconcilyx-controller-manager
kubectl -n reconcilyx-system port-forward service/reconcilyx 8082:8082
```

---

## 5. Building and Publishing Images and Charts

### Container image

```bash
make docker-build IMG=ghcr.io/keninjavelas/reconcilyx:v4.0.0-beta
make docker-push IMG=ghcr.io/keninjavelas/reconcilyx:v4.0.0-beta
```

CI publishing is handled by `.github/workflows/publish-image.yml` for tags matching `v*`, producing `ghcr.io/keninjavelas/reconcilyx:<tag>`.

### Helm chart

```bash
make helm-crds   # copy regenerated CRDs into the chart
make helm-chart  # package into dist/reconcilyx-<version>.tgz
helm lint charts/reconcilyx
```

### Local binary

The Go binary embeds `ui/dist`, which is not committed. Build the UI first:

```bash
make build-ui
make build
./bin/manager --health-probe-bind-address=:8081
```

---

## 6. Static UI (Optional)

The static build can be served from any file host:

```bash
cd ui
npm ci
npm run build
# deploy ui/dist/
```

Set the Hub URL either in the UI at runtime or at build time via `VITE_API_BASE`. The GitHub Pages workflow at `.github/workflows/deploy-ui.yml` is optional and unverified; no live public URL is claimed. Configure `RECONCILYX_CORS_ORIGINS` and the API key for whichever origin hosts the static build.

---

## Commercial Use and Disclaimer

Reconcilyx is licensed under [Apache License 2.0](../LICENSE). The project supplies no warranty, managed hosting, security certification, or enterprise SLA. Review all RBAC policies and resource mutations, and run the execution gate in a non-production cluster first.
