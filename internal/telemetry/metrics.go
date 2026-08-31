package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	RLRecommendationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reconcilyx_rl_recommendations_total",
			Help: "Total number of RL recommendations made per workload",
		},
		[]string{"workload", "namespace", "action_type"},
	)

	RLUpdatesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reconcilyx_rl_updates_total",
			Help: "Total number of RL agent policy updates per workload and outcome",
		},
		[]string{"namespace", "workload", "outcome"},
	)

	RLRewardGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "reconcilyx_rl_reward",
			Help: "Latest reward signal received by the RL agent per workload",
		},
		[]string{"workload", "namespace"},
	)

	ForecastConfidence = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "reconcilyx_forecast_confidence",
			Help: "Confidence of the latest forecast per workload (0-100)",
		},
		[]string{"workload", "namespace"},
	)

	ForecastCPU = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "reconcilyx_forecast_cpu_cores",
			Help: "Forecasted peak CPU usage in cores per workload",
		},
		[]string{"workload", "namespace"},
	)

	ForecastMemory = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "reconcilyx_forecast_memory_bytes",
			Help: "Forecasted peak memory usage in bytes per workload",
		},
		[]string{"workload", "namespace"},
	)

	AnomalySeverity = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "reconcilyx_anomaly_severity",
			Help: "Severity of the last detected anomaly per workload (0-1)",
		},
		[]string{"workload", "namespace", "metric_type"},
	)

	AnomalyEventsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reconcilyx_anomaly_events_total",
			Help: "Total number of anomaly events detected per workload",
		},
		[]string{"workload", "namespace", "metric_type", "severity"},
	)

	ProposalCreatedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reconcilyx_proposal_created_total",
			Help: "Total number of OptimizationProposals created per policy",
		},
		[]string{"namespace", "policy_name", "workload"},
	)

	ProposalApprovedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reconcilyx_proposal_approved_total",
			Help: "Total number of approved OptimizationProposals per policy",
		},
		[]string{"namespace", "policy_name", "workload"},
	)

	SSAConflictsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reconcilyx_ssa_conflicts_total",
			Help: "Total number of Server-Side Apply conflicts encountered",
		},
		[]string{"workload", "namespace"},
	)

	GitOpsYieldsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reconcilyx_gitops_yields_total",
			Help: "Total number of times Reconcilyx yielded to external GitOps changes",
		},
		[]string{"workload", "namespace"},
	)

	PRCreatedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reconcilyx_pr_created_total",
			Help: "Total number of GitOps pull requests created",
		},
		[]string{"provider", "owner", "repo"},
	)

	PRAutoMergedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reconcilyx_pr_automerge_total",
			Help: "Total number of GitOps pull requests auto-merged",
		},
		[]string{"provider", "owner", "repo"},
	)

	OptimizationLatency = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "reconcilyx_optimization_duration_seconds",
			Help:    "Duration of end-to-end optimization analysis in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)

	OptimizationSavingsUSD = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "reconcilyx_optimization_savings_usd_monthly",
			Help: "Estimated monthly savings in USD for the latest proposal per workload",
		},
		[]string{"workload", "namespace"},
	)

	OptimizationCarbonSavedGrams = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "reconcilyx_optimization_carbon_saved_grams_monthly",
			Help: "Estimated monthly CO2 saved in grams for the latest proposal per workload",
		},
		[]string{"workload", "namespace"},
	)

	FederatedDeltasSentTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reconcilyx_federated_deltas_sent_total",
			Help: "Total number of Q-table deltas sent from spoke to hub",
		},
		[]string{"cluster"},
	)

	FederatedDeltasReceivedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reconcilyx_federated_deltas_received_total",
			Help: "Total number of Q-table deltas received by hub from spokes",
		},
		[]string{"cluster"},
	)

	FederatedDeltasBatchesSentTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reconcilyx_federated_batches_sent_total",
			Help: "Total number of delta batches transmitted from spoke to hub",
		},
		[]string{"cluster", "outcome"},
	)

	FederatedGlobalModelVersion = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "reconcilyx_federated_global_model_version",
			Help: "Current version of the federated global Q-table on the hub",
		},
	)

	FederatedGlobalModelEntries = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "reconcilyx_federated_global_model_entries",
			Help: "Number of (state, action) entries in the current global Q-table",
		},
	)

	FederatedBootstrapLoaded = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reconcilyx_federated_bootstrap_total",
			Help: "How many times a spoke attempted to bootstrap global model on start",
		},
		[]string{"cluster", "outcome"},
	)
)
