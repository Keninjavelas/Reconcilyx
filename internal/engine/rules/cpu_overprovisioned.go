package rules

import (
	"fmt"
	"math"
	"sort"

	"github.com/reconcilyx/operator/api/v1alpha1"
	"github.com/reconcilyx/operator/internal/engine"
	"github.com/reconcilyx/operator/internal/telemetry"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

type CPUOverprovisionedRule struct{}

func (r *CPUOverprovisionedRule) Name() string {
	return "CPUOverprovisioned"
}

func (r *CPUOverprovisionedRule) Description() string {
	return "Detects containers where CPU requests significantly exceed p95 usage + 15% headroom"
}

func (r *CPUOverprovisionedRule) Evaluate(containerName string, metrics *telemetry.ContainerMetrics, currentResources corev1.ResourceRequirements) (*engine.Finding, error) {
	// Never downsize containers experiencing OOMKills or frequent CrashLoop restarts.
	if metrics.Memory.OOMCount > 0 || metrics.Restarts >= 3 {
		return nil, nil
	}

	reqQuantity, ok := currentResources.Requests[corev1.ResourceCPU]
	if !ok {
		return nil, nil
	}

	requestedCores := float64(reqQuantity.MilliValue()) / 1000.0
	baseCores := effectiveCPUP95(metrics.CPU)

	// Asymmetric CPU policy: p95 usage + 15% headroom.
	targetCores := baseCores * 1.15

	if requestedCores > targetCores && (requestedCores-targetCores) > 0.05 {
		milliCores := int64(math.Round(targetCores * 1000))
		if milliCores < 10 {
			milliCores = 10 // enforce minimum 10m
		}
		proposedReq := resource.MustParse(fmt.Sprintf("%dm", milliCores))

		return &engine.Finding{
			RuleName:       r.Name(),
			ContainerName:  containerName,
			Severity:       1,
			Recommendation: v1alpha1.RecommendationRightSize,
			ProposedRequest: &corev1.ResourceList{
				corev1.ResourceCPU: proposedReq,
			},
			Rationale: fmt.Sprintf("CPU requested (%.2f) exceeds p95 usage (%.2f). Proposing %dm with 15%% headroom.", requestedCores, baseCores, milliCores),
		}, nil
	}

	return nil, nil
}

func effectiveCPUP95(cpu telemetry.CPUMetrics) float64 {
	if cpu.P95Cores > 0 {
		return cpu.P95Cores
	}
	if len(cpu.HistoricalCores) > 0 {
		sorted := append([]float64(nil), cpu.HistoricalCores...)
		sort.Float64s(sorted)
		idx := int(math.Ceil(0.95*float64(len(sorted)))) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= len(sorted) {
			idx = len(sorted) - 1
		}
		return sorted[idx]
	}
	return cpu.UsageCores
}
