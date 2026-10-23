package telemetry

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

// ScrapeGPUMetrics extracts NVIDIA DCGM metrics for a workload from Prometheus.
// It returns zero values when no DCGM series exist; it never fabricates GPU
// utilization for workloads without an exporter.
func ScrapeGPUMetrics(ctx context.Context, promClient MetricsClient, workload, namespace string) (GPUMetrics, error) {
	if promClient == nil {
		return GPUMetrics{}, fmt.Errorf("GPU metrics require a Prometheus client")
	}
	inspector := NewWorkloadInspector(promClient)
	byContainer, err := inspector.InspectGPU(ctx, corev1.ObjectReference{Name: workload, Namespace: namespace})
	if err != nil {
		return GPUMetrics{}, err
	}
	if m, ok := byContainer[""]; ok {
		return m, nil
	}

	var aggregate GPUMetrics
	containers := 0
	for _, m := range byContainer {
		if m.GPUCount <= 0 {
			continue
		}
		aggregate.GPUCount += m.GPUCount
		aggregate.GPUUtilizationPct += m.GPUUtilizationPct * float64(m.GPUCount)
		aggregate.GPUMemoryUsedBytes += m.GPUMemoryUsedBytes
		aggregate.GPUMemoryTotalBytes += m.GPUMemoryTotalBytes
		if m.TemperatureCelsius > aggregate.TemperatureCelsius {
			aggregate.TemperatureCelsius = m.TemperatureCelsius
		}
		containers += int(m.GPUCount)
	}
	if containers > 0 {
		aggregate.GPUUtilizationPct /= float64(containers)
	}
	return aggregate, nil
}
