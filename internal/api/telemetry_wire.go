package api

import (
	"github.com/reconcilyx/operator/internal/telemetry"
)

func init() {
	telemetryFederatedVersionGauge = telemetry.FederatedGlobalModelVersion.Set
	telemetryFederatedEntriesGauge = telemetry.FederatedGlobalModelEntries.Set
	telemetryDeltasReceivedOne = func(cluster string) {
		telemetry.FederatedDeltasReceivedTotal.WithLabelValues(cluster).Inc()
	}
	telemetryDeltasSentOne = func(cluster string) {
		telemetry.FederatedDeltasSentTotal.WithLabelValues(cluster).Inc()
	}
	telemetryDeltasBatchSentOne = func(cluster, outcome string) {
		telemetry.FederatedDeltasBatchesSentTotal.WithLabelValues(cluster, outcome).Inc()
	}
	telemetryBootstrapOutcomeOne = func(cluster, outcome string) {
		telemetry.FederatedBootstrapLoaded.WithLabelValues(cluster, outcome).Inc()
	}
}

func EmitFederatedDeltaSent(cluster string) { telemetryDeltasSentOne(cluster) }
func EmitFederatedBatchOutcome(cluster, outcome string) {
	telemetryDeltasBatchSentOne(cluster, outcome)
}
func EmitFederatedBootstrapOutcome(cluster, outcome string) {
	telemetryBootstrapOutcomeOne(cluster, outcome)
}
