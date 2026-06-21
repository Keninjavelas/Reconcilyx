package telemetry

import (
	"context"
	"fmt"

	"github.com/prometheus/common/model"
	corev1 "k8s.io/api/core/v1"
)

type defaultInspector struct {
	client MetricsClient
}

// NewWorkloadInspector creates a WorkloadInspector using the provided MetricsClient.
func NewWorkloadInspector(client MetricsClient) WorkloadInspector {
	return &defaultInspector{
		client: client,
	}
}

// podMatchRegex strictly matches Deployments, StatefulSets, and DaemonSets without colliding with longer prefixes.
const podMatchRegex = `^%s-(?:[a-f0-9]+-[a-z0-9]+|[0-9]+|[a-z0-9]{5})$`

func (i *defaultInspector) InspectCPU(ctx context.Context, ref corev1.ObjectReference) (map[string]CPUMetrics, error) {
	result := make(map[string]CPUMetrics)

	// 1. Requested CPU
	reqQuery := fmt.Sprintf(`max(kube_pod_container_resource_requests{namespace="%s", pod=~"`+podMatchRegex+`", container!="POD", container!="", resource="cpu"}) by (container)`, ref.Namespace, ref.Name)
	reqVal, err := i.client.Query(ctx, reqQuery)
	if err != nil && err != ErrNoData {
		return nil, fmt.Errorf("failed to get CPU requests: %w", err)
	}
	reqMap := parseVectorToMap(reqVal)

	// 2. Limited CPU
	limQuery := fmt.Sprintf(`max(kube_pod_container_resource_limits{namespace="%s", pod=~"`+podMatchRegex+`", container!="POD", container!="", resource="cpu"}) by (container)`, ref.Namespace, ref.Name)
	limVal, err := i.client.Query(ctx, limQuery)
	if err != nil && err != ErrNoData {
		return nil, fmt.Errorf("failed to get CPU limits: %w", err)
	}
	limMap := parseVectorToMap(limVal)

	// 3. Actual CPU Usage (Max of rate across replicas to capture the hottest container)
	usageQuery := fmt.Sprintf(`max(rate(container_cpu_usage_seconds_total{namespace="%s", pod=~"`+podMatchRegex+`", container!="POD", container!=""}[5m])) by (container)`, ref.Namespace, ref.Name)
	usageVal, err := i.client.Query(ctx, usageQuery)
	if err != nil && err != ErrNoData {
		return nil, fmt.Errorf("failed to get CPU usage: %w", err)
	}
	usageMap := parseVectorToMap(usageVal)

	// 4. Throttling
	throttleQuery := fmt.Sprintf(`max(rate(container_cpu_cfs_throttled_seconds_total{namespace="%s", pod=~"`+podMatchRegex+`", container!="POD", container!=""}[5m])) by (container)`, ref.Namespace, ref.Name)
	throttleVal, err := i.client.Query(ctx, throttleQuery)
	if err != nil && err != ErrNoData {
		return nil, fmt.Errorf("failed to get CPU throttling: %w", err)
	}
	throttleMap := parseVectorToMap(throttleVal)

	// Merge into CPUMetrics map
	containers := gatherContainers(reqMap, limMap, usageMap, throttleMap)
	for _, c := range containers {
		result[c] = CPUMetrics{
			RequestedCores: reqMap[c],
			LimitedCores:   limMap[c],
			UsageCores:     usageMap[c],
			ThrottledSecs:  throttleMap[c],
		}
	}

	return result, nil
}

func (i *defaultInspector) InspectMemory(ctx context.Context, ref corev1.ObjectReference) (map[string]MemoryMetrics, error) {
	result := make(map[string]MemoryMetrics)

	// 1. Requested Memory
	reqQuery := fmt.Sprintf(`max(kube_pod_container_resource_requests{namespace="%s", pod=~"`+podMatchRegex+`", container!="POD", container!="", resource="memory"}) by (container)`, ref.Namespace, ref.Name)
	reqVal, err := i.client.Query(ctx, reqQuery)
	if err != nil && err != ErrNoData {
		return nil, fmt.Errorf("failed to get Memory requests: %w", err)
	}
	reqMap := parseVectorToMap(reqVal)

	// 2. Limited Memory
	limQuery := fmt.Sprintf(`max(kube_pod_container_resource_limits{namespace="%s", pod=~"`+podMatchRegex+`", container!="POD", container!="", resource="memory"}) by (container)`, ref.Namespace, ref.Name)
	limVal, err := i.client.Query(ctx, limQuery)
	if err != nil && err != ErrNoData {
		return nil, fmt.Errorf("failed to get Memory limits: %w", err)
	}
	limMap := parseVectorToMap(limVal)

	// 3. Actual Memory Working Set
	usageQuery := fmt.Sprintf(`max(max_over_time(container_memory_working_set_bytes{namespace="%s", pod=~"`+podMatchRegex+`", container!="POD", container!=""}[5m])) by (container)`, ref.Namespace, ref.Name)
	usageVal, err := i.client.Query(ctx, usageQuery)
	if err != nil && err != ErrNoData {
		return nil, fmt.Errorf("failed to get Memory usage: %w", err)
	}
	usageMap := parseVectorToMap(usageVal)

	// 4. OOM Count
	oomQuery := fmt.Sprintf(`max(kube_pod_container_status_last_terminated_reason{namespace="%s", pod=~"`+podMatchRegex+`", container!="POD", container!="", reason="OOMKilled"}) by (container)`, ref.Namespace, ref.Name)
	oomVal, err := i.client.Query(ctx, oomQuery)
	if err != nil && err != ErrNoData {
		return nil, fmt.Errorf("failed to get OOM count: %w", err)
	}
	oomMap := parseVectorToMap(oomVal)

	// Merge into MemoryMetrics map
	containers := gatherContainers(reqMap, limMap, usageMap, oomMap)
	for _, c := range containers {
		result[c] = MemoryMetrics{
			RequestedBytes:  reqMap[c],
			LimitedBytes:    limMap[c],
			WorkingSetBytes: usageMap[c],
			OOMCount:        int32(oomMap[c]),
		}
	}

	return result, nil
}

func (i *defaultInspector) InspectRestarts(ctx context.Context, ref corev1.ObjectReference) (map[string]int32, error) {
	result := make(map[string]int32)
	
	restartQuery := fmt.Sprintf(`max(kube_pod_container_status_restarts_total{namespace="%s", pod=~"`+podMatchRegex+`", container!="POD", container!=""}) by (container)`, ref.Namespace, ref.Name)
	restartVal, err := i.client.Query(ctx, restartQuery)
	if err != nil && err != ErrNoData {
		return nil, fmt.Errorf("failed to get Restarts: %w", err)
	}
	restartMap := parseVectorToMap(restartVal)

	for k, v := range restartMap {
		result[k] = int32(v)
	}

	return result, nil
}

// parseVectorToMap extracts float64 values grouped by the "container" label from a Prometheus model.Value.
func parseVectorToMap(val model.Value) map[string]float64 {
	res := make(map[string]float64)
	if val == nil {
		return res
	}

	switch v := val.(type) {
	case model.Vector:
		for _, sample := range v {
			if container, ok := sample.Metric["container"]; ok {
				res[string(container)] = float64(sample.Value)
			}
		}
	case *model.Scalar:
		// Not expected with 'by (container)', but safe fallback.
	}
	return res
}

// parseScalar extracts a float64 from a Prometheus model.Value.
func parseScalar(val model.Value) float64 {
	if val == nil {
		return 0
	}
	switch v := val.(type) {
	case model.Vector:
		if len(v) > 0 {
			return float64(v[0].Value)
		}
	case *model.Scalar:
		return float64(v.Value)
	}
	return 0
}

func gatherContainers(maps ...map[string]float64) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, m := range maps {
		for k := range m {
			if _, ok := seen[k]; !ok {
				seen[k] = struct{}{}
				result = append(result, k)
			}
		}
	}
	return result
}
