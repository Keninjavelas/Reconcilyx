#!/usr/bin/env bash
set -euo pipefail

# ============================================================
# demo/run.sh — End-to-End Reconcilyx 2.0 demo
# ============================================================
#
# Prerequisites:
#   - kind, kubectl, docker, make on PATH
#   - Go toolchain (>= 1.22)
#   - (Optional) helm, for Prometheus stack installation
#
# Usage:
#   ./demo/run.sh              # full end-to-end flow
#   SKIP_BUILD=1 ./demo/run.sh  # reuse existing pre-built image
#   CLEANUP=1 ./demo/run.sh     # delete cluster when finished
# ============================================================

IMG="${IMG:-reconcilyx/hub:latest}"
KIND_NAME="${KIND_NAME:-reconcilyx}"
DEMO_NS="default"
SKIP_BUILD="${SKIP_BUILD:-0}"
CLEANUP="${CLEANUP:-0}"

log() { printf '\033[1;36m[demo]\033[0m %s\n' "$*"; }
step() { printf '\n\033[1;33m==>\033[0m \033[1m%s\033[0m\n\n' "$*"; }

cleanup() {
  if [[ "${CLEANUP}" == "1" ]]; then
    step "Deleting Kind cluster ${KIND_NAME}"
    kind delete cluster --name "${KIND_NAME}" || true
  fi
}
trap cleanup EXIT

# 1. Kind cluster with InPlacePodVerticalScaling feature gate
step "1/6 · Creating Kind cluster (${KIND_NAME}) with InPlacePodVerticalScaling"
cat > /tmp/kind-reconcilyx.yaml <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
featureGates:
  "InPlacePodVerticalScaling": true
EOF
if ! kind get clusters | grep -q "${KIND_NAME}"; then
  kind create cluster --name "${KIND_NAME}" --config /tmp/kind-reconcilyx.yaml
fi
kubectl cluster-info --context "kind-${KIND_NAME}"

# 2. Build + deploy manager + CRDs
step "2/6 · Installing CRDs, building image and deploying Reconcilyx Hub controller"
make install
if [[ "${SKIP_BUILD}" != "1" ]]; then
  make docker-build "IMG=${IMG}"
fi
kind load docker-image "${IMG}" --name "${KIND_NAME}"
make deploy "IMG=${IMG}"

kubectl rollout status -n reconcilyx-system deployment/reconcilyx-controller-manager --timeout=120s

# 3. (Optional but recommended) Install Prometheus via kube-prometheus
if kubectl get ns monitoring 2>/dev/null | grep -q monitoring; then
  log "monitoring namespace already present — skipping Prometheus install"
else
  step "(opt) · Installing kube-prometheus stack (may take 2-3 minutes)"
  if command -v helm >/dev/null; then
    helm repo add prometheus-community https://prometheus-community.github.io/helm-charts >/dev/null 2>&1 || true
    helm repo update >/dev/null 2>&1
    helm upgrade --install kube-prometheus prometheus-community/kube-prometheus-stack \
      --namespace monitoring --create-namespace \
      --set grafana.enabled=true --timeout=10m || log "Prometheus install skipped — check cluster resources"
  else
    log "Helm not installed — skipping Prometheus install. Analysis engine will gracefully degrade to Observe mode."
  fi
fi

# 4. Deploy over-provisioned nginx workload
step "3/6 · Deploying over-provisioned nginx workload (2vCPU, 2Gi)"
kubectl apply -f - <<'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-demo
  namespace: default
  labels:
    app: nginx
spec:
  replicas: 1
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:latest
        resources:
          requests:
            cpu: "2000m"
            memory: "2Gi"
EOF
kubectl rollout status deployment/nginx-demo --timeout=90s
kubectl get pod -n "${DEMO_NS}" -l app=nginx -o wide

# 5. Apply an OptimizationPolicy (mock GitOps)
step "4/6 · Creating OptimizationPolicy (mock GitOps provider + AutoApply)"
kubectl apply -f - <<'EOF'
apiVersion: optimization.reconcilyx.io/v1alpha1
kind: OptimizationPolicy
metadata:
  name: nginx-cost-optimize
  namespace: default
spec:
  mode: AutoApply
  cooldownPeriod: 15m
  confidenceRequirement: 0.30
  riskTolerance: 70
  gitOps:
    enabled: true
    provider: mock
  targetSelectors:
    workloadSelector:
      matchLabels:
        app: nginx
    namespaces:
      - default
EOF

# 6. Wait for a proposal and print results
step "5/6 · Waiting for OptimizationProposal to appear (max 2 minutes)"
for i in $(seq 1 40); do
  found="$(kubectl get optimizationproposals.optimization.reconcilyx.io -n "${DEMO_NS}" -o name 2>/dev/null | head -n1 || true)"
  if [[ -n "${found}" ]]; then
    log "Proposal appeared: ${found}"
    break
  fi
  sleep 3
done

step "6/6 · Displaying results"
kubectl get optimizationproposals.optimization.reconcilyx.io -n "${DEMO_NS}" -o wide || true

log "Controller logs (last 25 lines, use 'stern reconcilyx-controller-manager -n reconcilyx-system' for more):"
kubectl logs -n reconcilyx-system deployment/reconcilyx-controller-manager -c manager --tail=25 2>/dev/null || true

log ""
log "===== Next steps ====="
log "  kubectl get optimizationproposals -o wide       # track execution"
log "  kubectl describe optimizationproposal <name>    # inspect rationale, confidence, risk"
log "  kubectl get pod -l app=nginx -o jsonpath='{.items[0].spec.containers[0].resources}' ; echo"
log "  make run ENABLE_WEBHOOKS=false PROMETHEUS_URL=http://localhost:9090  # run locally against the cluster"
log "  CLEANUP=1 ./demo/run.sh                         # tear down when finished"
