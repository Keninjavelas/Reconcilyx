package engine_test

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/reconcilyx/operator/api/v1alpha1"
	"github.com/reconcilyx/operator/internal/engine"
	"github.com/reconcilyx/operator/internal/engine/forecasting"
	"github.com/reconcilyx/operator/internal/engine/rl"
	"github.com/reconcilyx/operator/internal/engine/rules"
	"github.com/reconcilyx/operator/internal/federated"
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

type mockMetricsClient struct{}

func (m *mockMetricsClient) Query(ctx context.Context, query string) (model.Value, error) {
	return nil, nil
}

func (m *mockMetricsClient) QueryRange(ctx context.Context, query string, r time.Duration) (model.Value, error) {
	return nil, nil
}

func (m *mockMetricsClient) HealthCheck(ctx context.Context) error {
	return nil
}

type mockRLAgent struct{}

func (m *mockRLAgent) Recommend(ctx context.Context, state *rl.WorkloadState) (*rl.RecommendationResult, error) {
	return &rl.RecommendationResult{
		Action: &rl.Action{
			Adjustment: map[string]rl.ResourceAdjustment{}, // No adjustments
		},
		Confidence: 0.0,
	}, nil
}

func (m *mockRLAgent) Update(ctx context.Context, reward float64, action *rl.Action, previousState *rl.WorkloadState, nextState *rl.WorkloadState) error {
	return nil
}

func (m *mockRLAgent) LoadGlobal(_ context.Context, _ *federated.GlobalQTable) {}
func (m *mockRLAgent) SetFederatedEmitter(_ *federated.Emitter)          {}

func TestAnalyzer_Analyze(t *testing.T) {
	registry := engine.NewRuleRegistry()
	registry.Register(&rules.CPUOverprovisionedRule{})
	registry.Register(&rules.MemOverprovisionedRule{})
	registry.Register(&rules.IdleWorkloadRule{})

	scorer := engine.NewScorer()
	mockClient := &mockMetricsClient{}
	mockAgent := &mockRLAgent{}

	t.Run("Generates_Proposal_for_Overprovisioned_Workload", func(t *testing.T) {
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

		analyzer := engine.NewAnalyzer(provider, registry, scorer, forecasting.NewPredictor(mockClient), engine.WithRLAgent(mockAgent))

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
		// Carbon savings should be populated (non-nil) when resource reduction is positive
		assert.NotNil(t, proposal.Spec.ExpectedImpact.CarbonSavedGramsCO2)
		assert.True(t, proposal.Spec.ExpectedImpact.CarbonSavedGramsCO2.Value() > 0)
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

		analyzer := engine.NewAnalyzer(provider, registry, scorer, forecasting.NewPredictor(mockClient), engine.WithRLAgent(mockAgent))
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
