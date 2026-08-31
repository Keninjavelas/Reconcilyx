package controller

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	RollbackTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reconcilyx_rollback_total",
			Help: "Number of autonomous rollbacks triggered by Reconcilyx due to post-optimization workload stress",
		},
		[]string{"namespace", "workload", "reason"},
	)

	PolicyWorkloadsMatched = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "reconcilyx_policy_workloads_matched",
			Help: "Number of workloads currently matched by each OptimizationPolicy",
		},
		[]string{"namespace", "policy_name"},
	)
)

func init() {
	metrics.Registry.MustRegister(RollbackTotal, PolicyWorkloadsMatched)
}
