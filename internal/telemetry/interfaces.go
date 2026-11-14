package telemetry

import (
	"context"
	"time"

	"github.com/prometheus/common/model"
	corev1 "k8s.io/api/core/v1"
)

// MetricsClient handles raw interaction with the Prometheus API.
type MetricsClient interface {
	// Query executes a single PromQL query and returns the raw model.Value.
	Query(ctx context.Context, query string) (model.Value, error)
	
	// QueryRange executes a PromQL query over a time range.
	QueryRange(ctx context.Context, query string, r time.Duration) (model.Value, error)
	
	// HealthCheck verifies the underlying provider is reachable.
	HealthCheck(ctx context.Context) error
}

// WindowedMetricsClient extends MetricsClient with explicit [start, end] range queries
// for deterministic historical telemetry backfill without changing MetricsClient mocks.
type WindowedMetricsClient interface {
	QueryRangeWindow(ctx context.Context, query string, start, end time.Time, step time.Duration) (model.Value, error)
}

// WorkloadInspector abstracts the translation of a K8s object into its real-world telemetry state.
type WorkloadInspector interface {
	InspectCPU(ctx context.Context, ref corev1.ObjectReference) (map[string]CPUMetrics, error)
	InspectMemory(ctx context.Context, ref corev1.ObjectReference) (map[string]MemoryMetrics, error)
	InspectRestarts(ctx context.Context, ref corev1.ObjectReference) (map[string]int32, error)
	InspectJVM(ctx context.Context, ref corev1.ObjectReference) (map[string]JVMHeapMetrics, error)
	InspectGPU(ctx context.Context, ref corev1.ObjectReference) (map[string]GPUMetrics, error)
}

// TelemetryProvider is the high-level façade injected into the Optimization Engine.
type TelemetryProvider interface {
	GetWorkloadMetrics(ctx context.Context, ref corev1.ObjectReference) (*WorkloadMetrics, error)
}
