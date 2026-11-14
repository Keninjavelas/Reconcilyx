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

type MemOverprovisionedRule struct{}

func (r *MemOverprovisionedRule) Name() string {
	return "MemoryOverprovisioned"
}

func (r *MemOverprovisionedRule) Description() string {
	return "Detects memory overprovisioning relative to p99 working set + 25% headroom and guards against OOMKills"
}

func (r *MemOverprovisionedRule) Evaluate(containerName string, metrics *telemetry.ContainerMetrics, currentResources corev1.ResourceRequirements) (*engine.Finding, error) {
	reqQuantity, hasReq := currentResources.Requests[corev1.ResourceMemory]
	limQuantity, hasLim := currentResources.Limits[corev1.ResourceMemory]

	requestedBytes := float64(reqQuantity.Value())
	limitedBytes := float64(limQuantity.Value())
	baseBytes := effectiveMemoryP99(metrics.Memory)

	// Guard: Never downsize containers with recent OOMKill or frequent restarts.
	// When OOMKills are observed, propose an upward adjustment (+25% headroom).
	if metrics.Memory.OOMCount > 0 {
		baseline := requestedBytes
		if hasLim && limitedBytes > baseline {
			baseline = limitedBytes
		}
		if baseBytes > baseline {
			baseline = baseBytes
		}
		if baseline <= 0 {
			baseline = 64 * 1024 * 1024
		}
		upsizedBytes := baseline * 1.25
		mb := int64(math.Ceil(upsizedBytes / (1024 * 1024)))
		if mb < 32 {
			mb = 32
		}
		proposedMem := resource.MustParse(fmt.Sprintf("%dMi", mb))
		return &engine.Finding{
			RuleName:       r.Name(),
			ContainerName:  containerName,
			Severity:       3,
			Recommendation: v1alpha1.RecommendationRightSize,
			ProposedRequest: &corev1.ResourceList{
				corev1.ResourceMemory: proposedMem.DeepCopy(),
			},
			ProposedLimit: &corev1.ResourceList{
				corev1.ResourceMemory: proposedMem.DeepCopy(),
			},
			Rationale: fmt.Sprintf("Detected %d OOMKill event(s); refusing downward sizing and proposing %dMi (+25%% headroom) on requests and limits.", metrics.Memory.OOMCount, mb),
		}, nil
	}

	if metrics.Restarts >= 3 {
		return nil, nil
	}

	if !hasReq {
		return nil, nil
	}

	// Asymmetric Memory policy: p99 working set + 25% headroom on both requests and limits.
	targetBytes := baseBytes * 1.25

	if requestedBytes > targetBytes && (requestedBytes-targetBytes) > (10*1024*1024) {
		mb := int64(math.Ceil(targetBytes / (1024 * 1024)))
		if mb < 10 {
			mb = 10 // enforce minimum 10Mi
		}
		proposedMem := resource.MustParse(fmt.Sprintf("%dMi", mb))

		return &engine.Finding{
			RuleName:       r.Name(),
			ContainerName:  containerName,
			Severity:       1,
			Recommendation: v1alpha1.RecommendationRightSize,
			ProposedRequest: &corev1.ResourceList{
				corev1.ResourceMemory: proposedMem.DeepCopy(),
			},
			ProposedLimit: &corev1.ResourceList{
				corev1.ResourceMemory: proposedMem.DeepCopy(),
			},
			Rationale: fmt.Sprintf("Memory requested (%.0fMB) exceeds p99 working set (%.0fMB). Proposing %dMi (+25%% headroom) on both requests and limits.", requestedBytes/(1024*1024), baseBytes/(1024*1024), mb),
		}, nil
	}

	return nil, nil
}

func effectiveMemoryP99(mem telemetry.MemoryMetrics) float64 {
	if mem.P99WorkingSetBytes > 0 {
		return mem.P99WorkingSetBytes
	}
	if len(mem.HistoricalWorkingSetBytes) > 0 {
		sorted := make([]float64, len(mem.HistoricalWorkingSetBytes))
		for i, v := range mem.HistoricalWorkingSetBytes {
			sorted[i] = float64(v)
		}
		sort.Float64s(sorted)
		idx := int(math.Ceil(0.99*float64(len(sorted)))) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= len(sorted) {
			idx = len(sorted) - 1
		}
		return sorted[idx]
	}
	return mem.WorkingSetBytes
}
