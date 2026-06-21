package rules

import (
	"fmt"

	"github.com/reconcilyx/operator/api/v1alpha1"
	"github.com/reconcilyx/operator/internal/engine"
	"github.com/reconcilyx/operator/internal/telemetry"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

type MemOverprovisionedRule struct{}

func (r *MemOverprovisionedRule) Name() string {
	return "MemoryOverprovisioned"
}

func (r *MemOverprovisionedRule) Description() string {
	return "Detects containers where Memory requests are significantly higher than working set"
}

func (r *MemOverprovisionedRule) Evaluate(containerName string, metrics *telemetry.ContainerMetrics, currentResources corev1.ResourceRequirements) (*engine.Finding, error) {
	reqQuantity, ok := currentResources.Requests[corev1.ResourceMemory]
	if !ok {
		return nil, nil
	}

	requestedBytes := float64(reqQuantity.Value())
	
	// We only optimize if usage + 20% buffer is still less than requested.
	targetBytes := metrics.Memory.WorkingSetBytes * 1.2

	if requestedBytes > targetBytes && (requestedBytes-targetBytes) > (10*1024*1024) { // Only if savings > 10MB
		mb := int64(targetBytes / (1024 * 1024))
		if mb < 10 {
			mb = 10 // enforce minimum 10Mi
		}
		proposedReq := resource.MustParse(fmt.Sprintf("%dMi", mb))

		return &engine.Finding{
			RuleName:       r.Name(),
			ContainerName:  containerName,
			Severity:       1,
			Recommendation: v1alpha1.RecommendationRightSize,
			ProposedRequest: &corev1.ResourceList{
				corev1.ResourceMemory: proposedReq,
			},
			Rationale: fmt.Sprintf("Memory requested (%.0fMB) exceeds usage (%.0fMB). Proposing %dMi with 20%% buffer.", requestedBytes/(1024*1024), metrics.Memory.WorkingSetBytes/(1024*1024), mb),
		}, nil
	}

	return nil, nil
}
