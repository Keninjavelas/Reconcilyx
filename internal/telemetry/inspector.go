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

// InspectJVM reads JVM heap metrics exposed by a JMX exporter. Metrics without
// a container label are returned under the empty key and applied to the
// workload's containers by the provider.
func (i *defaultInspector) InspectJVM(ctx context.Context, ref corev1.ObjectReference) (map[string]JVMHeapMetrics, error) {
	result := make(map[string]JVMHeapMetrics)

	maxMap, err := i.queryByContainer(ctx, fmt.Sprintf(
		`max(jvm_memory_max_bytes{namespace="%s", pod=~"`+podMatchRegex+`", area="heap"}) by (container)`,
		ref.Namespace, ref.Name))
	if err != nil {
		return nil, err
	}
	committedMap, err := i.queryByContainer(ctx, fmt.Sprintf(
		`max(jvm_memory_bytes_committed{namespace="%s", pod=~"`+podMatchRegex+`", area="heap"}) by (container)`,
		ref.Namespace, ref.Name))
	if err != nil {
		return nil, err
	}
	usedMap, err := i.queryByContainer(ctx, fmt.Sprintf(
		`max(jvm_memory_bytes_used{namespace="%s", pod=~"`+podMatchRegex+`", area="heap"}) by (container)`,
		ref.Namespace, ref.Name))
	if err != nil {
		return nil, err
	}

	for _, containerName := range gatherContainers(maxMap, committedMap, usedMap) {
		heapMax := maxMap[containerName]
		if heapMax <= 0 {
			heapMax = committedMap[containerName]
		}
		heapUsed := usedMap[containerName]
		if heapMax <= 0 && heapUsed <= 0 {
			continue
		}
		result[containerName] = JVMHeapMetrics{
			IsJVMWorkload: true,
			XmxBytes:      heapMax,
			HeapUsedBytes: heapUsed,
		}
	}
	if len(result) > 0 {
		return result, nil
	}

	// JMX exporters without pod relabeling do not carry a container label.
	heapMax, err := i.queryScalar(ctx, fmt.Sprintf(
		`max(jvm_memory_max_bytes{namespace="%s", pod=~"`+podMatchRegex+`", area="heap"})`,
		ref.Namespace, ref.Name))
	if err != nil {
		return nil, err
	}
	heapUsed, err := i.queryScalar(ctx, fmt.Sprintf(
		`max(jvm_memory_bytes_used{namespace="%s", pod=~"`+podMatchRegex+`", area="heap"})`,
		ref.Namespace, ref.Name))
	if err != nil {
		return nil, err
	}
	if heapMax > 0 || heapUsed > 0 {
		result[""] = JVMHeapMetrics{IsJVMWorkload: true, XmxBytes: heapMax, HeapUsedBytes: heapUsed}
	}
	return result, nil
}

// InspectGPU reads NVIDIA DCGM metrics. It returns an empty map when no DCGM
// series are present, so non-GPU workloads are not assigned synthetic data.
func (i *defaultInspector) InspectGPU(ctx context.Context, ref corev1.ObjectReference) (map[string]GPUMetrics, error) {
	result := make(map[string]GPUMetrics)

	utilMap, err := i.queryByContainer(ctx, fmt.Sprintf(
		`max(DCGM_FI_DEV_GPU_UTIL{namespace="%s", pod=~"`+podMatchRegex+`"}) by (container)`,
		ref.Namespace, ref.Name))
	if err != nil {
		return nil, err
	}
	countMap, err := i.queryByContainer(ctx, fmt.Sprintf(
		`count(DCGM_FI_DEV_GPU_UTIL{namespace="%s", pod=~"`+podMatchRegex+`"}) by (container)`,
		ref.Namespace, ref.Name))
	if err != nil {
		return nil, err
	}
	usedMap, err := i.queryByContainer(ctx, fmt.Sprintf(
		`sum(DCGM_FI_DEV_FB_USED{namespace="%s", pod=~"`+podMatchRegex+`"}) by (container)`,
		ref.Namespace, ref.Name))
	if err != nil {
		return nil, err
	}
	totalMap, err := i.queryByContainer(ctx, fmt.Sprintf(
		`sum(DCGM_FI_DEV_FB_TOTAL{namespace="%s", pod=~"`+podMatchRegex+`"}) by (container)`,
		ref.Namespace, ref.Name))
	if err != nil {
		return nil, err
	}
	tempMap, err := i.queryByContainer(ctx, fmt.Sprintf(
		`max(DCGM_FI_DEV_GPU_TEMP{namespace="%s", pod=~"`+podMatchRegex+`"}) by (container)`,
		ref.Namespace, ref.Name))
	if err != nil {
		return nil, err
	}

	for _, containerName := range gatherContainers(utilMap, countMap, usedMap, totalMap, tempMap) {
		count := int32(countMap[containerName])
		util := utilMap[containerName]
		used := usedMap[containerName]
		total := totalMap[containerName]
		temp := tempMap[containerName]
		if count <= 0 && util == 0 && used == 0 && total == 0 && temp == 0 {
			continue
		}
		if count <= 0 {
			count = 1
		}
		result[containerName] = GPUMetrics{
			GPUCount:            count,
			GPUUtilizationPct:   util,
			GPUMemoryUsedBytes:  used * 1024 * 1024,
			GPUMemoryTotalBytes: total * 1024 * 1024,
			TemperatureCelsius:  temp,
		}
	}
	if len(result) > 0 {
		return result, nil
	}

	// DCGM exporters that do not attach the pod's container label still expose
	// usable device metrics; apply them to the whole workload.
	util, err := i.queryScalar(ctx, fmt.Sprintf(
		`avg(DCGM_FI_DEV_GPU_UTIL{namespace="%s", pod=~"`+podMatchRegex+`"})`,
		ref.Namespace, ref.Name))
	if err != nil {
		return nil, err
	}
	count, err := i.queryScalar(ctx, fmt.Sprintf(
		`count(DCGM_FI_DEV_GPU_UTIL{namespace="%s", pod=~"`+podMatchRegex+`"})`,
		ref.Namespace, ref.Name))
	if err != nil {
		return nil, err
	}
	if count > 0 {
		used, _ := i.queryScalar(ctx, fmt.Sprintf(
			`sum(DCGM_FI_DEV_FB_USED{namespace="%s", pod=~"`+podMatchRegex+`"})`,
			ref.Namespace, ref.Name))
		total, _ := i.queryScalar(ctx, fmt.Sprintf(
			`sum(DCGM_FI_DEV_FB_TOTAL{namespace="%s", pod=~"`+podMatchRegex+`"})`,
			ref.Namespace, ref.Name))
		temp, _ := i.queryScalar(ctx, fmt.Sprintf(
			`max(DCGM_FI_DEV_GPU_TEMP{namespace="%s", pod=~"`+podMatchRegex+`"})`,
			ref.Namespace, ref.Name))
		result[""] = GPUMetrics{
			GPUCount:            int32(count),
			GPUUtilizationPct:   util,
			GPUMemoryUsedBytes:  used * 1024 * 1024,
			GPUMemoryTotalBytes: total * 1024 * 1024,
			TemperatureCelsius:  temp,
		}
	}
	return result, nil
}

func (i *defaultInspector) queryByContainer(ctx context.Context, query string) (map[string]float64, error) {
	val, err := i.client.Query(ctx, query)
	if err != nil && err != ErrNoData {
		return nil, err
	}
	return parseVectorToMap(val), nil
}

func (i *defaultInspector) queryScalar(ctx context.Context, query string) (float64, error) {
	val, err := i.client.Query(ctx, query)
	if err != nil && err != ErrNoData {
		return 0, err
	}
	return parseScalar(val), nil
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
