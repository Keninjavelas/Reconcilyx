package telemetry

import (
	"errors"
)

var (
	// ErrNoData indicates the query executed successfully but returned zero results.
	ErrNoData = errors.New("telemetry: no data returned for query")

	// ErrPartialTelemetry indicates some metrics succeeded, but critical metrics failed.
	ErrPartialTelemetry = errors.New("telemetry: partial telemetry returned")

	// ErrPrometheusUnavailable indicates the telemetry provider is completely unreachable.
	ErrPrometheusUnavailable = errors.New("telemetry: prometheus unavailable")
)

// WorkloadMetrics aggregates all telemetry for a specific workload, grouped by container.
type WorkloadMetrics struct {
	Replicas   int32
	Containers map[string]*ContainerMetrics
}

// ContainerMetrics holds the right-sizing data for a specific container within a workload.
type ContainerMetrics struct {
	Name     string
	CPU      CPUMetrics
	Memory   MemoryMetrics
	Restarts int32
}

// CPUMetrics contains usage and limit data for CPU.
type CPUMetrics struct {
	RequestedCores float64
	LimitedCores   float64
	UsageCores     float64
	ThrottledSecs  float64
}

// MemoryMetrics contains usage and limit data for Memory.
type MemoryMetrics struct {
	RequestedBytes  float64
	LimitedBytes    float64
	WorkingSetBytes float64
	OOMCount        int32
}
