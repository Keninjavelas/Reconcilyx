package telemetry

import (
	"context"

	corev1 "k8s.io/api/core/v1"
)

const (
	// ProvenanceOpenCostPrometheus indicates live OpenCost / Kubecost pricing telemetry.
	ProvenanceOpenCostPrometheus = "opencost-prometheus"
	hoursPerMonth                = 730.0
)

// InspectOpenCost queries OpenCost / Kubecost node and management cost series
// from Prometheus. Returns nil when OpenCost metrics are not installed.
func InspectOpenCost(ctx context.Context, client MetricsClient, _ corev1.ObjectReference) (*CostAttribution, error) {
	if client == nil {
		return nil, nil
	}

	nodeVal, nodeErr := client.Query(ctx, `sum(avg_over_time(node_total_hourly_cost[1h]))`)
	mgmtVal, mgmtErr := client.Query(ctx, `sum(avg_over_time(kubecost_cluster_management_cost[1h]))`)

	hourlyNode := parseScalar(nodeVal)
	mgmt := parseScalar(mgmtVal)

	if (nodeErr != nil && mgmtErr != nil) || (hourlyNode == 0 && mgmt == 0) {
		return nil, nil
	}

	return &CostAttribution{
		HourlyNodeCostUSD:    hourlyNode,
		ClusterManagementUSD: mgmt,
		MonthlyNodeCostUSD:   hourlyNode * hoursPerMonth,
		Source:               ProvenanceOpenCostPrometheus,
	}, nil
}
