package pipelinemetrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// RetriesTotal counts retry attempts issued across external HTTP pipelines
	// (cloud pricing, carbon telemetry, GitOps webhooks/APIs, and federated spoke emitter).
	RetriesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reconcilyx_pipeline_retries_total",
			Help: "Total number of HTTP pipeline retry attempts executed before succeeding or falling back",
		},
		[]string{"component", "status_code"},
	)

	// BackfillWindowsTotal counts telemetry backfill windows processed or skipped idempotently.
	BackfillWindowsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reconcilyx_pipeline_backfill_windows_total",
			Help: "Total number of historical telemetry backfill windows evaluated by outcome",
		},
		[]string{"namespace", "outcome"},
	)

	// BackfillDedupedTotal counts duplicate telemetry backfill buckets skipped via SHA-256 idempotency keys.
	BackfillDedupedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reconcilyx_pipeline_backfill_deduped_total",
			Help: "Total number of duplicate historical telemetry backfill buckets skipped idempotently",
		},
		[]string{"namespace"},
	)

	// FallbackActive indicates (1=active, 0=live) whether a pipeline component is currently
	// relying on its static catalog fallback instead of live upstream telemetry or pricing.
	FallbackActive = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "reconcilyx_pipeline_fallback_active",
			Help: "Whether static fallback mode is currently active (1) or live data succeeded (0) per component/provider",
		},
		[]string{"component", "provider"},
	)

	// SpokeSpoolQueuedBatches tracks the number of federated Q-table delta batches
	// persisted on local spoke disk waiting to drain to the Hub.
	SpokeSpoolQueuedBatches = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "reconcilyx_spoke_spool_queued_batches",
			Help: "Number of federated delta batches currently queued in the spoke disk spool",
		},
		[]string{"cluster"},
	)
)

func init() {
	metrics.Registry.MustRegister(
		RetriesTotal,
		BackfillWindowsTotal,
		BackfillDedupedTotal,
		FallbackActive,
		SpokeSpoolQueuedBatches,
	)
}
