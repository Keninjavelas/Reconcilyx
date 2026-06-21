# Telemetry Foundation Architecture Review

## Executive Summary
The `internal/telemetry` subsystem provides a well-structured, interface-driven approach to observability. The layering separates the raw HTTP client from domain-specific PromQL translation (Inspector) and high-level orchestration (Provider). It strongly demonstrates dependency inversion and testability.

However, a strict production-readiness review reveals critical flaws in the PromQL queries and the Domain Model itself. The most severe issue is the aggregate nature of the metrics—Kubernetes resource limits are applied at the *container* level, but the telemetry model aggregates metrics at the *pod/workload* level. If the Optimization Engine consumes this data, it will generate invalid patches that cannot accurately target main containers vs. sidecars (e.g., `istio-proxy`).

**Recommendation:** GO WITH CONDITIONS.

---

## 1. Architecture Assessment
**Strengths:**
- **Strict Layering:** Separating `MetricsClient` (HTTP logic), `WorkloadInspector` (PromQL generation), and `TelemetryProvider` (Aggregation and Resilience) is textbook software engineering.
- **Dependency Inversion:** The `internal/engine` will only ever rely on the `TelemetryProvider` interface, allowing complete mocking without pulling in HTTP logic.
- **Resilience:** The exponential backoff, strict timeouts (5s), and early health checking prevent the operator from cascading into failure states when Prometheus is degraded.

**Weaknesses:**
- No structured logging is injected into the telemetry client. It currently lacks the ability to emit debug logs (e.g., `log.V(1).Info("Executing PromQL", "query", q)`).

---

## 2. PromQL Assessment
**Query Design Flaws:**
- **Deployment Name Risk:** The query uses `pod=~"%s-.*"` (e.g., `pod=~"frontend-.*"`). If a namespace contains deployments named `frontend` and `frontend-worker`, the query for `frontend` will accidentally aggregate metrics from both. This is a classic cardinality collision risk.
- **Container Aggregation:** Queries like `sum(kube_pod_container_resource_requests...)` sum *all* containers in the pod. It mixes the core application with `linkerd-proxy`, `datadog-agent`, and `kube-rbac-proxy`.
- **P95 Approximation:** The MVP states it evaluates P95, but the query `sum(rate(...[5m]))` merely yields the current 5-minute average usage, not a percentile over time. A true P95 requires `quantile_over_time(0.95, rate(...)[24h:])` (using subqueries).

---

## 3. Failure Mode Assessment
- **Prometheus Unavailable:** Well handled. `HealthCheck()` runs first, returning `ErrPrometheusUnavailable`.
- **Partial Query Failures:** Well handled via `ErrPartialTelemetry`. The Engine can choose to fail-safe.
- **Empty Result Sets:** Returns `ErrNoData`, which safely skips workloads without metrics (e.g., brand new deployments or missing ServiceMonitors).
- **Workload Deletion:** Returns `ErrNoData`, preventing race conditions.
- **Stale Metrics:** Currently unhandled. If Prometheus is up but ingestion is broken, queries might return stale data. (Acceptable for MVP, but needs a `time()` diff check in production).

---

## 4. API & Model Assessment
**Are the telemetry models sufficient for optimization?** No.

**Missing Fields:**
- **Container-Level Granularity:** `WorkloadMetrics` must contain a map of containers: `map[string]*ContainerMetrics` instead of top-level `CPUMetrics` and `MemoryMetrics`. You cannot right-size a pod; you can only right-size a container.
- **Container Name:** To patch a K8s Deployment, you need to know *which* container's `resources` struct to update.

---

## 5. Portfolio Assessment
**Would a Staff Engineer be impressed?**
Yes, by the testing strategy, mock implementations, error definitions, and interface boundaries. It shows deep Go knowledge.

**What still feels MVP-grade?**
The regex-based PromQL `pod=~"name-.*"` matching and the aggregate metric models. These are shortcuts that break down immediately in complex namespaces.

**What upgrades would make this subsystem exceptional?**
Using `kube_pod_labels` to do exact replica-set joining via PromQL (e.g., `* on(pod) group_left() kube_pod_labels{...}`), and implementing true `quantile_over_time` subqueries.

---

## Technical Debt Register
1. **Container Aggregation:** `WorkloadMetrics` lacks container specificity.
2. **PromQL Regex Matching:** `pod=~"name-.*"` risks cardinality overlap.
3. **P95 Accuracy:** CPU usage queries calculate basic rates instead of percentiles.
4. **Missing Logger:** Telemetry provider cannot log warnings natively.

---

## Top 10 Improvements
1. Refactor `WorkloadMetrics` to expose `map[string]*ContainerMetrics`.
2. Rewrite `WorkloadInspector` PromQL to group by `container` instead of summing them together.
3. Improve pod matching regex to expect a hash `pod=~"%s-[a-f0-9]+-[a-z0-9]+"` for Deployments to avoid overlapping prefixes.
4. Replace `rate(...[5m])` with `quantile_over_time(0.95, ...)` for real baseline analysis.
5. Inject `logr.Logger` into the `TelemetryProvider`.
6. Add `time()` boundary checks to warn if data is > 10m stale.
7. Support `StatefulSet` specific pod naming (`name-0`, `name-1`).
8. Add caching layer around PromQL responses.
9. Support configurable lookback windows (e.g., 24h vs 7d).
10. Add Prometheus query timing to a custom metrics registry to track API latency.

---

## Readiness Recommendation

**GO WITH CONDITIONS**

Before writing the Engine Loop (`internal/engine`), you **MUST** resolve the API granularity flaw. If the engine is built on pod-aggregate metrics, the reconciliation logic will be fundamentally broken and will overwrite sidecar resources.

**Required Changes Before Engine Implementation:**
1. Refactor `internal/telemetry/models.go` to group metrics by `Container` instead of globally summing them per pod.
   - Example: `Containers map[string]*ContainerMetrics` where `ContainerMetrics` contains CPU and Memory.
2. Update the `internal/telemetry/inspector.go` PromQL queries to include `by (container)` and return data per-container.
3. Fix the unit tests to match the new container-granular API.
