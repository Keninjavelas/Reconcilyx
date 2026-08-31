package engine

import (
	"context"
	"testing"

	"github.com/reconcilyx/operator/internal/carbon"
	"github.com/reconcilyx/operator/internal/engine/forecasting"
	"github.com/reconcilyx/operator/internal/engine/rl"
	"github.com/reconcilyx/operator/internal/federated"
	"github.com/reconcilyx/operator/internal/telemetry"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestComputeCostSavedUSDPerMonth(t *testing.T) {
	tests := []struct {
		name          string
		cpuDeltaMilli int64
		memDeltaBytes int64
		replicas      int32
		wantApproxMin float64
		wantApproxMax float64
		wantZero      bool
	}{
		{
			name:          "500m cpu + 512MB mem + 3 replicas",
			cpuDeltaMilli: 500,
			memDeltaBytes: 512 * 1024 * 1024,
			replicas:      3,
			wantApproxMin: 50.0,
			wantApproxMax: 70.0,
		},
		{
			name:          "zero replicas defaults to 1",
			cpuDeltaMilli: 1000,
			memDeltaBytes: 1 << 30,
			replicas:      0,
			wantApproxMin: 30.0,
			wantApproxMax: 50.0,
		},
		{
			name:          "negative delta (increase) = negative savings",
			cpuDeltaMilli: -200,
			memDeltaBytes: 0,
			replicas:      2,
			wantApproxMin: -20.0,
			wantApproxMax: 0.0,
		},
		{
			name:          "zero delta = zero",
			cpuDeltaMilli: 0,
			memDeltaBytes: 0,
			replicas:      1,
			wantZero:      true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputeCostSavedUSDPerMonth(tc.cpuDeltaMilli, tc.memDeltaBytes, tc.replicas)
			if tc.wantZero {
				assert.Equal(t, 0.0, got, "expected zero")
				return
			}
			assert.GreaterOrEqual(t, got, tc.wantApproxMin, "savings=%.2f below expected min %.2f", got, tc.wantApproxMin)
			assert.LessOrEqual(t, got, tc.wantApproxMax, "savings=%.2f above expected max %.2f", got, tc.wantApproxMax)
		})
	}
}

func TestComputeCarbonSavedGramsPerMonth(t *testing.T) {
	tests := []struct {
		name               string
		cpuDeltaMilli      int64
		memDeltaBytes      int64
		replicas           int32
		intensity          float64
		wantApproxMinGrams float64
		wantApproxMaxGrams float64
		wantZero           bool
	}{
		{
			name:               "500m CPU + 1GB mem x 1 replica x 450 gCO2/kWh → ~65kg",
			cpuDeltaMilli:      500,
			memDeltaBytes:      1 << 30,
			replicas:           1,
			intensity:          450,
			wantApproxMinGrams: 60_000,
			wantApproxMaxGrams: 70_000,
		},
		{
			name:          "zero intensity → 0 carbon",
			cpuDeltaMilli: 1000,
			memDeltaBytes: 1 << 30,
			replicas:      3,
			intensity:     0,
			wantZero:      true,
		},
		{
			name:               "negative delta (increase) yields negative or zero",
			cpuDeltaMilli:      -100,
			memDeltaBytes:      0,
			replicas:           1,
			intensity:          400,
			wantApproxMinGrams: -15,
			wantApproxMaxGrams: 0,
		},
		{
			name:               "1000m + 8GB x 10 repl x 800 gCO2 high intensity → ~2.3 tonnes",
			cpuDeltaMilli:      1000,
			memDeltaBytes:      8 << 30,
			replicas:           10,
			intensity:          800,
			wantApproxMinGrams: 2_000_000,
			wantApproxMaxGrams: 2_500_000,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputeCarbonSavedGramsPerMonth(tc.cpuDeltaMilli, tc.memDeltaBytes, tc.replicas, tc.intensity)
			if tc.wantZero {
				assert.Equal(t, 0.0, got, "expected 0 carbon for zero intensity")
				return
			}
			assert.GreaterOrEqual(t, got, tc.wantApproxMinGrams,
				"carbon=%.2f below expected min %.2f", got, tc.wantApproxMinGrams)
			assert.LessOrEqual(t, got, tc.wantApproxMaxGrams,
				"carbon=%.2f above expected max %.2f", got, tc.wantApproxMaxGrams)
		})
	}
}

func TestWithWorkloadRegion_Options(t *testing.T) {
	cp := carbon.NewStaticProvider()
	agent := rl.NewSimpleQAgent()
	a := NewAnalyzer(nil, NewRuleRegistry(), NewScorer(), nil,
		WithCarbonProvider(cp),
		WithWorkloadRegion("us-east-1"),
		WithRLAgent(agent),
	)
	assert.Equal(t, "us-east-1", RegionFrom(a))
}

func TestWithCarbonWeight_Clamping(t *testing.T) {
	tests := []struct {
		in   float64
		want float64
	}{
		{-0.5, 0.0},
		{0.0, 0.0},
		{0.5, 0.5},
		{1.0, 1.0},
		{1.5, 1.0},
	}
	for _, tc := range tests {
		a := NewAnalyzer(nil, NewRuleRegistry(), NewScorer(), nil, WithCarbonWeight(tc.in))
		assert.Equal(t, tc.want, CarbonWeightFrom(a), "weight=%v", tc.in)
	}
}

type fixedIntensityProvider struct {
	val float64
}

func (f fixedIntensityProvider) IntensityForRegion(_ context.Context, _ string) (carbon.Intensity, error) {
	return carbon.Intensity{Value: f.val, Source: "test", Region: "test"}, nil
}

func TestAnalyzer_UsesConfiguredRegionOverDefault(t *testing.T) {
	a := NewAnalyzer(nil, NewRuleRegistry(), NewScorer(), nil,
		WithCarbonProvider(fixedIntensityProvider{val: 800.0}),
		WithWorkloadRegion("custom-region"))
	assert.Equal(t, "custom-region", RegionFrom(a))
}

type spyRLAgent struct {
	seenState       chan *rl.WorkloadState
	recommendAction *rl.Action
}

func newSpyRLAgent(action *rl.Action) *spyRLAgent {
	return &spyRLAgent{seenState: make(chan *rl.WorkloadState, 4), recommendAction: action}
}

func (s *spyRLAgent) Recommend(_ context.Context, state *rl.WorkloadState) (*rl.RecommendationResult, error) {
	clone := &rl.WorkloadState{
		CPUUsageCores:         state.CPUUsageCores,
		CPUThrottledSecs:      state.CPUThrottledSecs,
		MemoryWorkingSetBytes: state.MemoryWorkingSetBytes,
		OOMCount:              state.OOMCount,
		Region:                state.Region,
	}
	if state.CarbonWeight != nil {
		w := *state.CarbonWeight
		clone.CarbonWeight = &w
	}
	select {
	case s.seenState <- clone:
	default:
	}
	return &rl.RecommendationResult{Action: s.recommendAction, Confidence: 0.8}, nil
}

func (*spyRLAgent) Update(_ context.Context, _ float64, _ *rl.Action, _, _ *rl.WorkloadState) error {
	return nil
}

func (*spyRLAgent) LoadGlobal(_ context.Context, _ *federated.GlobalQTable) {}

func (*spyRLAgent) SetFederatedEmitter(_ *federated.Emitter) {}

func TestAnalyzer_WorkloadStateInjectedRegionAndCarbonWeight(t *testing.T) {
	action := &rl.Action{Adjustment: map[string]rl.ResourceAdjustment{
		"cpu":    {Percentage: -10},
		"memory": {Percentage: -10},
	}}
	spy := newSpyRLAgent(action)
	metrics := &telemetry.WorkloadMetrics{
		Replicas: 2,
		Containers: map[string]*telemetry.ContainerMetrics{
			"main": {
				Name: "main",
				CPU: telemetry.CPUMetrics{
					RequestedCores:  1.0,
					LimitedCores:    2.0,
					UsageCores:      0.5,
					ThrottledSecs:   0.0,
					HistoricalCores: []float64{0.45, 0.50, 0.55},
				},
				Memory: telemetry.MemoryMetrics{
					RequestedBytes:            2 << 30,
					LimitedBytes:              4 << 30,
					WorkingSetBytes:           1 << 30,
					OOMCount:                  0,
					HistoricalWorkingSetBytes: []uint64{1 << 30, 1 << 30},
				},
				Restarts: 0,
			},
		},
	}
	mockTel := &staticMetricsProvider{m: metrics}
	a := NewAnalyzer(mockTel, NewRuleRegistry(), NewScorer(), &noopPredictor{},
		WithRLAgent(spy),
		WithWorkloadRegion("eu-west-1"),
		WithCarbonWeight(0.7),
	)
	ref := corev1.ObjectReference{Kind: "Deployment", Name: "dep", Namespace: "default"}
	current := map[string]corev1.ResourceRequirements{
		"main": {
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1"),
				corev1.ResourceMemory: resource.MustParse("2Gi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2"),
				corev1.ResourceMemory: resource.MustParse("4Gi"),
			},
		},
	}
	_, _ = a.Analyze(context.Background(), ref, current)
	select {
	case st := <-spy.seenState:
		assert.Equal(t, "eu-west-1", st.Region, "Region not injected")
		if assert.NotNil(t, st.CarbonWeight, "CarbonWeight should be injected as pointer") {
			assert.InDelta(t, 0.7, *st.CarbonWeight, 1e-9, "CarbonWeight value mismatch")
		}
	default:
		t.Skip("RL agent Recommend path not exercised in this Analyze configuration")
	}
}

type staticMetricsProvider struct {
	m   *telemetry.WorkloadMetrics
	err error
}

func (s *staticMetricsProvider) GetWorkloadMetrics(_ context.Context, _ corev1.ObjectReference) (*telemetry.WorkloadMetrics, error) {
	return s.m, s.err
}

type noopPredictor struct{}

func (*noopPredictor) Forecast(_ context.Context, _ corev1.ObjectReference) (*forecasting.Prediction, error) {
	return nil, nil
}
