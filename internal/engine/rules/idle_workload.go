package rules

import (
	"fmt"

	"github.com/reconcilyx/operator/api/v1alpha1"
	"github.com/reconcilyx/operator/internal/engine"
	"github.com/reconcilyx/operator/internal/telemetry"
	corev1 "k8s.io/api/core/v1"
)

type IdleWorkloadRule struct{}

func (r *IdleWorkloadRule) Name() string {
	return "IdleWorkload"
}

func (r *IdleWorkloadRule) Description() string {
	return "Detects containers doing virtually no work"
}

func (r *IdleWorkloadRule) Evaluate(containerName string, metrics *telemetry.ContainerMetrics, currentResources corev1.ResourceRequirements) (*engine.Finding, error) {
	// A workload is considered idle if CPU < 0.01 cores and Memory < 50MB
	if metrics.CPU.UsageCores < 0.01 && metrics.Memory.WorkingSetBytes < (50*1024*1024) {
		return &engine.Finding{
			RuleName:       r.Name(),
			ContainerName:  containerName,
			Severity:       3, // High severity, absolute waste
			Recommendation: v1alpha1.RecommendationScaleZero,
			Rationale:      fmt.Sprintf("Workload appears completely idle (CPU: %.3f, Mem: %.0fMB)", metrics.CPU.UsageCores, metrics.Memory.WorkingSetBytes/(1024*1024)),
		}, nil
	}

	return nil, nil
}
