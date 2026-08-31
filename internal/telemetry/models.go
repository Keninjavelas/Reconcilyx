package telemetry

import (
	"errors"
)

var (
	ErrNoData                = errors.New("telemetry: no data returned for query")
	ErrPartialTelemetry      = errors.New("telemetry: partial telemetry returned")
	ErrPrometheusUnavailable = errors.New("telemetry: prometheus unavailable")
)

type WorkloadMetrics struct {
	Replicas   int32
	Containers map[string]*ContainerMetrics
}

type ContainerMetrics struct {
	Name     string
	CPU      CPUMetrics
	Memory   MemoryMetrics
	JVM      JVMHeapMetrics
	GPU      GPUMetrics
	Restarts int32
}

type CPUMetrics struct {
	RequestedCores  float64
	LimitedCores    float64
	UsageCores      float64
	ThrottledSecs   float64
	HistoricalCores []float64
}

type MemoryMetrics struct {
	RequestedBytes            float64
	LimitedBytes              float64
	WorkingSetBytes           float64
	OOMCount                  int32
	HistoricalWorkingSetBytes []uint64
}

type JVMHeapMetrics struct {
	IsJVMWorkload bool    `json:"isJVMWorkload"`
	XmxBytes      float64 `json:"xmxBytes"`
	XmsBytes      float64 `json:"xmsBytes"`
	HeapUsedBytes float64 `json:"heapUsedBytes"`
}

type GPUMetrics struct {
	GPUCount           int32   `json:"gpuCount"`
	GPUUtilizationPct  float64 `json:"gpuUtilizationPct"`
	GPUMemoryUsedBytes float64 `json:"gpuMemoryUsedBytes"`
	GPUMemoryTotalBytes float64 `json:"gpuMemoryTotalBytes"`
	TemperatureCelsius float64 `json:"temperatureCelsius"`
}
