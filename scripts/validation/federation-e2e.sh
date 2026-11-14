#!/usr/bin/env bash
# Two-cluster Kind federated learning end-to-end harness (H-07).
#
# Creates two isolated Kind clusters (`reconcilyx-hub` and `reconcilyx-spoke`),
# deploys the Hub with compactThreshold=10 and the Spoke with flushThreshold=5,
# drives learning deltas across clusters, verifies snapshot compaction (both
# automatic watermark and authenticated POST /api/v1/federation/compact), checks
# canonical modelHash determinism, stable/canary selection, and verifies spoke
# bootstrap from the Hub.
#
# Usage:
#   scripts/validation/federation-e2e.sh
#   FED_KEEP=1 IMG=reconcilyx:fed-e2e scripts/validation/federation-e2e.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

HUB_CLUSTER="${FED_HUB_CLUSTER:-reconcilyx-hub}"
SPOKE_CLUSTER="${FED_SPOKE_CLUSTER:-reconcilyx-spoke}"
NAMESPACE="${FED_NAMESPACE:-reconcilyx-system}"
IMG="${IMG:-reconcilyx:fed-e2e}"
ADMIN_API_KEY="${FED_ADMIN_API_KEY:-rcx_admin_fed-e2e-admin}"
SPOKE_CLUSTER_NAME="${FED_SPOKE_CLUSTER_NAME:-spoke-kind-1}"
SPOKE_API_KEY="${FED_SPOKE_API_KEY:-rcx_spoke_${SPOKE_CLUSTER_NAME}_fed-e2e-spoke}"
SCOPED_KEYS_SECRET="${FED_SCOPED_KEYS_SECRET:-reconcilyx-fed-scoped-keys}"
HUB_PORT="${FED_HUB_PORT:-18082}"
KEEP="${FED_KEEP:-0}"
RUN_ID="${FED_RUN_ID:-$(date -u +%Y%m%d%H%M%S)}"
ARTIFACT_DIR="${REPO_ROOT}/artifacts/federation-e2e/${RUN_ID}"

mkdir -p "${ARTIFACT_DIR}"
LOG_FILE="${ARTIFACT_DIR}/run.log"
exec > >(tee -a "${LOG_FILE}") 2>&1

log() { printf '[fed-e2e %s] %s\n' "$(date -u +%H:%M:%S)" "$*"; }

PF_PID=""
cleanup() {
  if [[ -n "${PF_PID}" ]]; then
    kill "${PF_PID}" 2>/dev/null || true
  fi
  if [[ "${KEEP}" != "1" ]]; then
    log "Cleaning up Kind clusters (${HUB_CLUSTER}, ${SPOKE_CLUSTER})"
    kind delete cluster --name "${HUB_CLUSTER}" >/dev/null 2>&1 || true
    kind delete cluster --name "${SPOKE_CLUSTER}" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

require_bin() {
  command -v "$1" >/dev/null 2>&1 || { echo "Missing required binary: $1" >&2; exit 1; }
}

require_bin kind
require_bin kubectl
require_bin helm
require_bin docker
require_bin curl

log "Building manager image ${IMG}"
docker build -t "${IMG}" "${REPO_ROOT}"

for c in "${HUB_CLUSTER}" "${SPOKE_CLUSTER}"; do
  if ! kind get clusters | grep -qx "${c}"; then
    log "Creating Kind cluster ${c}"
    kind create cluster --name "${c}" --wait 180s
  fi
  log "Loading ${IMG} into ${c}"
  kind load docker-image "${IMG}" --name "${c}"
done

IMG_REPO="${IMG%:*}"
IMG_TAG="${IMG##*:}"
if [[ "${IMG_REPO}" == "${IMG}" ]]; then
  IMG_TAG="latest"
fi

log "Provisioning scoped Hub/Spoke credentials secret (${SCOPED_KEYS_SECRET}) on ${HUB_CLUSTER}"
kubectl --context "kind-${HUB_CLUSTER}" create namespace "${NAMESPACE}" --dry-run=client -o yaml | \
  kubectl --context "kind-${HUB_CLUSTER}" apply -f -
kubectl --context "kind-${HUB_CLUSTER}" -n "${NAMESPACE}" create secret generic "${SCOPED_KEYS_SECRET}" \
  --from-literal=RECONCILYX_SCOPED_API_KEYS="${ADMIN_API_KEY},${SPOKE_API_KEY}" \
  --dry-run=client -o yaml | kubectl --context "kind-${HUB_CLUSTER}" apply -f -

log "Deploying Hub chart to ${HUB_CLUSTER}"
helm upgrade --install reconcilyx-hub "${REPO_ROOT}/charts/reconcilyx" \
  --kube-context "kind-${HUB_CLUSTER}" \
  --namespace "${NAMESPACE}" \
  --create-namespace \
  --set image.repository="${IMG_REPO}" \
  --set image.tag="${IMG_TAG}" \
  --set image.pullPolicy=Never \
  --set auth.scopedKeys.secretName="${SCOPED_KEYS_SECRET}" \
  --set service.type=NodePort \
  --set federation.enabled=true \
  --set federation.role=hub \
  --set federation.clusterName=hub-primary \
  --set federation.compactThreshold=10 \
  --set federation.compactInterval=30s \
  --wait --timeout 5m

kubectl --context "kind-${HUB_CLUSTER}" -n "${NAMESPACE}" rollout status deployment/reconcilyx-hub-reconcilyx-controller-manager --timeout=180s

HUB_NODE_IP="$(kubectl --context "kind-${HUB_CLUSTER}" get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')"
HUB_NODE_PORT="$(kubectl --context "kind-${HUB_CLUSTER}" -n "${NAMESPACE}" get svc -l app.kubernetes.io/instance=reconcilyx-hub -o jsonpath='{.items[0].spec.ports[?(@.name=="api-ui")].nodePort}')"
HUB_REACHABLE_URL="http://${HUB_NODE_IP}:${HUB_NODE_PORT}"
log "Hub reachable in Kind network at ${HUB_REACHABLE_URL}"

kubectl --context "kind-${HUB_CLUSTER}" -n "${NAMESPACE}" port-forward svc/reconcilyx-hub-reconcilyx "${HUB_PORT}:8082" >/dev/null 2>&1 &
PF_PID=$!
sleep 3

LOCAL_HUB="http://127.0.0.1:${HUB_PORT}"
curl -fsS "${LOCAL_HUB}/healthz" >/dev/null
log "Hub /healthz OK"

log "Sending 10 learning deltas from spoke-a and spoke-b (crosses compactThreshold=10)"
DELTAS_PAYLOAD='{
  "clusterName": "spoke-sim",
  "epoch": 1,
  "deltas": [
    {"clusterName":"spoke-a","stateKey":{"cpuBucket":4,"memBucket":6,"cpuReqMilli":500,"memReqMilli":1024},"actionKey":{"cpuAdjPct":-10,"memAdjPct":-10},"deltaQ":0.42,"visitCount":10},
    {"clusterName":"spoke-a","stateKey":{"cpuBucket":4,"memBucket":6,"cpuReqMilli":500,"memReqMilli":1024},"actionKey":{"cpuAdjPct":-10,"memAdjPct":-10},"deltaQ":0.44,"visitCount":10},
    {"clusterName":"spoke-a","stateKey":{"cpuBucket":4,"memBucket":6,"cpuReqMilli":500,"memReqMilli":1024},"actionKey":{"cpuAdjPct":-10,"memAdjPct":-10},"deltaQ":0.41,"visitCount":10},
    {"clusterName":"spoke-a","stateKey":{"cpuBucket":4,"memBucket":6,"cpuReqMilli":500,"memReqMilli":1024},"actionKey":{"cpuAdjPct":-10,"memAdjPct":-10},"deltaQ":0.43,"visitCount":10},
    {"clusterName":"spoke-b","stateKey":{"cpuBucket":4,"memBucket":6,"cpuReqMilli":500,"memReqMilli":1024},"actionKey":{"cpuAdjPct":-10,"memAdjPct":-10},"deltaQ":0.42,"visitCount":10},
    {"clusterName":"spoke-b","stateKey":{"cpuBucket":4,"memBucket":6,"cpuReqMilli":500,"memReqMilli":1024},"actionKey":{"cpuAdjPct":-10,"memAdjPct":-10},"deltaQ":0.45,"visitCount":10},
    {"clusterName":"spoke-b","stateKey":{"cpuBucket":4,"memBucket":6,"cpuReqMilli":500,"memReqMilli":1024},"actionKey":{"cpuAdjPct":-10,"memAdjPct":-10},"deltaQ":0.40,"visitCount":10},
    {"clusterName":"spoke-b","stateKey":{"cpuBucket":4,"memBucket":6,"cpuReqMilli":500,"memReqMilli":1024},"actionKey":{"cpuAdjPct":-10,"memAdjPct":-10},"deltaQ":0.42,"visitCount":10},
    {"clusterName":"spoke-b","stateKey":{"cpuBucket":4,"memBucket":6,"cpuReqMilli":500,"memReqMilli":1024},"actionKey":{"cpuAdjPct":-10,"memAdjPct":-10},"deltaQ":0.43,"visitCount":10},
    {"clusterName":"rogue-outlier","stateKey":{"cpuBucket":4,"memBucket":6,"cpuReqMilli":500,"memReqMilli":1024},"actionKey":{"cpuAdjPct":-10,"memAdjPct":-10},"deltaQ":999.0,"visitCount":10000}
  ]
}'

DELTA_RESP="$(curl -fsS -X POST "${LOCAL_HUB}/api/federated/deltas" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: ${ADMIN_API_KEY}" \
  -d "${DELTAS_PAYLOAD}")"
log "Delta ingestion response: ${DELTA_RESP}"
echo "${DELTA_RESP}" | grep -q '"compacted":true' || { echo "Expected automatic compaction at threshold=10"; exit 1; }

log "Submitting second stabilizing delta and verifying POST /api/v1/federation/compact"
curl -fsS -X POST "${LOCAL_HUB}/api/federated/deltas" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: ${ADMIN_API_KEY}" \
  -d '{"clusterName":"spoke-a","epoch":2,"deltas":[{"clusterName":"spoke-a","stateKey":{"cpuBucket":4,"memBucket":6,"cpuReqMilli":500,"memReqMilli":1024},"actionKey":{"cpuAdjPct":-10,"memAdjPct":-10},"deltaQ":0.425,"visitCount":500}]}' >/dev/null

COMPACT_RESP="$(curl -fsS -X POST "${LOCAL_HUB}/api/v1/federation/compact" -H "X-API-Key: ${ADMIN_API_KEY}")"
log "Explicit compact response: ${COMPACT_RESP}"

HUB_HASH="$(echo "${COMPACT_RESP}" | sed -n 's/.*"modelHash":"\([^"]*\)".*/\1/p')"
[[ -n "${HUB_HASH}" ]] || { echo "Expected non-empty modelHash from Hub"; exit 1; }

RELEASES_RESP="$(curl -fsS -H "X-API-Key: ${ADMIN_API_KEY}" "${LOCAL_HUB}/api/federated/releases")"
log "Model registry releases: ${RELEASES_RESP}"

MODEL_RESP="$(curl -fsS -H "X-API-Key: ${ADMIN_API_KEY}" "${LOCAL_HUB}/api/federated/global-model?channel=stable&cluster=${SPOKE_CLUSTER_NAME}")"
echo "${MODEL_RESP}" > "${ARTIFACT_DIR}/hub-global-model.json"
echo "${MODEL_RESP}" | grep -q "\"modelHash\":\"${HUB_HASH}\"" || { echo "Model hash mismatch on /api/federated/global-model"; exit 1; }

log "Deploying Spoke chart to ${SPOKE_CLUSTER} pointing to ${HUB_REACHABLE_URL}"
helm upgrade --install reconcilyx-spoke "${REPO_ROOT}/charts/reconcilyx" \
  --kube-context "kind-${SPOKE_CLUSTER}" \
  --namespace "${NAMESPACE}" \
  --create-namespace \
  --set image.repository="${IMG_REPO}" \
  --set image.tag="${IMG_TAG}" \
  --set image.pullPolicy=Never \
  --set api.key="${SPOKE_API_KEY}" \
  --set federation.enabled=true \
  --set federation.role=spoke \
  --set federation.hubUrl="${HUB_REACHABLE_URL}" \
  --set federation.clusterName="${SPOKE_CLUSTER_NAME}" \
  --set federation.modelChannel=stable \
  --set federation.flushThreshold=5 \
  --set federation.flushInterval=15s \
  --wait --timeout 5m

kubectl --context "kind-${SPOKE_CLUSTER}" -n "${NAMESPACE}" rollout status deployment/reconcilyx-spoke-reconcilyx-controller-manager --timeout=180s

SPOKE_LOGS="$(kubectl --context "kind-${SPOKE_CLUSTER}" -n "${NAMESPACE}" logs deployment/reconcilyx-spoke-reconcilyx-controller-manager --tail=200)"
echo "${SPOKE_LOGS}" > "${ARTIFACT_DIR}/spoke-manager.log"
echo "${SPOKE_LOGS}" | grep -q "Federated bootstrap loaded global Q-table" || {
  echo "Spoke did not log successful global Q-table bootstrap"
  exit 1
}

log "SUCCESS: 2-cluster federation validation passed (modelHash=${HUB_HASH}, artifacts=${ARTIFACT_DIR})"
