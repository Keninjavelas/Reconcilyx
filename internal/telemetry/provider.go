package telemetry

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

type defaultProvider struct {
	client    MetricsClient
	inspector WorkloadInspector
}

// NewTelemetryProvider creates the high-level façade for the Optimization Engine.
func NewTelemetryProvider(client MetricsClient) TelemetryProvider {
	return &defaultProvider{
		client:    client,
		inspector: NewWorkloadInspector(client),
	}
}

func (p *defaultProvider) GetWorkloadMetrics(ctx context.Context, ref corev1.ObjectReference) (*WorkloadMetrics, error) {
	// Pre-flight health check to avoid cascading timeouts
	if err := p.client.HealthCheck(ctx); err != nil {
		return nil, fmt.Errorf("telemetry provider unavailable: %w", err)
	}

	cpuMap, errCPU := p.inspector.InspectCPU(ctx, ref)
	memMap, errMem := p.inspector.InspectMemory(ctx, ref)
	restartMap, errRestart := p.inspector.InspectRestarts(ctx, ref)

	// Fetch Replica count from kube-state-metrics as well
	repQuery := fmt.Sprintf(`sum(kube_deployment_status_replicas_available{namespace="%s", deployment="%s"})`, ref.Namespace, ref.Name)
	repVal, errRep := p.client.Query(ctx, repQuery)
	replicas := int32(parseScalar(repVal))

	// Partial Failure Handling
	if errCPU != nil && errMem != nil {
		return nil, fmt.Errorf("failed to gather both CPU and Memory metrics (CPU: %v, Mem: %v)", errCPU, errMem)
	}

	// Merge the container data
	containerMetrics := make(map[string]*ContainerMetrics)
	
	// Collect all unique container names
	seen := make(map[string]struct{})
	for k := range cpuMap { seen[k] = struct{}{} }
	for k := range memMap { seen[k] = struct{}{} }
	for k := range restartMap { seen[k] = struct{}{} }

	for containerName := range seen {
		cm := &ContainerMetrics{
			Name: containerName,
		}
		if c, ok := cpuMap[containerName]; ok {
			cm.CPU = c
		}
		if m, ok := memMap[containerName]; ok {
			cm.Memory = m
		}
		if r, ok := restartMap[containerName]; ok {
			cm.Restarts = r
		}
		containerMetrics[containerName] = cm
	}

	workloadMetrics := &WorkloadMetrics{
		Replicas:   replicas,
		Containers: containerMetrics,
	}

	if errCPU != nil || errMem != nil || errRestart != nil {
		// Log warning in a real system: "Partial telemetry retrieved"
		return workloadMetrics, ErrPartialTelemetry
	}

	if errRep != nil {
		// Replicas are non-critical if we have CPU/Mem, but we should log it
	}

	return workloadMetrics, nil
}
