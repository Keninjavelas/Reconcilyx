#!/usr/bin/env bash
# End-to-end validation for the in-place resize lifecycle.
#
# The harness creates an isolated namespace with a two-container Deployment,
# pre-seeds live pod requests that differ from the Deployment template (using
# the operator's SSA field manager), approves a RightSize proposal through the
# Hub API, and waits for telemetry-backed stabilization.
#
# It fails unless all of these are observed:
#   - the execution snapshot came from live pods, not the Deployment template
#   - only the targeted app container was resized; the sidecar was preserved
#   - the pods were not recreated and no container restarted
#   - the learning receipt was finalized with the expected cost saving
#   - the policy evaluation, resize, and completion were written to the audit log
#
# Requirements: kubectl access, curl, a deployed manager (installed with
# `--set actuation.inPlaceResizeEnabled=true --set config.enableInPlaceResizing=true`
# when validating split-RBAC live resize), and Prometheus installed with
# `make install-observability` (or another metrics backend reachable through
# RECONCILYX_PROMETHEUS_URL).
#
# Usage:
#   scripts/validation/e2e.sh
#   E2E_KEEP=1 E2E_TIMEOUT=900 scripts/validation/e2e.sh
#
# Environment:
#   KUBECTL                       kubectl command (may include flags)
#   E2E_MANAGER_NAMESPACE         manager namespace (default reconcilyx-system)
#   E2E_MANAGER_DEPLOYMENT        manager deployment (default reconcilyx-controller-manager)
#   E2E_NAMESPACE                 namespace to create (default reconcilyx-e2e-<run id>)
#   E2E_RUN_ID                    artifact/run identifier (default UTC timestamp)
#   E2E_IMAGE                     container image for both fixture containers
#   E2E_TIMEOUT                   seconds to wait for Completed (default 600)
#   E2E_POLL_INTERVAL             seconds between status polls (default 10)
#   E2E_ARTIFACTS                 artifact root (default <repo>/artifacts/e2e)
#   E2E_KEEP=1                    keep the namespace after the run
#   E2E_API_URL                   Hub API base URL; otherwise port-forward svc/reconcilyx
#   E2E_API_PORT                  local port-forward port (default 18082)
#   E2E_API_KEY                   Hub API key; otherwise read from the manager env/secret
#   E2E_ALLOW_NO_PROMETHEUS=1     proceed even if the manager has no Prometheus URL
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

KUBECTL="${KUBECTL:-kubectl}"
MANAGER_NAMESPACE="${E2E_MANAGER_NAMESPACE:-reconcilyx-system}"
MANAGER_DEPLOYMENT="${E2E_MANAGER_DEPLOYMENT:-reconcilyx-controller-manager}"
RUN_ID="${E2E_RUN_ID:-$(date -u +%Y%m%d%H%M%S)}"
NAMESPACE="${E2E_NAMESPACE:-reconcilyx-e2e-${RUN_ID}}"
IMAGE="${E2E_IMAGE:-registry.k8s.io/pause:3.10}"
TIMEOUT="${E2E_TIMEOUT:-600}"
POLL="${E2E_POLL_INTERVAL:-10}"
KEEP="${E2E_KEEP:-0}"
ARTIFACT_ROOT="${E2E_ARTIFACTS:-${REPO_ROOT}/artifacts/e2e}"
ARTIFACT_DIR="${ARTIFACT_ROOT}/${RUN_ID}"

WORKLOAD="e2e-target"
POLICY="e2e-policy-${RUN_ID}"
PROPOSAL="e2e-learning-${RUN_ID}"

TEMPLATE_APP_CPU="${E2E_TEMPLATE_APP_CPU:-300m}"
TEMPLATE_APP_MEMORY="256Mi"
TEMPLATE_SIDECAR_CPU="${E2E_TEMPLATE_SIDECAR_CPU:-50m}"
TEMPLATE_SIDECAR_MEMORY="64Mi"
LIVE_CPU="250m"
LIVE_MEMORY="192Mi"
TARGET_APP_CPU="200m"
TARGET_APP_MEMORY="160Mi"
EXPECTED_COST_SAVED="4106m"

STARTED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
RESULTS="${ARTIFACT_DIR}/assertions.tsv"
CHECKS=0
FAILURES=0
NAMESPACE_CREATED=0
COLLECTED=0
PF_PID=""
API_KEY=""
API_KEY_SOURCE="none"
API_URL="${E2E_API_URL:-}"
API_URL="${API_URL%/}"

log() { printf '[e2e %s] %s\n' "$(date -u +%H:%M:%S)" "$*"; }

record_result() { printf '%s\t%s\t%s\t%s\n' "$1" "$2" "$3" "$4" >>"$RESULTS"; }

check_eq() {
	local name="$1" expected="$2" actual="$3"
	local expected_display="${expected//$'\n'/|}"
	local actual_display="${actual//$'\n'/|}"
	CHECKS=$((CHECKS + 1))
	if [[ "$expected" == "$actual" ]]; then
		log "PASS ${name}: ${actual_display}"
		record_result PASS "$name" "$expected_display" "$actual_display"
	else
		log "FAIL ${name}: expected '${expected_display}', got '${actual_display}'"
		record_result FAIL "$name" "$expected_display" "$actual_display"
		FAILURES=$((FAILURES + 1))
	fi
}

check_nonempty() {
	local name="$1" actual="$2"
	CHECKS=$((CHECKS + 1))
	if [[ -n "$actual" ]]; then
		log "PASS ${name}: present"
		record_result PASS "$name" "non-empty" "$actual"
	else
		log "FAIL ${name}: value is empty"
		record_result FAIL "$name" "non-empty" ""
		FAILURES=$((FAILURES + 1))
	fi
}

check_positive_number() {
	local name="$1" actual="$2"
	CHECKS=$((CHECKS + 1))
	if [[ -n "$actual" ]] && awk -v v="$actual" 'BEGIN { exit !(v + 0 > 0) }'; then
		log "PASS ${name}: ${actual}"
		record_result PASS "$name" "> 0" "$actual"
	else
		log "FAIL ${name}: expected a positive number, got '${actual}'"
		record_result FAIL "$name" "> 0" "$actual"
		FAILURES=$((FAILURES + 1))
	fi
}

check_contains() {
	local name="$1" file="$2" needle="$3"
	CHECKS=$((CHECKS + 1))
	if [[ -f "$file" ]] && grep -Fq -- "$needle" "$file"; then
		log "PASS ${name}: found '${needle}'"
		record_result PASS "$name" "$needle" "present"
	else
		log "FAIL ${name}: '${needle}' not found in ${file}"
		record_result FAIL "$name" "$needle" "missing"
		FAILURES=$((FAILURES + 1))
	fi
}

get_field() {
	$KUBECTL -n "$NAMESPACE" get optimizationproposal "$PROPOSAL" -o jsonpath="$1" 2>/dev/null
}

pod_unique() {
	$KUBECTL -n "$NAMESPACE" get pods -l "app=${WORKLOAD}" -o jsonpath="$1" 2>/dev/null |
		tr ' ' '\n' | sed '/^$/d' | sort -u | tr '\n' ',' | sed 's/,$//'
}

terminal_phase() {
	case "$1" in
	Failed | Rejected | Expired | RolledBack | RollbackRequired | RollingBack) return 0 ;;
	*) return 1 ;;
	esac
}

collect_artifacts() {
	((COLLECTED == 1)) && return 0
	COLLECTED=1
	mkdir -p "$ARTIFACT_DIR"
	((NAMESPACE_CREATED == 1)) || return 0

	$KUBECTL -n "$NAMESPACE" get optimizationproposal "$PROPOSAL" -o yaml >"$ARTIFACT_DIR/proposal.yaml" 2>&1 || true
	$KUBECTL -n "$NAMESPACE" get optimizationproposal "$PROPOSAL" -o jsonpath='{.status.executionSnapshot}' >"$ARTIFACT_DIR/execution-snapshot.json" 2>/dev/null || true
	$KUBECTL -n "$NAMESPACE" get optimizationproposal "$PROPOSAL" -o jsonpath='{.status.learning}' >"$ARTIFACT_DIR/receipt.json" 2>/dev/null || true
	$KUBECTL -n "$NAMESPACE" get pods -l "app=${WORKLOAD}" -o yaml >"$ARTIFACT_DIR/pods.yaml" 2>&1 || true
	$KUBECTL -n "$NAMESPACE" get deployment "$WORKLOAD" -o yaml >"$ARTIFACT_DIR/deployment.yaml" 2>&1 || true
	$KUBECTL -n "$NAMESPACE" get events --sort-by=.lastTimestamp >"$ARTIFACT_DIR/events.txt" 2>&1 || true
	$KUBECTL -n "$NAMESPACE" get configmap reconcilyx-audit -o go-template='{{index .data "audit.jsonl"}}' >"$ARTIFACT_DIR/audit.jsonl" 2>/dev/null || true
	if [[ -s "$ARTIFACT_DIR/audit.jsonl" ]]; then
		grep -F "\"proposal\":\"${PROPOSAL}\"" "$ARTIFACT_DIR/audit.jsonl" >"$ARTIFACT_DIR/audit-proposal.jsonl" || true
	else
		: >"$ARTIFACT_DIR/audit-proposal.jsonl"
	fi
	$KUBECTL -n "$MANAGER_NAMESPACE" logs "deployment/${MANAGER_DEPLOYMENT}" --tail=500 >"$ARTIFACT_DIR/manager.log" 2>&1 || true
}

finish() {
	collect_artifacts
	{
		echo "# Reconcilyx end-to-end validation"
		echo "run_id: ${RUN_ID}"
		echo "namespace: ${NAMESPACE}"
		echo "proposal: ${PROPOSAL}"
		echo "image: ${IMAGE}"
		echo "started_at_utc: ${STARTED_AT}"
		echo "finished_at_utc: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
		echo "api_key_source: ${API_KEY_SOURCE}"
		echo "checks: ${CHECKS}"
		echo "failures: ${FAILURES}"
		echo "artifacts: ${ARTIFACT_DIR}"
		echo
		cat "$RESULTS"
	} >"$ARTIFACT_DIR/summary.txt"
	cat "$ARTIFACT_DIR/summary.txt"

	if ((FAILURES > 0)); then
		log "RESULT: FAIL (${FAILURES} failed check(s))"
		exit 1
	fi
	log "RESULT: PASS (${CHECKS} checks)"
	exit 0
}

fatal() {
	CHECKS=$((CHECKS + 1))
	FAILURES=$((FAILURES + 1))
	log "FATAL: $*"
	record_result FAIL "fatal" "successful run" "$*"
	finish
}

cleanup() {
	if [[ -n "$PF_PID" ]]; then
		kill "$PF_PID" >/dev/null 2>&1 || true
	fi
	if ((NAMESPACE_CREATED == 1)); then
		if [[ "$KEEP" == "1" ]]; then
			log "Keeping namespace ${NAMESPACE} (E2E_KEEP=1)"
		else
			log "Deleting namespace ${NAMESPACE}"
			$KUBECTL delete namespace "$NAMESPACE" --wait=false --ignore-not-found >/dev/null 2>&1 || true
		fi
	fi
}
trap cleanup EXIT

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
	sed -n '2,45p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
	exit 0
fi

mkdir -p "$ARTIFACT_DIR"
: >"$RESULTS"
{
	echo "run_id=${RUN_ID}"
	echo "namespace=${NAMESPACE}"
	echo "manager_namespace=${MANAGER_NAMESPACE}"
	echo "manager_deployment=${MANAGER_DEPLOYMENT}"
	echo "image=${IMAGE}"
	echo "timeout=${TIMEOUT}"
} >"$ARTIFACT_DIR/config.txt"

log "Starting end-to-end validation run ${RUN_ID}"
log "Artifacts: ${ARTIFACT_DIR}"

# ---------------------------------------------------------------------------
# Preflight
# ---------------------------------------------------------------------------
command -v curl >/dev/null 2>&1 || fatal "curl is required"
command -v awk >/dev/null 2>&1 || fatal "awk is required"
command -v grep >/dev/null 2>&1 || fatal "grep is required"
$KUBECTL get crd optimizationproposals.optimization.reconcilyx.io >/dev/null 2>&1 ||
	fatal "OptimizationProposal CRD not found; is the operator installed?"
$KUBECTL -n "$MANAGER_NAMESPACE" get deployment "$MANAGER_DEPLOYMENT" >/dev/null 2>&1 ||
	fatal "manager deployment ${MANAGER_NAMESPACE}/${MANAGER_DEPLOYMENT} not found"

MANAGER_SA=$($KUBECTL -n "$MANAGER_NAMESPACE" get deployment "$MANAGER_DEPLOYMENT" \
	-o jsonpath='{.spec.template.spec.serviceAccountName}' 2>/dev/null)
MANAGER_SA="${MANAGER_SA:-default}"
if ! $KUBECTL auth can-i patch pods --subresource=resize --as="system:serviceaccount:${MANAGER_NAMESPACE}:${MANAGER_SA}" >/dev/null 2>&1; then
	log "WARN ServiceAccount ${MANAGER_NAMESPACE}/${MANAGER_SA} lacks pods/resize patch RBAC; re-deploy Helm with --set actuation.inPlaceResizeEnabled=true --set config.enableInPlaceResizing=true"
fi

PROMETHEUS_URL=$($KUBECTL -n "$MANAGER_NAMESPACE" get deployment "$MANAGER_DEPLOYMENT" \
	-o jsonpath='{range .spec.template.spec.containers[0].env[?(@.name=="RECONCILYX_PROMETHEUS_URL")]}{.value}{end}' 2>/dev/null)
if [[ -z "$PROMETHEUS_URL" && "${E2E_ALLOW_NO_PROMETHEUS:-0}" != "1" ]]; then
	fatal "manager has no RECONCILYX_PROMETHEUS_URL; stabilization requires Prometheus. Run 'make install-observability' or set E2E_ALLOW_NO_PROMETHEUS=1."
fi
log "Prometheus URL: ${PROMETHEUS_URL:-<not configured>}"

if [[ -z "$API_KEY" && -n "${E2E_API_KEY:-}" ]]; then
	API_KEY="$E2E_API_KEY"
	API_KEY_SOURCE="explicit"
fi
if [[ -z "$API_KEY" ]]; then
	API_KEY=$($KUBECTL -n "$MANAGER_NAMESPACE" get deployment "$MANAGER_DEPLOYMENT" \
		-o jsonpath='{range .spec.template.spec.containers[0].env[?(@.name=="RECONCILYX_API_KEY")]}{.value}{end}' 2>/dev/null)
	[[ -n "$API_KEY" ]] && API_KEY_SOURCE="manager-env"
fi
if [[ -z "$API_KEY" ]]; then
	KEY_SECRET=$($KUBECTL -n "$MANAGER_NAMESPACE" get deployment "$MANAGER_DEPLOYMENT" \
		-o jsonpath='{range .spec.template.spec.containers[0].env[?(@.name=="RECONCILYX_API_KEY")]}{.valueFrom.secretKeyRef.name}{end}' 2>/dev/null)
	KEY_FIELD=$($KUBECTL -n "$MANAGER_NAMESPACE" get deployment "$MANAGER_DEPLOYMENT" \
		-o jsonpath='{range .spec.template.spec.containers[0].env[?(@.name=="RECONCILYX_API_KEY")]}{.valueFrom.secretKeyRef.key}{end}' 2>/dev/null)
	if [[ -n "$KEY_SECRET" && -n "$KEY_FIELD" ]]; then
		API_KEY=$($KUBECTL -n "$MANAGER_NAMESPACE" get secret "$KEY_SECRET" \
			-o jsonpath="{.data.${KEY_FIELD}}" 2>/dev/null | base64 -d 2>/dev/null)
		[[ -n "$API_KEY" ]] && API_KEY_SOURCE="manager-secret"
	fi
fi
[[ -z "$API_KEY" ]] && API_KEY_SOURCE="none (auth disabled)"

if [[ -z "$API_URL" ]]; then
	command -v "$(echo "$KUBECTL" | awk '{print $1}')" >/dev/null 2>&1 || fatal "kubectl not found"
	API_PORT="${E2E_API_PORT:-18082}"
	API_URL="http://127.0.0.1:${API_PORT}"
	log "Starting port-forward to svc/reconcilyx ${API_PORT}:8082"
	$KUBECTL -n "$MANAGER_NAMESPACE" port-forward svc/reconcilyx "${API_PORT}:8082" \
		>"$ARTIFACT_DIR/port-forward.log" 2>&1 &
	PF_PID=$!
	ready=0
	for _ in $(seq 1 30); do
		if curl -sf --max-time 2 "${API_URL}/healthz" >/dev/null 2>&1; then
			ready=1
			break
		fi
		sleep 1
	done
	((ready == 1)) || fatal "Hub API did not become reachable at ${API_URL}"
fi

curl -sf --max-time 5 "${API_URL}/healthz" >/dev/null 2>&1 ||
	fatal "Hub API health check failed at ${API_URL}/healthz"
log "Hub API: ${API_URL} (key source: ${API_KEY_SOURCE})"

# Optional Prometheus readiness probe through the service proxy. A harness RBAC
# failure is reported but not fatal; the manager's own telemetry is independent.
PROM_PROXY_BASE=""
if [[ "$PROMETHEUS_URL" =~ ^http://([a-z0-9-]+)\.([a-z0-9-]+)\.svc(:[0-9]+)?/?$ ]]; then
	prom_service="${BASH_REMATCH[1]}"
	prom_namespace="${BASH_REMATCH[2]}"
	prom_port="${BASH_REMATCH[3]#:}"
	prom_port="${prom_port:-80}"
	PROM_PROXY_BASE="/api/v1/namespaces/${prom_namespace}/services/${prom_service}:${prom_port}/proxy"
	if prom_body=$($KUBECTL get --raw "${PROM_PROXY_BASE}/-/ready" 2>/dev/null) &&
		[[ "$prom_body" == *"Prometheus Server is Ready"* ]]; then
		log "Prometheus readiness probe: ready"
	else
		log "WARN Prometheus readiness probe unavailable; stabilization may time out"
	fi
fi

urlencode() {
	local input="$1" output="" char i
	for ((i = 0; i < ${#input}; i++)); do
		char="${input:i:1}"
		case "$char" in
		[a-zA-Z0-9.~_-]) output+="$char" ;;
		*) output+=$(printf '%%%02X' "'${char}") ;;
		esac
	done
	printf '%s' "$output"
}

if $KUBECTL get namespace "$NAMESPACE" >/dev/null 2>&1; then
	fatal "namespace ${NAMESPACE} already exists; choose a fresh E2E_NAMESPACE or E2E_RUN_ID"
fi

# ---------------------------------------------------------------------------
# Fixture
# ---------------------------------------------------------------------------
log "Creating namespace ${NAMESPACE}"
if ! $KUBECTL apply -f - >"$ARTIFACT_DIR/namespace-apply.log" 2>&1 <<EOF
apiVersion: v1
kind: Namespace
metadata:
  name: ${NAMESPACE}
  labels:
    app.kubernetes.io/part-of: reconcilyx-e2e
    reconcilyx.io/e2e: "true"
    reconcilyx.io/e2e-run: "${RUN_ID}"
EOF
then
	fatal "could not create namespace ${NAMESPACE}"
fi
NAMESPACE_CREATED=1

deadline=$((SECONDS + 60))
until $KUBECTL -n "$NAMESPACE" get serviceaccount default >/dev/null 2>&1; do
	((SECONDS < deadline)) || fatal "namespace ${NAMESPACE} did not initialize"
	sleep 2
done

log "Deploying two-container fixture ${WORKLOAD}"
if ! $KUBECTL apply -f - >"$ARTIFACT_DIR/deployment-apply.log" 2>&1 <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${WORKLOAD}
  namespace: ${NAMESPACE}
  labels:
    app: ${WORKLOAD}
    reconcilyx.io/e2e: "true"
spec:
  replicas: 2
  selector:
    matchLabels:
      app: ${WORKLOAD}
  template:
    metadata:
      labels:
        app: ${WORKLOAD}
    spec:
      containers:
        - name: app
          image: ${IMAGE}
          resources:
            requests:
              cpu: ${TEMPLATE_APP_CPU}
              memory: ${TEMPLATE_APP_MEMORY}
            limits:
              cpu: "1"
              memory: 512Mi
        - name: istio-proxy
          image: ${IMAGE}
          resources:
            requests:
              cpu: ${TEMPLATE_SIDECAR_CPU}
              memory: ${TEMPLATE_SIDECAR_MEMORY}
            limits:
              cpu: 500m
              memory: 256Mi
EOF
then
	fatal "could not create fixture deployment"
fi

if ! $KUBECTL -n "$NAMESPACE" rollout status "deployment/${WORKLOAD}" --timeout=180s >"$ARTIFACT_DIR/rollout.log" 2>&1; then
	{
		echo "=== PODS ==="
		$KUBECTL get pods -A -o wide 2>&1 || true
		echo "=== DESCRIBE ==="
		$KUBECTL -n "$NAMESPACE" describe pod -l "app=${WORKLOAD}" 2>&1 || true
		echo "=== EVENTS ==="
		$KUBECTL -n "$NAMESPACE" get events --sort-by=.lastTimestamp 2>&1 || true
	} >>"$ARTIFACT_DIR/rollout.log"
	fatal "fixture deployment did not become available"
fi

# Pre-seed live pod requests with the operator's field manager. This reproduces
# the state that the snapshot must read: pods diverge from the template, and the
# operator co-owns the resize fields.
log "Pre-seeding live pod requests (${LIVE_CPU}/${LIVE_MEMORY}) as reconciler-owned state"
for pod in $($KUBECTL -n "$NAMESPACE" get pods -l "app=${WORKLOAD}" -o jsonpath='{.items[*].metadata.name}'); do
	if ! $KUBECTL apply --server-side --force-conflicts --subresource=resize \
		--field-manager=reconcilyx-operator -f - >/dev/null 2>&1 <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: ${pod}
  namespace: ${NAMESPACE}
spec:
  containers:
    - name: app
      resources:
        requests:
          cpu: ${LIVE_CPU}
          memory: ${LIVE_MEMORY}
    - name: istio-proxy
      resources:
        requests:
          cpu: ${LIVE_CPU}
          memory: ${LIVE_MEMORY}
EOF
	then
		fatal "could not pre-seed live resources for pod ${pod}"
	fi
done

deadline=$((SECONDS + 120))
while ((SECONDS < deadline)); do
	live_app_cpu=$(pod_unique '{.items[*].spec.containers[?(@.name=="app")].resources.requests.cpu}')
	live_app_memory=$(pod_unique '{.items[*].spec.containers[?(@.name=="app")].resources.requests.memory}')
	live_sidecar_cpu=$(pod_unique '{.items[*].spec.containers[?(@.name=="istio-proxy")].resources.requests.cpu}')
	live_sidecar_memory=$(pod_unique '{.items[*].spec.containers[?(@.name=="istio-proxy")].resources.requests.memory}')
	live_ready=$(pod_unique '{.items[*].status.containerStatuses[*].ready}')
	if [[ "$live_app_cpu" == "$LIVE_CPU" && "$live_app_memory" == "$LIVE_MEMORY" &&
		"$live_sidecar_cpu" == "$LIVE_CPU" && "$live_sidecar_memory" == "$LIVE_MEMORY" &&
		"$live_ready" == "true" ]]; then
		break
	fi
	sleep 3
done
[[ "$live_ready" == "true" ]] || fatal "fixture pods did not become ready after pre-seed"

check_eq "pre-seed app CPU request" "$LIVE_CPU" "$live_app_cpu"
check_eq "pre-seed app memory request" "$LIVE_MEMORY" "$live_app_memory"
check_eq "pre-seed sidecar CPU request" "$LIVE_CPU" "$live_sidecar_cpu"
check_eq "pre-seed sidecar memory request" "$LIVE_MEMORY" "$live_sidecar_memory"
check_eq "pre-seed pods ready" "true" "$live_ready"

managed_manager=$(pod_unique '{.items[*].metadata.managedFields[?(@.manager=="reconcilyx-operator")].manager}')
managed_subresource=$(pod_unique '{.items[*].metadata.managedFields[?(@.manager=="reconcilyx-operator")].subresource}')
check_eq "pre-seed ownership field manager" "reconcilyx-operator" "$managed_manager"
check_eq "pre-seed ownership subresource" "resize" "$managed_subresource"

PRE_UIDS=$($KUBECTL -n "$NAMESPACE" get pods -l "app=${WORKLOAD}" -o jsonpath='{range .items[*]}{.metadata.uid}{"\n"}{end}' | sort)
PRE_RESTARTS=$(pod_unique '{.items[*].status.containerStatuses[*].restartCount}')
check_eq "pre-seed container restarts" "0" "$PRE_RESTARTS"

template_app_cpu=$($KUBECTL -n "$NAMESPACE" get deployment "$WORKLOAD" -o jsonpath='{.spec.template.spec.containers[?(@.name=="app")].resources.requests.cpu}')
template_app_memory=$($KUBECTL -n "$NAMESPACE" get deployment "$WORKLOAD" -o jsonpath='{.spec.template.spec.containers[?(@.name=="app")].resources.requests.memory}')
template_sidecar_cpu=$($KUBECTL -n "$NAMESPACE" get deployment "$WORKLOAD" -o jsonpath='{.spec.template.spec.containers[?(@.name=="istio-proxy")].resources.requests.cpu}')
template_sidecar_memory=$($KUBECTL -n "$NAMESPACE" get deployment "$WORKLOAD" -o jsonpath='{.spec.template.spec.containers[?(@.name=="istio-proxy")].resources.requests.memory}')
check_eq "fixture template app CPU (diverges from pods)" "$TEMPLATE_APP_CPU" "$template_app_cpu"
check_eq "fixture template app memory (diverges from pods)" "$TEMPLATE_APP_MEMORY" "$template_app_memory"
check_eq "fixture template sidecar CPU (diverges from pods)" "$TEMPLATE_SIDECAR_CPU" "$template_sidecar_cpu"
check_eq "fixture template sidecar memory (diverges from pods)" "$TEMPLATE_SIDECAR_MEMORY" "$template_sidecar_memory"

# Wait until Prometheus has scrape data for the fixture. The learning receipt
# captures the pre-actuation state immediately after approval; without a
# sample, zero-valued fields are omitted and the receipt cannot be verified.
if [[ -n "$PROM_PROXY_BASE" ]]; then
	log "Waiting for Prometheus to observe the fixture pods"
	prom_query=$(urlencode "container_memory_working_set_bytes{namespace=\"${NAMESPACE}\",container=\"app\"}")
	deadline=$((SECONDS + 120))
	samples_ready=0
	while ((SECONDS < deadline)); do
		prom_result=$($KUBECTL get --raw "${PROM_PROXY_BASE}/api/v1/query?query=${prom_query}" 2>/dev/null)
		if [[ -n "$prom_result" && "$prom_result" != *'"result":[]'* && "$prom_result" == *'"result":'* ]]; then
			samples_ready=1
			break
		fi
		sleep 5
	done
	check_eq "Prometheus observed the fixture before approval" "1" "$samples_ready"
	((samples_ready == 1)) || fatal "Prometheus returned no container_memory_working_set_bytes for ${NAMESPACE} within 120s"
else
	log "WARN Prometheus service proxy unavailable; skipping fixture sample wait"
fi

# ---------------------------------------------------------------------------
# Policy and proposal
# ---------------------------------------------------------------------------
log "Applying label-scoped OptimizationPolicy ${POLICY}"
if ! $KUBECTL apply -f - >"$ARTIFACT_DIR/policy-apply.log" 2>&1 <<EOF
apiVersion: optimization.reconcilyx.io/v1alpha1
kind: OptimizationPolicy
metadata:
  name: ${POLICY}
  namespace: ${NAMESPACE}
spec:
  objective: Efficiency
  mode: Propose
  targetSelectors:
    namespaceSelector:
      matchLabels:
        reconcilyx.io/e2e-run: "${RUN_ID}"
    workloadSelector:
      matchLabels:
        app: ${WORKLOAD}
  safetyConstraints:
    riskTolerance: 40
    confidenceRequirement: 70
    cooldownPeriod: 30m
    minReplicas: 2
  recommendationGovernance:
    allowedRecommendations:
      - RightSize
EOF
then
	fatal "could not create OptimizationPolicy"
fi

deadline=$((SECONDS + 60))
policy_validated=""
while ((SECONDS < deadline)); do
	policy_validated=$($KUBECTL -n "$NAMESPACE" get optimizationpolicy "$POLICY" \
		-o jsonpath='{.status.conditions[?(@.type=="Validated")].status}' 2>/dev/null)
	[[ "$policy_validated" == "True" ]] && break
	sleep 2
done
check_eq "policy validated" "True" "$policy_validated"

log "Creating RightSize proposal ${PROPOSAL}"
if ! $KUBECTL apply -f - >"$ARTIFACT_DIR/proposal-apply.log" 2>&1 <<EOF
apiVersion: optimization.reconcilyx.io/v1alpha1
kind: OptimizationProposal
metadata:
  name: ${PROPOSAL}
  namespace: ${NAMESPACE}
spec:
  targetWorkload:
    apiVersion: apps/v1
    kind: Deployment
    name: ${WORKLOAD}
    namespace: ${NAMESPACE}
  recommendation: RightSize
  proposedResources:
    requests:
      cpu: ${TARGET_APP_CPU}
      memory: ${TARGET_APP_MEMORY}
  proposedResourcesByContainer:
    app:
      requests:
        cpu: ${TARGET_APP_CPU}
        memory: ${TARGET_APP_MEMORY}
  decisionMetadata:
    confidenceScore: 86
    riskScore: 18
    rationale: "Scripted end-to-end validation ${RUN_ID}"
EOF
then
	fatal "could not create OptimizationProposal"
fi

deadline=$((SECONDS + 120))
evaluation_time=""
while ((SECONDS < deadline)); do
	evaluation_time=$(get_field '{.status.lastEvaluationTime}')
	[[ -n "$evaluation_time" ]] && break
	phase=$(get_field '{.status.phase}')
	terminal_phase "$phase" && fatal "proposal entered ${phase} before evaluation: $(get_field '{.status.reason}')"
	sleep 3
done
phase=$(get_field '{.status.phase}')
approval_state=$(get_field '{.status.approvalState}')
check_nonempty "proposal evaluated" "$evaluation_time"
check_eq "policy decision is Pending" "Pending" "$approval_state"
if terminal_phase "$phase"; then
	fatal "proposal entered ${phase} during evaluation: $(get_field '{.status.reason}')"
fi

# ---------------------------------------------------------------------------
# Approve through the Hub API
# ---------------------------------------------------------------------------
log "Approving ${PROPOSAL} through the Hub API"
approve_args=(-sS --max-time 20 -X POST "${API_URL}/api/proposals/${PROPOSAL}/approve?namespace=${NAMESPACE}" \
	-H 'Content-Type: application/json' \
	-d "{\"actor\":\"e2e-harness\",\"reason\":\"Automated end-to-end validation ${RUN_ID}\"}")
[[ -n "$API_KEY" ]] && approve_args+=(-H "X-API-Key: ${API_KEY}")
approve_raw=$(curl "${approve_args[@]}" -w '\n%{http_code}' 2>"$ARTIFACT_DIR/api-approve.err")
approve_code="${approve_raw##*$'\n'}"
approve_body="${approve_raw%$'\n'*}"
printf '%s\n' "$approve_body" >"$ARTIFACT_DIR/api-approve.json"
check_eq "approve HTTP status" "200" "$approve_code"
check_contains "approve response" "$ARTIFACT_DIR/api-approve.json" '"status":"ok"'
if [[ "$approve_code" != "200" ]]; then
	fatal "Hub approval failed with HTTP ${approve_code}"
fi

# ---------------------------------------------------------------------------
# Snapshot assertions
# ---------------------------------------------------------------------------
log "Waiting for the execution snapshot"
deadline=$((SECONDS + 180))
snapshot_app_cpu=""
while ((SECONDS < deadline)); do
	snapshot_app_cpu=$(get_field '{.status.executionSnapshot.containerResources.app.resources.requests.cpu}')
	[[ -n "$snapshot_app_cpu" ]] && break
	phase=$(get_field '{.status.phase}')
	if terminal_phase "$phase"; then
		fatal "proposal entered ${phase} before capturing a snapshot: $(get_field '{.status.reason}')"
	fi
	sleep 3
done
[[ -n "$snapshot_app_cpu" ]] || fatal "proposal did not capture an execution snapshot"

snapshot_app_memory=$(get_field '{.status.executionSnapshot.containerResources.app.resources.requests.memory}')
snapshot_sidecar_cpu=$(get_field '{.status.executionSnapshot.containerResources.istio-proxy.resources.requests.cpu}')
snapshot_sidecar_memory=$(get_field '{.status.executionSnapshot.containerResources.istio-proxy.resources.requests.memory}')
snapshot_replicas=$(get_field '{.status.executionSnapshot.replicaCount}')
snapshot_timestamp=$(get_field '{.status.executionSnapshot.timestamp}')

check_eq "snapshot app CPU came from live pods" "$LIVE_CPU" "$snapshot_app_cpu"
check_eq "snapshot app memory came from live pods" "$LIVE_MEMORY" "$snapshot_app_memory"
check_eq "snapshot sidecar CPU came from live pods" "$LIVE_CPU" "$snapshot_sidecar_cpu"
check_eq "snapshot sidecar memory came from live pods" "$LIVE_MEMORY" "$snapshot_sidecar_memory"
check_eq "snapshot replica count" "2" "$snapshot_replicas"
check_nonempty "snapshot timestamp" "$snapshot_timestamp"

# ---------------------------------------------------------------------------
# Completion
# ---------------------------------------------------------------------------
log "Waiting up to ${TIMEOUT}s for stabilization to complete"
deadline=$((SECONDS + TIMEOUT))
phase=""
last_phase=""
reason=""
while ((SECONDS < deadline)); do
	phase=$(get_field '{.status.phase}')
	reason=$(get_field '{.status.reason}')
	if [[ "$phase" != "$last_phase" ]]; then
		log "phase: ${phase}"
		last_phase="$phase"
	fi
	[[ "$phase" == "Completed" ]] && break
	if terminal_phase "$phase"; then
		fatal "proposal entered ${phase}: ${reason}"
	fi
	sleep "$POLL"
done
check_eq "proposal phase" "Completed" "$phase"
[[ "$phase" == "Completed" ]] || fatal "timed out after ${TIMEOUT}s waiting for Completed (last phase: ${phase}, reason: ${reason})"

deadline=$((SECONDS + 30))
while ((SECONDS < deadline)); do
	if $KUBECTL -n "$NAMESPACE" get configmap reconcilyx-audit \
		-o go-template='{{index .data "audit.jsonl"}}' 2>/dev/null |
		grep -F "\"proposal\":\"${PROPOSAL}\"" | grep -Fq "completed: workload stabilized"; then
		break
	fi
	sleep 2
done

collect_artifacts

# ---------------------------------------------------------------------------
# Post-execution assertions
# ---------------------------------------------------------------------------
post_app_cpu=$(pod_unique '{.items[*].spec.containers[?(@.name=="app")].resources.requests.cpu}')
post_app_memory=$(pod_unique '{.items[*].spec.containers[?(@.name=="app")].resources.requests.memory}')
post_sidecar_cpu=$(pod_unique '{.items[*].spec.containers[?(@.name=="istio-proxy")].resources.requests.cpu}')
post_sidecar_memory=$(pod_unique '{.items[*].spec.containers[?(@.name=="istio-proxy")].resources.requests.memory}')
post_app_limit_cpu=$(pod_unique '{.items[*].spec.containers[?(@.name=="app")].resources.limits.cpu}')
post_app_limit_memory=$(pod_unique '{.items[*].spec.containers[?(@.name=="app")].resources.limits.memory}')
post_sidecar_limit_cpu=$(pod_unique '{.items[*].spec.containers[?(@.name=="istio-proxy")].resources.limits.cpu}')
post_sidecar_limit_memory=$(pod_unique '{.items[*].spec.containers[?(@.name=="istio-proxy")].resources.limits.memory}')
post_ready=$(pod_unique '{.items[*].status.containerStatuses[*].ready}')
post_restarts=$(pod_unique '{.items[*].status.containerStatuses[*].restartCount}')
post_names=$(pod_unique '{.items[*].spec.containers[*].name}')
post_container_count=$($KUBECTL -n "$NAMESPACE" get pods -l "app=${WORKLOAD}" \
	-o jsonpath='{range .items[*]}{range .spec.containers[*]}{.name}{"\n"}{end}{end}' | sed '/^$/d' | wc -l | tr -d ' ')
POST_UIDS=$($KUBECTL -n "$NAMESPACE" get pods -l "app=${WORKLOAD}" -o jsonpath='{range .items[*]}{.metadata.uid}{"\n"}{end}' | sort)

check_eq "app live CPU request after resize" "$TARGET_APP_CPU" "$post_app_cpu"
check_eq "app live memory request after resize" "$TARGET_APP_MEMORY" "$post_app_memory"
check_eq "sidecar live CPU request preserved" "$LIVE_CPU" "$post_sidecar_cpu"
check_eq "sidecar live memory request preserved" "$LIVE_MEMORY" "$post_sidecar_memory"
check_eq "app CPU limit untouched" "1" "$post_app_limit_cpu"
check_eq "app memory limit untouched" "512Mi" "$post_app_limit_memory"
check_eq "sidecar CPU limit untouched" "500m" "$post_sidecar_limit_cpu"
check_eq "sidecar memory limit untouched" "256Mi" "$post_sidecar_limit_memory"
check_eq "all containers ready" "true" "$post_ready"
check_eq "container restarts after resize" "0" "$post_restarts"
check_eq "pre-existing restart count preserved" "$PRE_RESTARTS" "$post_restarts"
check_eq "container set preserved" "app,istio-proxy" "$post_names"
check_eq "container count per pod pair" "4" "$post_container_count"
check_eq "pod UIDs unchanged (no pod recreation)" "$PRE_UIDS" "$POST_UIDS"

post_template_app_cpu=$($KUBECTL -n "$NAMESPACE" get deployment "$WORKLOAD" -o jsonpath='{.spec.template.spec.containers[?(@.name=="app")].resources.requests.cpu}')
post_template_sidecar_cpu=$($KUBECTL -n "$NAMESPACE" get deployment "$WORKLOAD" -o jsonpath='{.spec.template.spec.containers[?(@.name=="istio-proxy")].resources.requests.cpu}')
check_eq "Deployment template app request unchanged" "$TEMPLATE_APP_CPU" "$post_template_app_cpu"
check_eq "Deployment template sidecar request unchanged" "$TEMPLATE_SIDECAR_CPU" "$post_template_sidecar_cpu"

execution_state=$(get_field '{.status.executionState}')
approval_state=$(get_field '{.status.approvalState}')
policy_applied=$(get_field '{.status.policyApplied}')
check_eq "execution state" "Verified" "$execution_state"
check_eq "approval state" "Approved" "$approval_state"
check_eq "matched policy" "$POLICY" "$policy_applied"

# Learning receipt
learning_finalized=$(get_field '{.status.learning.finalized}')
learning_outcome=$(get_field '{.status.learning.outcome}')
learning_completed_at=$(get_field '{.status.learning.completedAt}')
learning_reward=$(get_field '{.status.learning.reward}')
app_cost_saved=$(get_field '{.status.learning.results.app.costSavedUSD}')
app_carbon_saved=$(get_field '{.status.learning.results.app.carbonSavedGramsCO2}')
app_cpu_adjustment=$(get_field '{.status.learning.results.app.cpuAdjustmentPercent}')
app_memory_adjustment=$(get_field '{.status.learning.results.app.memoryAdjustmentPercent}')
sidecar_cost_saved=$(get_field '{.status.learning.results.istio-proxy.costSavedUSD}')
sidecar_carbon_saved=$(get_field '{.status.learning.results.istio-proxy.carbonSavedGramsCO2}')
pre_app_memory=$(get_field '{.status.learning.pre.app.memoryWorkingSetBytes}')
post_app_memory=$(get_field '{.status.learning.results.app.post.memoryWorkingSetBytes}')
pre_sidecar_memory=$(get_field '{.status.learning.pre.istio-proxy.memoryWorkingSetBytes}')

check_eq "learning receipt finalized" "true" "$learning_finalized"
check_eq "learning outcome" "completed" "$learning_outcome"
check_nonempty "learning completedAt" "$learning_completed_at"
check_positive_number "learning reward" "$learning_reward"
check_eq "app cost saved" "$EXPECTED_COST_SAVED" "$app_cost_saved"
check_positive_number "app carbon saved" "$app_carbon_saved"
check_eq "app CPU adjustment percent" "-20" "$app_cpu_adjustment"
check_eq "app memory adjustment percent" "-20" "$app_memory_adjustment"
check_eq "sidecar cost saved" "0" "$sidecar_cost_saved"
check_eq "sidecar carbon saved" "0" "$sidecar_carbon_saved"
check_positive_number "pre-resize app memory working set" "$pre_app_memory"
check_positive_number "post-resize app memory working set" "$post_app_memory"
check_positive_number "pre-resize sidecar memory working set" "$pre_sidecar_memory"

# Audit evidence
audit_file="$ARTIFACT_DIR/audit-proposal.jsonl"
check_contains "audit has policy evaluation" "$audit_file" '"action":"policy-evaluation"'
check_contains "audit has JIT approval" "$audit_file" '"decision":"allowed: JIT validation passed"'
check_contains "audit has applied resize" "$audit_file" '"decision":"applied: in-place pod resize"'
check_contains "audit has completion" "$audit_file" '"decision":"completed: workload stabilized"'
check_contains "audit records the matched policy" "$audit_file" "\"policy\":\"${POLICY}\""

# Events
events_file="$ARTIFACT_DIR/events.txt"
check_contains "event PodResize" "$events_file" "PodResize"
check_contains "event ExecutionStarted" "$events_file" "ExecutionStarted"
check_contains "event ExecutionCompleted" "$events_file" "ExecutionCompleted"
check_contains "event records API approval" "$events_file" "Approved by e2e-harness"

finish
