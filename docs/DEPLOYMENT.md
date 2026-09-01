# Reconcilyx Deployment Guide

## Overview & Architecture

Reconcilyx consists of two main components:
1. **Frontend Dashboard:** A React single-page application that runs entirely in the browser (hosted on Vercel, GitHub Pages, or embedded in the Hub container).
2. **Backend Control Plane / Hub:** A Go controller-manager and REST API (`:8082`) that connects to Kubernetes, monitors workloads, evaluates cost/carbon metrics, and executes optimization proposals.

---

## 🌐 1. Live Frontend (Vercel)

The live official dashboard is hosted on Vercel:
👉 **[https://reconfigy.x.vercel.app](https://reconfigy.x.vercel.app)**

### Connecting to a Backend:
- Open the dashboard at `https://reconfigy.x.vercel.app`.
- Navigate to **Settings** → **Control Plane API URL**.
- Enter your public backend API URL (e.g., `https://api.yourdomain.com` or your session ngrok URL).
- Set your optional `X-API-Key` if configured on the backend.
- Click **Save Settings**. The dashboard persists this configuration in browser `localStorage`.

---

## 🖥️ 2. Production Backend Deployment (VPS: Linode, Vultr, DigitalOcean)

For 24/7 availability and multi-cluster monitoring, deploy the Hub on a virtual private server (VPS) with Docker and automatic TLS via Caddy or Nginx.

### Step 1: Provision the VPS & Install Docker
Provision an Ubuntu 22.04/24.04 or Debian VPS (e.g., 2 vCPU, 4GB RAM).

```bash
# Update system & install Docker
sudo apt update && sudo apt install -y docker.io docker-compose-plugin
sudo systemctl enable --now docker
```

### Step 2: Copy Cluster Kubeconfig
Place your target cluster's kubeconfig on the server:
```bash
mkdir -p ~/.kube
nano ~/.kube/config  # Paste your Kubernetes cluster kubeconfig
```

### Step 3: Run the Hub Container
Pull and run the Reconcilyx Hub container:
```bash
docker run -d \
  --name reconcilyx-hub \
  --restart unless-stopped \
  -p 127.0.0.1:8082:8082 \
  -v ~/.kube/config:/root/.kube/config:ro \
  -e RECONCILYX_CORS_ORIGINS="https://reconfigy.x.vercel.app,https://reconfigy-x-1d85g8ye7-aryans-projects-003ed0e9.vercel.app" \
  -e RECONCILYX_API_KEY="your-secure-production-api-key" \
  -e RECONCILYX_CARBON_PROVIDER="static" \
  ghcr.io/reconcilyx/operator:latest
```

### Step 4: Configure Automatic HTTPS with Caddy
Install Caddy for zero-configuration SSL/TLS certificates:
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

Reload Caddy:
```bash
sudo systemctl reload caddy
```

Verify the endpoint:
```bash
curl -H "X-API-Key: your-secure-production-api-key" https://api.yourdomain.com/healthz
```

---

## 🧪 3. Local Evaluation & Demos (Kind + ngrok)

> ⚠️ **Note:** Kind + ngrok is a **session-based demo environment** intended for local exploration and testing. It will terminate when your terminal or tunnel closes.

### Prerequisites:
- Docker Desktop running
- [Kind](https://kind.sigs.k8s.io/) and [Helm](https://helm.sh/) installed
- [ngrok](https://ngrok.com/) account & auth token

### Step 1: Create Kind Cluster & Install CRDs
```powershell
# Create cluster
kind create cluster --name reconcilyx
kubectl config use-context kind-reconcilyx

# Install CRDs
make install
```

### Step 2: Build & Load Docker Image
```powershell
make docker-build IMG=reconcilyx/hub:local
docker tag reconcilyx/hub:local ghcr.io/reconcilyx/operator:local
kind load docker-image ghcr.io/reconcilyx/operator:local --name reconcilyx
```

### Step 3: Install Helm Release
```powershell
helm upgrade --install reconcilyx ./charts/reconcilyx `
  --set image.tag=local `
  --set image.pullPolicy=Never `
  --set carbon.provider=static
```

### Step 4: Expose the API Port
```powershell
kubectl port-forward service/reconcilyx 8082:8082
```

In a second terminal:
```powershell
ngrok http 8082
```

Copy the generated `https://<subdomain>.ngrok-free.app` URL and paste it into **Settings** on `https://reconfigy.x.vercel.app`.

---

## ☸️ 4. In-Cluster Kubernetes Deployment (Helm)

For enterprise clusters, deploy the chart directly into your staging/production Kubernetes cluster:

```bash
helm upgrade --install reconcilyx ./charts/reconcilyx \
  --namespace reconcilyx-system \
  --create-namespace \
  --set carbon.provider=static \
  --set api.corsOrigins="https://reconfigy.x.vercel.app"
```

---

## 📄 5. GitHub Pages UI (Fallback)

The repository includes a GitHub Actions workflow at [.github/workflows/deploy-ui.yml](../.github/workflows/deploy-ui.yml) that builds and deploys the static UI to GitHub Pages whenever changes are pushed to `ui/`.

---

## Commercial Use & Disclaimer

Reconcilyx is licensed under [Apache License 2.0](../LICENSE). The project supplies no warranty, managed hosting, security certification, or enterprise SLA. Review all RBAC policies and resource mutations prior to production usage.
