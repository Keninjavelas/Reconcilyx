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
func (m *mockRLAgent) SetFederatedEmitter(_ *federated.Emitter)                {}

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

		// Asymmetric CPU target: 0.5 * 1.15 = 0.575 cores (575m)
		assert.Equal(t, int64(575), proposal.Spec.ProposedResources.Requests.Cpu().MilliValue())

		// Asymmetric Memory target: 512 * 1.25 = 640Mi on BOTH requests and limits
		assert.Equal(t, int64(640), proposal.Spec.ProposedResources.Requests.Memory().Value()/(1024*1024))
		assert.Equal(t, int64(640), proposal.Spec.ProposedResources.Limits.Memory().Value()/(1024*1024))

		byContainer, ok := proposal.Spec.ProposedResourcesByContainer["app"]
		assert.True(t, ok)
		assert.Equal(t, int64(575), byContainer.Requests.Cpu().MilliValue())
		assert.Equal(t, int64(640), byContainer.Limits.Memory().Value()/(1024*1024))
		_, hasCPULimit := byContainer.Limits[corev1.ResourceCPU]
		assert.False(t, hasCPULimit, "omitCpuLimits policy should avoid setting CPU limits")

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
						CPU:  telemetry.CPUMetrics{UsageCores: 1.8}, // closely matches 2000m
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

	t.Run("OmitCPULimits strips existing CPU limits while keeping Memory symmetry", func(t *testing.T) {
		provider := &mockTelemetryProvider{
			metrics: &telemetry.WorkloadMetrics{
				Replicas: 1,
				Containers: map[string]*telemetry.ContainerMetrics{
					"app": {
						Name:   "app",
						CPU:    telemetry.CPUMetrics{P95Cores: 0.4},
						Memory: telemetry.MemoryMetrics{P99WorkingSetBytes: 256 * 1024 * 1024},
					},
				},
			},
		}
		analyzer := engine.NewAnalyzer(provider, registry, scorer, forecasting.NewPredictor(mockClient), engine.WithRLAgent(mockAgent), engine.WithOmitCPULimits(true))
		currentResources := map[string]corev1.ResourceRequirements{
			"app": {
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2000m"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4000m"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
			},
		}
		proposal, err := analyzer.Analyze(context.Background(), corev1.ObjectReference{Name: "test-deploy"}, currentResources)
		assert.NoError(t, err)
		assert.NotNil(t, proposal)
		appSpec := proposal.Spec.ProposedResourcesByContainer["app"]
		_, hasCPULim := appSpec.Limits[corev1.ResourceCPU]
		assert.False(t, hasCPULim)
		assert.Equal(t, int64(320*1024*1024), appSpec.Limits.Memory().Value())
		assert.Contains(t, proposal.Spec.DecisionMetadata.Rationale, "omitCpuLimits")
	})
}
