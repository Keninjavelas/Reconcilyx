#!/usr/bin/env bash
set -euo pipefail

CHART_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

helm lint "${CHART_DIR}"

# 1. Default standalone mode: federation disabled, read-only RBAC by default
STANDALONE="$(helm template rel "${CHART_DIR}")"
echo "${STANDALONE}" | grep -q 'RECONCILYX_FEDERATION_ENABLED: "false"'
echo "${STANDALONE}" | grep -q 'RECONCILYX_FEDERATION_ROLE: "standalone"'
echo "${STANDALONE}" | grep -q 'RECONCILYX_HUB_URL: ""'
echo "${STANDALONE}" | grep -q 'name: RECONCILYX_ENABLE_INPLACE'
echo "${STANDALONE}" | grep -A1 'name: RECONCILYX_ENABLE_INPLACE' | grep -q 'value: "false"'
echo "${STANDALONE}" | grep -q 'name: RECONCILYX_ALLOW_INPLACE_MEMORY_SHRINK'
echo "${STANDALONE}" | grep -A1 'name: RECONCILYX_ALLOW_INPLACE_MEMORY_SHRINK' | grep -q 'value: "false"'
echo "${STANDALONE}" | grep -q 'name: RECONCILYX_EXCLUDED_NAMESPACES'
echo "${STANDALONE}" | grep -q 'scaledobjects'
if echo "${STANDALONE}" | grep -q 'pods/resize'; then
  echo "Default read-only ClusterRole must not include pods/resize" >&2
  exit 1
fi

# 2. Explicit opt-in actuation grants only pods/resize patch
ACTUATE="$(helm template rel "${CHART_DIR}" --set actuation.inPlaceResizeEnabled=true --set actuation.allowInPlaceMemoryShrink=true --set config.enableInPlaceResizing=true)"
echo "${ACTUATE}" | grep -q 'pods/resize'
echo "${ACTUATE}" | grep -A1 'name: RECONCILYX_ENABLE_INPLACE' | grep -q 'value: "true"'
echo "${ACTUATE}" | grep -A1 'name: RECONCILYX_ALLOW_INPLACE_MEMORY_SHRINK' | grep -q 'value: "true"'

# 3. Spoke mode: custom thresholds & intervals render cleanly into ConfigMap and Deployment
SPOKE="$(helm template rel "${CHART_DIR}" \
  --set federation.enabled=true \
  --set federation.role=spoke \
  --set federation.hubUrl=https://hub.reconcilyx.internal:8082 \
  --set federation.clusterName=prod-eu-1 \
  --set federation.modelChannel=canary \
  --set federation.flushThreshold=5 \
  --set federation.flushInterval=30s \
  --set federation.compactThreshold=10 \
  --set federation.compactInterval=2m)"

for key in \
  'RECONCILYX_HUB_URL: "https://hub.reconcilyx.internal:8082"' \
  'RECONCILYX_CLUSTER_NAME: "prod-eu-1"' \
  'RECONCILYX_MODEL_CHANNEL: "canary"' \
  'RECONCILYX_FEDERATION_FLUSH_THRESHOLD: "5"' \
  'RECONCILYX_FEDERATION_FLUSH_INTERVAL: "30s"' \
  'RECONCILYX_HUB_COMPACT_THRESHOLD: "10"' \
  'RECONCILYX_HUB_COMPACT_INTERVAL: "2m"' \
  'name: RECONCILYX_HUB_URL' \
  'name: RECONCILYX_CLUSTER_NAME' \
  'name: RECONCILYX_MODEL_CHANNEL' \
  'name: RECONCILYX_FEDERATION_FLUSH_THRESHOLD' \
  'name: RECONCILYX_FEDERATION_FLUSH_INTERVAL' \
  'name: RECONCILYX_HUB_COMPACT_THRESHOLD' \
  'name: RECONCILYX_HUB_COMPACT_INTERVAL'; do
  echo "${SPOKE}" | grep -q "${key}" || { echo "Missing expected Helm output: ${key}" >&2; exit 1; }
done

ZERO_TRUST=$(helm template rel "${CHART_DIR}" \
  --set auth.oidc.issuerUrl="https://dex.corp.internal" \
  --set auth.oidc.clientId="reconcilyx-ui" \
  --set auth.oidc.githubRepos="org/infra" \
  --set externalSecrets.enabled=true \
  --set externalSecrets.githubSecretName="github-app-key" \
  --set externalSecrets.electricityMapsSecretName="emaps-key")

echo "${ZERO_TRUST}" | grep -q 'RECONCILYX_OIDC_ISSUER_URL'
echo "${ZERO_TRUST}" | grep -q 'RECONCILYX_GITHUB_OIDC_REPOS'
echo "${ZERO_TRUST}" | grep -q '/var/run/secrets/reconcilyx'
echo "${ZERO_TRUST}" | grep -q 'github/app_private_key.pem'

# 4. Enterprise SRE/HA Suite: PDB, HPA, NetworkPolicy, PrometheusRule, Digest pin, Helm test hook
OPS_HA="$(helm template rel "${CHART_DIR}" \
  --set replicaCount=2 \
  --set image.digest="sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" \
  --set pdb.enabled=true \
  --set autoscaling.enabled=true \
  --set networkPolicy.enabled=true \
  --set monitoring.prometheusRule.enabled=true)"

echo "${OPS_HA}" | grep -q 'kind: PodDisruptionBudget'
echo "${OPS_HA}" | grep -q 'kind: HorizontalPodAutoscaler'
echo "${OPS_HA}" | grep -q 'kind: NetworkPolicy'
echo "${OPS_HA}" | grep -q 'kind: PrometheusRule'
echo "${OPS_HA}" | grep -q 'ReconcilyxFallbackModeSustained'
echo "${OPS_HA}" | grep -q 'ReconcilyxSpokeSpoolBacklogCritical'
echo "${OPS_HA}" | grep -q 'ReconcilyxRetryStormDetected'
echo "${OPS_HA}" | grep -q 'ReconcilyxControllerPodCrashLoop'
echo "${OPS_HA}" | grep -q 'ReconcilyxAutomatedRollbacksSpike'
echo "${OPS_HA}" | grep -q '@sha256:0123456789abcdef'
echo "${OPS_HA}" | grep -q 'helm.sh/hook'

echo "Helm template federation & enterprise HA assertions passed."
