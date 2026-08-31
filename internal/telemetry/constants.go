package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	ProposalStaleness = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "reconcilyx_proposal_staleness_seconds",
			Help:    "Time from proposal creation to agent processing",
			Buckets: prometheus.DefBuckets,
		},
	)
)
