# Deployment Guide

## Beta Boundary

This guide helps you run the Reconcilyx beta for development and evaluation. It does not certify the controller, its mutation path, or its integrations for production workloads. Start with a disposable Kind cluster and use an account with minimal privileges.

## Local UI and API

Build the UI and run the Hub on the same origin when possible. This avoids browser cross-origin configuration during local development.

```powershell
go build ./...
Set-Location ui
npm ci
npm run build
npm run dev
```

The Vite development server proxies `/api` and `/healthz` to `http://localhost:8082` by default. Set `VITE_API_BASE` or `RECONCILYX_API_URL` before `npm run dev` to use another local API origin.

## Kubernetes Evaluation

Review [charts/reconcilyx](../charts/reconcilyx) before installing. Do not apply the chart unchanged to a production cluster.

```powershell
helm upgrade --install reconcilyx ./charts/reconcilyx --namespace reconcilyx-system --create-namespace
kubectl get pods --namespace reconcilyx-system
```

The exact image, secrets, RBAC scope, Prometheus connectivity, and external provider credentials are environment-dependent. Verify the installed chart's values and the resulting service names before port-forwarding or configuring a client.

## Kind Plus ngrok Demo

This option exposes a local evaluation environment through a temporary public HTTPS URL. It requires Docker Desktop to be running and an ngrok account with an authentication token. It is not suitable for production, shared credentials, or a privileged cluster.

Create a disposable Kind cluster, build the controller image, and install the chart. The current chart deploys a single controller-manager image; it does not have separate `hub` or `agent` values.

```powershell
kind create cluster --name reconcilyx
make install
make docker-build IMG=reconcilyx/hub:local
kind load docker-image reconcilyx/hub:local --name reconcilyx

helm upgrade --install reconcilyx ./charts/reconcilyx `
  --namespace reconcilyx-system --create-namespace `
  --set image.repository=reconcilyx/hub `
  --set image.tag=local `
  --set image.pullPolicy=Never `
  --set carbon.provider=static `
  --set pricing.providers.aws.enabled=false `
  --set pricing.providers.gcp.enabled=false `
  --set pricing.providers.azure.enabled=false `
  --set-string api.key=replace-with-a-long-random-demo-key `
  --set-string api.corsOrigins=https://YOUR-GITHUB-USER.github.io
```

Wait for the controller-manager, then port-forward the chart service. With release name `reconcilyx`, the service is named `reconcilyx`.

```powershell
kubectl wait --for=condition=available deployment/reconcilyx-controller-manager --namespace reconcilyx-system --timeout=180s
kubectl --namespace reconcilyx-system port-forward service/reconcilyx 8082:8082
```

In a second terminal, configure ngrok once using your own token and tunnel the port-forwarded service:

```powershell
ngrok config add-authtoken YOUR_NGROK_TOKEN
ngrok http 8082
```

Set the displayed HTTPS URL and the demo API key under **Settings -> Control Plane API URL** in the Pages UI. Verify a read-only endpoint first:

```powershell
curl.exe -H "X-API-Key: replace-with-a-long-random-demo-key" https://YOUR-NGROK-URL/api/proposals
```

The API is reachable by anyone who learns the ngrok URL and key. Stop the tunnel and delete the Kind cluster when the demo ends. Use a Kubernetes Secret rather than Helm command-line values for any non-demo credential.

## GitHub Pages UI

The workflow at [deploy-ui.yml](../.github/workflows/deploy-ui.yml) publishes the static React dashboard after a push to `main` that changes `ui/`, or after manual dispatch. It does **not** deploy a Hub, Kubernetes controller, API, credentials, or data plane.

Repository administrators must enable **Settings -> Pages -> GitHub Actions**. Once a run succeeds, GitHub provides the actual Pages URL. Do not publish a guessed URL.

The hosted UI uses hash routes and lets each browser choose an API origin in **Settings -> Control Plane API URL**. That choice is stored only in that browser's local storage.

## Connecting a Hosted UI

1. Deploy and secure a Hub/API you operate.
2. Configure TLS and authentication for that API.
3. Set `RECONCILYX_CORS_ORIGINS` on the API to the exact Pages origin, for example `https://owner.github.io`.
4. Open the Pages UI and save the Hub's HTTPS origin and API key under **Settings -> Control Plane API URL**.
5. Confirm `/healthz` and a read-only endpoint before trying any write action.

Never use a wildcard CORS policy or expose a privileged Kubernetes control plane anonymously for a demo. GitHub Pages is public and static; it is not an authentication boundary.

## Commercial Use

Reconcilyx is available under Apache-2.0, which permits commercial use subject to the [license](../LICENSE). The project supplies no warranty, managed hosting, security certification, or enterprise support commitment.

## Cleanup

For an evaluation namespace, remove only resources you explicitly installed:

```powershell
helm uninstall reconcilyx --namespace reconcilyx-system
```

Do not use broad commands such as deleting all CRDs or all resources from an existing cluster.
