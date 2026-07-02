package engine_test

import (
	"context"
	"testing"

	"github.com/reconcilyx/operator/api/v1alpha1"
	"github.com/reconcilyx/operator/internal/engine"
	"github.com/reconcilyx/operator/internal/engine/forecasting"
	"github.com/reconcilyx/operator/internal/engine/rules"
	"github.com/reconcilyx/operator/internal/telemetry"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

type mockTelemetryProvider struct {
	metrics *telemetry.WorkloadMetrics
	err     error
}

func (m *mockTelemetryProvider) GetWorkloadMetrics(ctx context.Context, ref corev1.ObjectReference) (*telemetry.WorkloadMetrics, error) {
	return m.metrics, m.err
}

func TestAnalyzer_Analyze(t *testing.T) {
	registry := engine.NewRuleRegistry()
	registry.Register(&rules.CPUOverprovisionedRule{})
	registry.Register(&rules.MemOverprovisionedRule{})
	registry.Register(&rules.IdleWorkloadRule{})

	scorer := engine.NewScorer()

	t.Run("Generates Proposal for Overprovisioned Workload", func(t *testing.T) {
		provider := &mockTelemetryProvider{
			metrics: &telemetry.WorkloadMetrics{
				Replicas: 2,
				Containers: map[string]*telemetry.ContainerMetrics{
					"app": {
						Name: "app",
						CPU: telemetry.CPUMetrics{
							UsageCores: 0.5,
						},
						Memory: telemetry.MemoryMetrics{
							WorkingSetBytes: 512 * 1024 * 1024,
						},
					},
				},
			},
		}

		analyzer := engine.NewAnalyzer(provider, registry, scorer, forecasting.NewPredictor())

		currentResources := map[string]corev1.ResourceRequirements{
			"app": {
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2000m"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
			},
		}

		proposal, err := analyzer.Analyze(context.Background(), corev1.ObjectReference{Name: "test-deploy"}, currentResources)
		
		assert.NoError(t, err)
		assert.NotNil(t, proposal)
		
		assert.Equal(t, v1alpha1.RecommendationRightSize, proposal.Spec.Recommendation)
		
		// CPU target is 0.5 * 1.2 = 0.6 cores (600m)
		assert.Equal(t, int64(600), proposal.Spec.ProposedResources.Requests.Cpu().MilliValue())
		
		// Memory target is 512 * 1.2 = 614.4 MB (approx 614Mi)
		assert.Equal(t, int64(614), proposal.Spec.ProposedResources.Requests.Memory().Value() / (1024*1024))

		// Check positive savings
		assert.True(t, proposal.Spec.ExpectedImpact.EstimatedSavingsUSD.Value() > 0)
	})

	t.Run("No Optimization Needed", func(t *testing.T) {
		provider := &mockTelemetryProvider{
			metrics: &telemetry.WorkloadMetrics{
				Containers: map[string]*telemetry.ContainerMetrics{
					"app": {
						Name: "app",
						CPU: telemetry.CPUMetrics{UsageCores: 1.8}, // closely matches 2000m
					},
				},
			},
		}

		analyzer := engine.NewAnalyzer(provider, registry, scorer, forecasting.NewPredictor())
		currentResources := map[string]corev1.ResourceRequirements{
			"app": {
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("2000m"),
				},
			},
		}

		proposal, err := analyzer.Analyze(context.Background(), corev1.ObjectReference{Name: "test-deploy"}, currentResources)
		assert.NoError(t, err)
		assert.Nil(t, proposal) // Expect nil proposal if no rules fired
	})
}
