package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// OptimizationLatency measures the time taken from metric read to proposal generation.
	OptimizationLatency = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "reconcilyx_optimization_latency_seconds",
			Help:    "Time taken from metric read to proposal generation",
			Buckets: prometheus.DefBuckets,
		},
	)

	// SSAConflictsTotal counts the number of Server-Side Apply conflicts.
	SSAConflictsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "reconcilyx_ssa_conflicts_total",
			Help: "Total number of Server-Side Apply conflicts encountered",
		},
	)

	// ProposalStaleness measures how long a proposal sits in the queue before execution.
	ProposalStaleness = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "reconcilyx_proposal_staleness_seconds",
			Help:    "Time a proposal sat in the queue before execution",
			Buckets: prometheus.ExponentialBuckets(1, 2, 10), // 1s to ~512s
		},
	)
)

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(OptimizationLatency, SSAConflictsTotal, ProposalStaleness)
}
