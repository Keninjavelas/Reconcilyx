package rules

import (
	"fmt"

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
	return "Detects containers where CPU requests are significantly higher than usage"
}

func (r *CPUOverprovisionedRule) Evaluate(containerName string, metrics *telemetry.ContainerMetrics, currentResources corev1.ResourceRequirements) (*engine.Finding, error) {
	// If no requested CPU is defined, we can't optimize an overprovisioning issue.
	reqQuantity, ok := currentResources.Requests[corev1.ResourceCPU]
	if !ok {
		return nil, nil
	}

	requestedCores := float64(reqQuantity.MilliValue()) / 1000.0

	// We only optimize if usage + 20% buffer is still less than requested.
	targetCores := metrics.CPU.UsageCores * 1.2

	if requestedCores > targetCores && (requestedCores-targetCores) > 0.05 {
		// Calculate new request: targetCores (convert to milli, then quantity)
		milliCores := int64(targetCores * 1000)
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
			Rationale: fmt.Sprintf("CPU requested (%.2f) exceeds usage (%.2f). Proposing %.2f with 20%% buffer.", requestedCores, metrics.CPU.UsageCores, targetCores),
		}, nil
	}

	return nil, nil
}
