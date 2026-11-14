# Reconcilyx 3:00 AM Incident Runbook

This runbook provides step-by-step triage and recovery procedures for Reconcilyx controller, Hub API, federated Spoke WAL, and telemetry pipelines. Every `PrometheusRule` alert emitted by `charts/reconcilyx/templates/prometheusrule.yaml` links directly to a section anchor below.

---

## Global Correlation & Tracing First-Response

Every HTTP request processed by the Reconcilyx Hub API or `kubectl-reconcilyx` CLI carries a W3C `traceparent` header and `X-Request-ID` header, which are echoed on responses and emitted in JSON `slog` records (`trace_id`, `span_id`, `parent_span_id`, `request_id`).

```bash
# Filter controller logs by trace_id or request_id from a failing CI/UI response:
kubectl logs -n reconcilyx-system deployment/reconcilyx-controller-manager --since=30m | grep "<trace_id>"
```

Check pipeline and provenance health in one call:
```bash
curl -fsS -H "X-API-Key: ${RCX_VIEWER_KEY}" http://127.0.0.1:8082/api/v1/healthz | jq .
```

---

## Emergency Procedures

### Emergency Kill Switch (Halt All Live Mutations Immediately)

If Reconcilyx right-sizing is suspected of destabilizing a workload, switch the operator into read-only observation mode immediately without uninstalling CRDs or losing audit history:

1. **Cluster-wide Helm override (removes `pods/resize` patch RBAC and sets `RECONCILYX_ENABLE_INPLACE=false`):**
   ```bash
   helm upgrade reconcilyx charts/reconcilyx -n reconcilyx-system \
     --reuse-values \
     --set actuation.inPlaceResizeEnabled=false \
     --set config.enableInPlaceResizing=false
   ```
2. **Pause an individual namespace policy without touching other teams:**
   ```bash
   kubectl patch optimizationpolicy <policy-name> -n <namespace> \
     --type=merge -p '{"spec":{"mode":"Recommend"}}'
   ```

### Revert a Live In-Place Pod Resize

If a container experiences CPU throttling or memory pressure after an in-place resize and automated TTL rollback has not yet fired:

```bash
kubectl reconcilyx rollback <proposal-name> -n <namespace> \
  --reason "INC-XXXX: manual emergency revert" \
  --server http://127.0.0.1:8082 \
  --token "${RCX_ADMIN_KEY}"
```
Or force rollback via CR annotation / Hub API:
```bash
curl -fsS -X POST -H "X-API-Key: ${RCX_ADMIN_KEY}" -H "Content-Type: application/json" \
  -d '{"reason":"INC-XXXX emergency revert"}' \
  "http://127.0.0.1:8082/api/proposals/<proposal-name>/rollback"
```

### Rollback a Bad Reconcilyx Release (`helm rollback`)

```bash
helm history reconcilyx -n reconcilyx-system
helm rollback reconcilyx <PREVIOUS_GOOD_REVISION> -n reconcilyx-system --wait --timeout 5m
helm test reconcilyx -n reconcilyx-system
```

### Secret & Scoped API Key Rotation (Zero Downtime)

If a scoped credential (`rcx_ci_...`, `rcx_spoke_...`, `rcx_admin_...`) is leaked:
1. Generate a replacement key with the matching prefix (`rcx_admin_<new>`, `rcx_spoke_<cluster>_<new>`).
2. During transition, supply both old and new comma-separated keys in the `RECONCILYX_SCOPED_API_KEYS` Secret, restart/rollout receivers, then remove the compromised key:
   ```bash
   kubectl create secret generic reconcilyx-scoped-keys -n reconcilyx-system \
     --from-literal=RECONCILYX_SCOPED_API_KEYS="rcx_admin_<new>,rcx_spoke_prod-eu_<token>" \
     --dry-run=client -o yaml | kubectl apply -f -
   kubectl rollout restart deployment/reconcilyx-controller-manager -n reconcilyx-system
   ```

---

## Prometheus Alert Runbooks

### ReconcilyxFallbackModeSustained

- **Trigger:** `reconcilyx_pipeline_fallback_active == 1` for >= 15m.
- **Impact:** Live upstream pricing (AWS/GCP/Azure), carbon intensity (WattTime/Electricity Maps), or Prometheus queries are failing or unauthorized; Reconcilyx has failed over cleanly to deterministic static catalogs (prices/carbon may reflect catalog snapshots rather than spot fluctuations).
- **Diagnosis:**
  1. Query `/api/v1/healthz` and inspect `pricingSource`, `carbonSource`, and `telemetryStatus`.
  2. Check manager logs for `component` (`cloud_pricing`, `carbon_provider`, `prometheus_client`) and HTTP status codes (`401`/`403` indicates expired credentials or broken IRSA/Workload Identity; `429`/`5xx` indicates upstream outage).
  3. Verify `NetworkPolicy` egress allows TCP `443` / `9090` if `networkPolicy.enabled=true`.
- **Resolution:**
  - Renew expired ExternalSecrets / projected tmpfs mounts (`/var/run/secrets/reconcilyx/*`), or fix IAM Role / GKE ServiceAccount annotations on the manager ServiceAccount.
  - Once healthy, inspect Prometheus gaps and run a historical telemetry backfill if needed.

### ReconcilyxSpokeSpoolBacklogCritical

- **Trigger:** `reconcilyx_spoke_spool_queued_batches > 20` for >= 10m.
- **Impact:** A Spoke cluster cannot flush Q-table learning deltas to the Hub and is spooling batches to its 50 MiB bounded FIFO WAL directory (`RECONCILYX_SPOKE_SPOOL_DIR`). If the 50 MiB ceiling is reached, oldest unsent delta batches are evicted.
- **Diagnosis:**
  1. Check Hub reachability and TLS/auth logs on the Hub for `401`/`403` (verify the spoke holds a cluster-bound key `rcx_spoke_<clusterName>_...` matching `federation.clusterName`).
  2. Inspect spoke disk spool volume usage (`kubectl exec` not possible in distroless — check `reconcilyx_spoke_spool_queued_batches` and manager logs for `"federated: hub rejected batch"`).
- **Resolution:**
  - Restore network path or fix Hub URL / scoped key mismatch on the spoke Helm release (`federation.hubUrl`).
  - Once Hub connectivity resumes, `Emitter.Flush` automatically drains the disk spool FIFO with `X-Reconcilyx-Idempotency-Key` so no duplicate Q-table deltas are double-counted on the Hub.

### ReconcilyxRetryStormDetected

- **Trigger:** `sum by (component) (rate(reconcilyx_pipeline_retries_total[5m])) > 1` for >= 10m.
- **Impact:** Sustained upstream throttling (`429 Retry-After`) or transient `502/503/504` responses from cloud pricing, carbon, GitOps, or Hub APIs.
- **Diagnosis:**
  1. Group `reconcilyx_pipeline_retries_total` by `component` and `status_code` to pinpoint the failing dependency.
  2. Look for `429` rate limits (reduce poll frequency via `RECONCILYX_PRICING_CACHE_TTL_HOURS` or `federation.flushInterval`) vs `5xx` (upstream outage).
- **Resolution:**
  - Wait out transient upstream throttling (Transport honors `Retry-After` up to `MaxDelay=5s` and only retries idempotent requests).
  - Increase cache TTL or switch carbon/pricing tier keys if quota-exhausted.

### ReconcilyxControllerPodCrashLoop

- **Trigger:** `increase(kube_pod_container_status_restarts_total{namespace="...", container="manager"}[15m]) > 2`.
- **Impact:** Controller reconciliation and/or Hub API serving is degraded or down.
- **Diagnosis:**
  1. Inspect previous container termination reason and logs:
     ```bash
     kubectl describe pod -l app.kubernetes.io/name=reconcilyx -n reconcilyx-system
     kubectl logs -l app.kubernetes.io/name=reconcilyx -n reconcilyx-system --previous --tail=200
     ```
  2. Check for `OOMKilled` (increase `resources.limits.memory` if tracking >10k pods or large backfill ranges) vs readiness probe timeout or missing Secret mounts (`externalSecrets.enabled=true` referencing nonexistent Secret).
- **Resolution:**
  - Fix Secret references or bump memory limits in Helm values, or run `helm rollback reconcilyx <rev> -n reconcilyx-system`.

### ReconcilyxAutomatedRollbacksSpike

- **Trigger:** `sum(increase(reconcilyx_rollback_total[30m])) > 2`.
- **Impact:** Multiple workloads breached post-resize latency/error/OOM stabilization thresholds and were automatically reverted.
- **Diagnosis:**
  1. List affected proposals: `kubectl get optimizationproposals -A -o wide` and filter `RollbackScheduled`/`RolledBack`.
  2. Review `status.learning` receipts and target workload pod events for `OOMKilling` or CPU CFS throttling.
- **Resolution:**
  - Trigger the **Emergency Kill Switch** or switch affected policies to `mode: Recommend` (GitOps shadow PRs instead of live mutation).
  - Run `kubectl reconcilyx backfill --namespace <ns> --from <RFC3339> --to <RFC3339>` once Prometheus telemetry has stabilized so the Q-table and baseline recommender capture accurate burst peaks before re-enabling actuation.

---

## Historical Telemetry Backfill Recovery

After any Prometheus or Hub outage exceeding 2 scrape intervals, recover missing UtilizationBuckets idempotently (keyed by SHA-256 `(workloadUID | windowStartUnix)` — safe to re-run repeatedly without double-counting):

```bash
kubectl reconcilyx backfill \
  --namespace payments \
  --from 2026-11-13T01:00:00Z \
  --to 2026-11-13T04:00:00Z \
  --step 5m \
  --server http://127.0.0.1:8082 \
  --token "${RCX_ADMIN_KEY}"
```
Verify dedup counters via Prometheus metric `reconcilyx_pipeline_backfill_deduped_total` and advance of `newWatermark`.
