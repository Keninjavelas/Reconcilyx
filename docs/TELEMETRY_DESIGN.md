# Reconcilyx Telemetry Design

## 1. Telemetry Architecture

### Data Sources
- **Primary:** Prometheus (via standard PromQL HTTP API). This is the sole source of truth for the MVP, capturing both cAdvisor container metrics and `kube-state-metrics`.
- **Future Extensibility:** The architecture is interface-driven. Future implementations may introduce an eBPF client (e.g., Cilium Hubble) to augment Prometheus data with low-level kernel metrics.

### Collection & Query Flow
1. **Trigger:** The Optimization Engine initiates an evaluation for a specific workload.
2. **Query Building:** The `WorkloadInspector` translates the workload reference into a series of templated PromQL queries.
3. **Execution:** The `MetricsClient` executes these queries asynchronously (where possible) against the Prometheus HTTP API.
4. **Parsing:** The `MetricsClient` parses the returned `model.Value` (Vector/Matrix) into typed Go structs.
5. **Aggregation:** The `TelemetryProvider` aggregates the distinct metrics (CPU, Memory, State) into a unified `WorkloadMetrics` model.

### Caching Strategy
To prevent overwhelming the Prometheus API during high-scale evaluations, queries will be cached in-memory with a short TTL (e.g., 60 seconds). For the MVP, caching is heavily scoped to the boundaries of a single reconciliation loop.

### Failure Handling
The system assumes the observability stack is flaky. It employs exponential backoff for HTTP requests, context timeouts for long-running queries, and a circuit breaker pattern if Prometheus goes down completely.

---

## 2. MVP Metrics Catalog

### CPU
| Metric | Source (Prometheus Series) | Query Method | Unit | Intended Usage |
| :--- | :--- | :--- | :--- | :--- |
| Requested CPU | `kube_pod_container_resource_requests` | `sum(...) by (...)` | Cores | Compare against actual usage for under-provisioning. |
| Limited CPU | `kube_pod_container_resource_limits` | `sum(...) by (...)` | Cores | Assess maximum allowed burst. |
| Actual CPU Usage | `container_cpu_usage_seconds_total` | `rate(...[5m])` (P95 over time) | Cores | Determine the actual required baseline. |
| CPU Throttling | `container_cpu_cfs_throttled_seconds_total` | `rate(...[5m])` | Seconds | Identify performance starvation. |

### Memory
| Metric | Source (Prometheus Series) | Query Method | Unit | Intended Usage |
| :--- | :--- | :--- | :--- | :--- |
| Requested Memory| `kube_pod_container_resource_requests` | `sum(...) by (...)` | Bytes | Compare against working set. |
| Limited Memory | `kube_pod_container_resource_limits` | `sum(...) by (...)` | Bytes | Understand OOM thresholds. |
| Actual Memory | `container_memory_working_set_bytes` | `max_over_time(...[5m])` | Bytes | Real memory usage (excluding cache). |
| Memory Pressure | `kube_pod_container_status_last_terminated_reason` | `count(reason="OOMKilled")`| Count | Detect severe memory starvation. |

### Workload
| Metric | Source (Prometheus Series) | Query Method | Unit | Intended Usage |
| :--- | :--- | :--- | :--- | :--- |
| Replica Count | `kube_deployment_status_replicas_available` | `max(...)` | Count | Ensure quorum before optimizing. |
| Restart Count | `kube_pod_container_status_restarts_total` | `increase(...[1h])` | Count | Detect instability. |
| Pod Age | `kube_pod_start_time` | `time() - min(...)` | Seconds | Avoid optimizing brand new pods. |
| Namespace/Labels| K8s API directly | N/A | Metadata | Policy mapping. |

### Operator (Self-Monitoring)
| Metric | Source | Type | Unit | Intended Usage |
| :--- | :--- | :--- | :--- | :--- |
| Proposal Count | Operator Registry | Counter | Count | Track business impact. |
| Recon Count | controller-runtime | Counter | Count | Track engine activity. |
| Error Count | Operator Registry | Counter | Count | Track system health. |

---

## 3. Query Interfaces

To ensure low coupling, the `internal/telemetry` package defines the following strict interfaces:

```go
package telemetry

import (
	"context"
	"time"
	corev1 "k8s.io/api/core/v1"
)

// MetricsClient handles raw interaction with the Prometheus API.
type MetricsClient interface {
	Query(ctx context.Context, query string) (float64, error)
	QueryRange(ctx context.Context, query string, r time.Duration) ([]DataPoint, error)
	HealthCheck(ctx context.Context) error
}

// WorkloadInspector abstracts the translation of a K8s object into its real-world telemetry state.
type WorkloadInspector interface {
	InspectCPU(ctx context.Context, ref corev1.ObjectReference) (*CPUMetrics, error)
	InspectMemory(ctx context.Context, ref corev1.ObjectReference) (*MemoryMetrics, error)
}

// TelemetryProvider is the high-level façade injected into the Optimization Engine.
type TelemetryProvider interface {
	GetWorkloadMetrics(ctx context.Context, ref corev1.ObjectReference) (*WorkloadMetrics, error)
}
```

---

## 4. Error Handling Model

Because optimization is a non-critical control loop (unlike standard K8s deployments), safety is prioritized over availability:
- **Timeout Strategy:** Strict 5-second context timeouts per query. Optimization must not block forever.
- **Retry Strategy:** 3 attempts with exponential backoff (e.g., 500ms, 1s, 2s).
- **Partial Failure Handling:** If CPU metrics succeed but Memory fails, the `TelemetryProvider` returns `ErrPartialTelemetry`. The Engine must abandon the evaluation (fail-safe).
- **Empty Result Handling:** Return a typed `ErrNoData`. The Engine handles this gracefully (e.g., skipping brand new deployments).
- **Prometheus Unavailable:** If `HealthCheck()` fails, the Operator pauses all proposal generation and logs a critical alert.

---

## 5. Testing Strategy

- **Unit Tests:** The entire logic of query building, data parsing, and error aggregation will be tested using a mocked `MetricsClient`. 
- **Integration Tests:** We will scaffold `setup-envtest` equivalents for Prometheus, likely using a lightweight mock HTTP server that perfectly mimics the Prometheus v1 HTTP API responses, verifying that the `TelemetryProvider` correctly negotiates HTTP.
- **Mock Strategy:** The project will define standard mock implementations of `TelemetryProvider` to be injected into the Engine tests during future phases.
