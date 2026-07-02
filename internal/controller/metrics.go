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
)

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(RollbackTotal)
}
