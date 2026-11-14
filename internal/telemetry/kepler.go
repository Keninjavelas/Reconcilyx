package telemetry

import (
	"context"
	"fmt"

	"github.com/prometheus/common/model"
	corev1 "k8s.io/api/core/v1"
)

const (
	// ProvenanceKeplerEBPF indicates real eBPF RAPL power telemetry from Kepler.
	ProvenanceKeplerEBPF = "kepler-ebpf"
	// ProvenanceTDPEstimate indicates deterministic TDP linear fallback when Kepler is absent.
	ProvenanceTDPEstimate = "tdp-model-estimate"

	defaultIdleWattsPerContainer = 3.0
	defaultDynamicWattsPerCore   = 15.0
)

// InspectEnergy queries Kepler Prometheus metrics when deployed, and falls back
// deterministically to a TDP linear estimate per container when Kepler is absent.
// Provenance explicitly distinguishes real eBPF energy telemetry from estimates.
func InspectEnergy(ctx context.Context, client MetricsClient, ref corev1.ObjectReference, usageByContainer map[string]float64) (map[string]EnergyMetrics, string) {
	result := make(map[string]EnergyMetrics, len(usageByContainer))

	if client != nil && ref.Name != "" {
		totalQuery := fmt.Sprintf(`sum(rate(kepler_container_joules_total{container_namespace="%s", pod_name=~"`+podMatchRegex+`"}[5m])) by (container_name)`, ref.Namespace, ref.Name)
		pkgQuery := fmt.Sprintf(`sum(rate(kepler_container_package_joules_total{container_namespace="%s", pod_name=~"`+podMatchRegex+`"}[5m])) by (container_name)`, ref.Namespace, ref.Name)

		totalVal, totalErr := client.Query(ctx, totalQuery)
		pkgVal, pkgErr := client.Query(ctx, pkgQuery)
		if totalErr == nil || pkgErr == nil {
			totalMap := parseVectorLabelMap(totalVal, "container_name", "container")
			pkgMap := parseVectorLabelMap(pkgVal, "container_name", "container")
			if len(totalMap) > 0 || len(pkgMap) > 0 {
				for _, c := range gatherContainers(totalMap, pkgMap) {
					totalW := totalMap[c]
					pkgW := pkgMap[c]
					if totalW == 0 {
						totalW = pkgW
					}
					result[c] = EnergyMetrics{
						PackageWatts: pkgW,
						TotalWatts:   totalW,
						Provenance:   ProvenanceKeplerEBPF,
					}
				}
				return result, ProvenanceKeplerEBPF
			}
		}
	}

	for name, cores := range usageByContainer {
		if cores < 0 {
			cores = 0
		}
		watts := defaultIdleWattsPerContainer + cores*defaultDynamicWattsPerCore
		result[name] = EnergyMetrics{
			PackageWatts: watts * 0.85,
			TotalWatts:   watts,
			Provenance:   ProvenanceTDPEstimate,
		}
	}
	return result, ProvenanceTDPEstimate
}

func parseVectorLabelMap(val model.Value, labelKeys ...string) map[string]float64 {
	out := make(map[string]float64)
	vec, ok := val.(model.Vector)
	if !ok {
		return out
	}
	for _, sample := range vec {
		for _, k := range labelKeys {
			if name, exists := sample.Metric[model.LabelName(k)]; exists && string(name) != "" {
				out[string(name)] = float64(sample.Value)
				break
			}
		}
	}
	return out
}
