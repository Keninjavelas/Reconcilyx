package controller

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	optimizationv1alpha1 "github.com/reconcilyx/operator/api/v1alpha1"
	"github.com/reconcilyx/operator/internal/engine/rl"
	"github.com/reconcilyx/operator/internal/federated"
	"github.com/reconcilyx/operator/internal/telemetry"
)

type stubTelemetryProvider struct {
	metrics *telemetry.WorkloadMetrics
	err     error
	calls   int
}

func (s *stubTelemetryProvider) GetWorkloadMetrics(context.Context, corev1.ObjectReference) (*telemetry.WorkloadMetrics, error) {
	s.calls++
	return s.metrics, s.err
}

type updateCall struct {
	reward float64
	action *rl.Action
	prev   *rl.WorkloadState
	next   *rl.WorkloadState
}

type spyAgent struct {
	updates []updateCall
	err     error
}

func (s *spyAgent) Recommend(context.Context, *rl.WorkloadState) (*rl.RecommendationResult, error) {
	return &rl.RecommendationResult{}, nil
}

func (s *spyAgent) Update(_ context.Context, reward float64, action *rl.Action, prev, next *rl.WorkloadState) error {
	s.updates = append(s.updates, updateCall{reward: reward, action: action, prev: prev, next: next})
	return s.err
}

func (s *spyAgent) LoadGlobal(context.Context, *federated.GlobalQTable) {}
func (s *spyAgent) SetFederatedEmitter(*federated.Emitter)              {}

func learningTestProposal(phase optimizationv1alpha1.OptimizationProposalPhase) *optimizationv1alpha1.OptimizationProposal {
	cpu := resource.MustParse("500m")
	mem := resource.MustParse("512Mi")
	proposedCPU := resource.MustParse("400m")
	return &optimizationv1alpha1.OptimizationProposal{
		ObjectMeta: metav1.ObjectMeta{Name: "test-proposal", Namespace: "default"},
		Spec: optimizationv1alpha1.OptimizationProposalSpec{
			TargetWorkload: corev1.ObjectReference{Kind: "Deployment", Name: "api", Namespace: "default"},
			Recommendation: optimizationv1alpha1.RecommendationRightSize,
			ProposedResources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    proposedCPU,
					corev1.ResourceMemory: mem,
				},
			},
		},
		Status: optimizationv1alpha1.OptimizationProposalStatus{
			Phase: phase,
			ExecutionSnapshot: &optimizationv1alpha1.ExecutionSnapshot{
				ReplicaCount: 2,
				ContainerResources: map[string]optimizationv1alpha1.ContainerResourceState{
					"app": {
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    cpu,
								corev1.ResourceMemory: mem,
							},
						},
					},
				},
			},
			Learning: &optimizationv1alpha1.LearningRecord{
				Pre: map[string]optimizationv1alpha1.LearningContainerState{
					"app": {CPUUsageCores: 0.21, MemoryWorkingSetBytes: 300 * 1024 * 1024},
				},
			},
		},
	}
}

func testPostMetrics() *telemetry.WorkloadMetrics {
	return &telemetry.WorkloadMetrics{
		Replicas: 2,
		Containers: map[string]*telemetry.ContainerMetrics{
			"app": {
				CPU:    telemetry.CPUMetrics{UsageCores: 0.19},
				Memory: telemetry.MemoryMetrics{WorkingSetBytes: 290 * 1024 * 1024},
			},
		},
	}
}

func TestApplyRLLearningRecordsReward(t *testing.T) {
	agent := &spyAgent{}
	provider := &stubTelemetryProvider{metrics: testPostMetrics()}
	reconciler := &OptimizationProposalReconciler{RLAgent: agent, TelemetryProvider: provider}
	proposal := learningTestProposal(optimizationv1alpha1.PhaseCompleted)

	if err := reconciler.applyRLLearning(context.Background(), proposal); err != nil {
		t.Fatalf("applyRLLearning returned error: %v", err)
	}

	if len(agent.updates) != 1 {
		t.Fatalf("expected 1 agent update, got %d", len(agent.updates))
	}
	update := agent.updates[0]
	if update.reward <= 0 {
		t.Fatalf("expected positive reward, got %f", update.reward)
	}
	if got := update.action.Adjustment[string(corev1.ResourceCPU)].Percentage; got != -20 {
		t.Fatalf("expected -20 CPU adjustment, got %v", got)
	}
	if got := update.action.Adjustment[string(corev1.ResourceMemory)].Percentage; got != 0 {
		t.Fatalf("expected 0 memory adjustment, got %v", got)
	}
	if update.prev.JVMBucket != 0 || update.prev.GPUBucket != 0 {
		t.Fatalf("expected pre-state buckets to be preserved, got jvm=%d gpu=%d", update.prev.JVMBucket, update.prev.GPUBucket)
	}

	record := proposal.Status.Learning
	if !record.Finalized || record.Outcome != learningOutcomeCompleted {
		t.Fatalf("expected finalized completed record, got %+v", record)
	}
	if record.Reward != update.reward {
		t.Fatalf("expected persisted reward %f, got %f", update.reward, record.Reward)
	}
	result := record.Results["app"]
	if result.CostSavedUSD == nil || result.CostSavedUSD.IsZero() {
		t.Fatalf("expected persisted cost savings, got %+v", result.CostSavedUSD)
	}
	if result.Post.CPUUsageCores != 0.19 {
		t.Fatalf("expected post CPU usage persisted, got %v", result.Post.CPUUsageCores)
	}
}

func TestApplyRLLearningIsIdempotent(t *testing.T) {
	agent := &spyAgent{}
	reconciler := &OptimizationProposalReconciler{
		RLAgent:           agent,
		TelemetryProvider: &stubTelemetryProvider{metrics: testPostMetrics()},
	}
	proposal := learningTestProposal(optimizationv1alpha1.PhaseCompleted)

	if err := reconciler.applyRLLearning(context.Background(), proposal); err != nil {
		t.Fatalf("first update failed: %v", err)
	}
	if err := reconciler.applyRLLearning(context.Background(), proposal); err != nil {
		t.Fatalf("second update failed: %v", err)
	}
	if len(agent.updates) != 1 {
		t.Fatalf("expected idempotent single update, got %d", len(agent.updates))
	}
}

func TestApplyRLLearningRollbackPenalizesAttemptedAction(t *testing.T) {
	agent := &spyAgent{}
	reconciler := &OptimizationProposalReconciler{
		RLAgent:           agent,
		TelemetryProvider: &stubTelemetryProvider{metrics: testPostMetrics()},
	}
	proposal := learningTestProposal(optimizationv1alpha1.PhaseRolledBack)

	if err := reconciler.applyRLLearning(context.Background(), proposal); err != nil {
		t.Fatalf("applyRLLearning returned error: %v", err)
	}
	if len(agent.updates) != 1 {
		t.Fatalf("expected 1 agent update, got %d", len(agent.updates))
	}
	update := agent.updates[0]
	if update.reward != -1.0 {
		t.Fatalf("expected rollback reward -1, got %f", update.reward)
	}
	if got := update.action.Adjustment[string(corev1.ResourceCPU)].Percentage; got != -20 {
		t.Fatalf("expected attempted -20 CPU action to be penalized, got %v", got)
	}
	if got := update.next.CurrentCPURequest.MilliValue(); got != 500 {
		t.Fatalf("expected rollback next state to restore 500m, got %dm", got)
	}
	if proposal.Status.Learning.Outcome != learningOutcomeRollback {
		t.Fatalf("expected rollback outcome, got %q", proposal.Status.Learning.Outcome)
	}
}

func TestApplyRLLearningFinalizesLegacyProposalWithoutPreState(t *testing.T) {
	agent := &spyAgent{}
	reconciler := &OptimizationProposalReconciler{RLAgent: agent}
	proposal := learningTestProposal(optimizationv1alpha1.PhaseCompleted)
	proposal.Status.Learning = nil

	if err := reconciler.applyRLLearning(context.Background(), proposal); err != nil {
		t.Fatalf("applyRLLearning returned error: %v", err)
	}
	if len(agent.updates) != 0 {
		t.Fatalf("expected no update without pre-state, got %d", len(agent.updates))
	}
	if proposal.Status.Learning == nil || !proposal.Status.Learning.Finalized || proposal.Status.Learning.SkipReason == "" {
		t.Fatalf("expected finalized skip record, got %+v", proposal.Status.Learning)
	}
}

func TestCaptureLearningPreStatePopulatesBuckets(t *testing.T) {
	metrics := &telemetry.WorkloadMetrics{
		Containers: map[string]*telemetry.ContainerMetrics{
			"app": {
				CPU:    telemetry.CPUMetrics{UsageCores: 0.35, ThrottledSecs: 1.5},
				Memory: telemetry.MemoryMetrics{WorkingSetBytes: 1024 * 1024 * 1024, OOMCount: 1},
				JVM:    telemetry.JVMHeapMetrics{IsJVMWorkload: true, XmxBytes: 1024 * 1024 * 1024},
				GPU:    telemetry.GPUMetrics{GPUCount: 1, GPUUtilizationPct: 14.5},
			},
		},
	}
	reconciler := &OptimizationProposalReconciler{TelemetryProvider: &stubTelemetryProvider{metrics: metrics}}
	proposal := learningTestProposal(optimizationv1alpha1.PhaseExecuting)
	proposal.Status.Learning = nil

	record := reconciler.captureLearningPreState(context.Background(), proposal)
	state := record.Pre["app"]
	if state.JVMBucket != rl.JVMBucketMedium {
		t.Fatalf("expected JVM medium bucket, got %d", state.JVMBucket)
	}
	if state.GPUBucket != rl.GPUBucketIdle {
		t.Fatalf("expected GPU idle bucket, got %d", state.GPUBucket)
	}
	if state.OOMCount != 1 || state.CPUThrottledSeconds != 1.5 {
		t.Fatalf("expected throttling and OOM inputs persisted, got %+v", state)
	}
}

func TestAdjustmentPercentSnapsToActionGrid(t *testing.T) {
	cases := []struct {
		prev, next string
		want       float64
	}{
		{"500m", "400m", -20},
		{"500m", "450m", -10},
		{"500m", "500m", 0},
		{"500m", "560m", 10},
		{"500m", "1000m", 20},
		{"0", "500m", 0},
	}
	for _, tc := range cases {
		prev := resource.MustParse(tc.prev)
		next := resource.MustParse(tc.next)
		if got := adjustmentPercent(prev, next); got != tc.want {
			t.Errorf("adjustmentPercent(%s, %s) = %v, want %v", tc.prev, tc.next, got, tc.want)
		}
	}
}
