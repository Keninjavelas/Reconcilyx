package controller

import (
	"context"
	"fmt"
	"math"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	optimizationv1alpha1 "github.com/reconcilyx/operator/api/v1alpha1"
	"github.com/reconcilyx/operator/internal/engine"
	"github.com/reconcilyx/operator/internal/engine/rl"
	"github.com/reconcilyx/operator/internal/telemetry"
)

const (
	learningOutcomeCompleted = "completed"
	learningOutcomeRollback  = "rolled_back"
)

// needsRLLearning reports whether the proposal still has an unfinalized
// learning record. Legacy proposals without a pre-actuation record are
// finalized as skipped rather than retried forever.
func (r *OptimizationProposalReconciler) needsRLLearning(proposal *optimizationv1alpha1.OptimizationProposal) bool {
	if r.RLAgent == nil {
		return false
	}
	if proposal.Status.Phase != optimizationv1alpha1.PhaseCompleted &&
		proposal.Status.Phase != optimizationv1alpha1.PhaseRolledBack {
		return false
	}
	return proposal.Status.Learning == nil || !proposal.Status.Learning.Finalized
}

// captureLearningPreState snapshots the telemetry used by the action-selection
// state before the resize is applied. It must run before Actuator.ApplyMutation.
func (r *OptimizationProposalReconciler) captureLearningPreState(ctx context.Context, proposal *optimizationv1alpha1.OptimizationProposal) *optimizationv1alpha1.LearningRecord {
	record := &optimizationv1alpha1.LearningRecord{
		Pre: map[string]optimizationv1alpha1.LearningContainerState{},
	}
	if proposal.Status.ExecutionSnapshot == nil {
		record.SkipReason = "execution snapshot is missing"
		return record
	}
	if r.TelemetryProvider == nil {
		record.SkipReason = "telemetry provider is not configured"
		return record
	}

	metrics, err := r.TelemetryProvider.GetWorkloadMetrics(ctx, proposal.Spec.TargetWorkload)
	if err != nil && err != telemetry.ErrPartialTelemetry {
		record.SkipReason = fmt.Sprintf("pre-actuation telemetry unavailable: %v", err)
		return record
	}

	for containerName := range proposal.Status.ExecutionSnapshot.ContainerResources {
		state := optimizationv1alpha1.LearningContainerState{}
		if metrics != nil {
			if cm, ok := metrics.Containers[containerName]; ok {
				state = learningStateFromMetrics(cm)
			}
		}
		record.Pre[containerName] = state
	}
	return record
}

func learningStateFromMetrics(cm *telemetry.ContainerMetrics) optimizationv1alpha1.LearningContainerState {
	if cm == nil {
		return optimizationv1alpha1.LearningContainerState{}
	}
	jvmHeapMB := int64(cm.JVM.XmxBytes / (1024 * 1024))
	return optimizationv1alpha1.LearningContainerState{
		CPUUsageCores:         cm.CPU.UsageCores,
		MemoryWorkingSetBytes: int64(cm.Memory.WorkingSetBytes),
		CPUThrottledSeconds:   cm.CPU.ThrottledSecs,
		OOMCount:              cm.Memory.OOMCount,
		JVMBucket:             rl.DetermineJVMBucket(jvmHeapMB, cm.JVM.IsJVMWorkload),
		GPUBucket:             rl.DetermineGPUBucket(cm.GPU.GPUUtilizationPct, cm.GPU.GPUCount > 0),
	}
}

func (r *OptimizationProposalReconciler) carbonWeightFor(ctx context.Context, proposal *optimizationv1alpha1.OptimizationProposal) float64 {
	weight := r.CarbonWeight
	if proposal.Status.PolicyApplied != "" {
		policy := &optimizationv1alpha1.OptimizationPolicy{}
		key := types.NamespacedName{Name: proposal.Status.PolicyApplied, Namespace: proposal.Namespace}
		if err := r.Get(ctx, key, policy); err == nil && policy.Spec.SafetyConstraints.CarbonWeight != nil {
			weight = *policy.Spec.SafetyConstraints.CarbonWeight
		}
	}
	if weight < 0 {
		return 0
	}
	if weight > 1 {
		return 1
	}
	return weight
}

// applyRLLearning computes the reward from the persisted pre-actuation state
// and current telemetry, updates the agent once, and records the reward inputs
// on the proposal. It is idempotent through Learning.Finalized.
func (r *OptimizationProposalReconciler) applyRLLearning(ctx context.Context, proposal *optimizationv1alpha1.OptimizationProposal) error {
	if r.RLAgent == nil {
		return nil
	}
	if proposal.Status.Learning != nil && proposal.Status.Learning.Finalized {
		return nil
	}
	if proposal.Status.Learning == nil {
		proposal.Status.Learning = &optimizationv1alpha1.LearningRecord{
			Finalized:  true,
			SkipReason: "proposal has no pre-actuation learning state",
		}
		return nil
	}
	record := proposal.Status.Learning
	if record.SkipReason != "" {
		record.Finalized = true
		return nil
	}
	if proposal.Status.ExecutionSnapshot == nil || len(record.Pre) == 0 {
		record.Finalized = true
		record.SkipReason = "execution snapshot or pre-actuation state is missing"
		return nil
	}
	if r.TelemetryProvider == nil {
		record.Finalized = true
		record.SkipReason = "telemetry provider is not configured"
		return nil
	}

	postMetrics, err := r.TelemetryProvider.GetWorkloadMetrics(ctx, proposal.Spec.TargetWorkload)
	if err != nil && err != telemetry.ErrPartialTelemetry {
		return fmt.Errorf("failed to fetch post-actuation telemetry: %w", err)
	}

	snapshot := proposal.Status.ExecutionSnapshot
	replicas := snapshot.ReplicaCount
	if replicas <= 0 {
		replicas = 1
	}
	rolledBack := proposal.Status.Phase == optimizationv1alpha1.PhaseRolledBack
	carbonWeight := r.carbonWeightFor(ctx, proposal)
	carbonIntensity := 0.0
	if r.CarbonProvider != nil {
		if intensity, cErr := r.CarbonProvider.IntensityForRegion(ctx, r.WorkloadRegion); cErr == nil {
			carbonIntensity = intensity.Value
		}
	}

	record.Results = make(map[string]optimizationv1alpha1.LearningContainerResult, len(snapshot.ContainerResources))
	var overallReward float64

	for containerName, snapState := range snapshot.ContainerResources {
		pre, ok := record.Pre[containerName]
		if !ok {
			continue
		}
		prevCPUReq := snapState.Resources.Requests.Cpu().DeepCopy()
		prevMemReq := snapState.Resources.Requests.Memory().DeepCopy()
		attemptedCPUReq, attemptedMemReq := proposedRequestsFor(proposal, prevCPUReq, prevMemReq)

		post := optimizationv1alpha1.LearningContainerState{}
		postCM := (*telemetry.ContainerMetrics)(nil)
		if postMetrics != nil {
			postCM = postMetrics.Containers[containerName]
			post = learningStateFromMetrics(postCM)
		}

		costSaved := 0.0
		carbonSaved := 0.0
		nextCPUReq := attemptedCPUReq.DeepCopy()
		nextMemReq := attemptedMemReq.DeepCopy()
		if rolledBack {
			nextCPUReq = prevCPUReq.DeepCopy()
			nextMemReq = prevMemReq.DeepCopy()
		} else {
			cpuDeltaMilli := prevCPUReq.MilliValue() - attemptedCPUReq.MilliValue()
			memDeltaBytes := prevMemReq.Value() - attemptedMemReq.Value()
			costSaved = engine.ComputeCostSavedUSDPerMonth(cpuDeltaMilli, memDeltaBytes, replicas)
			carbonSaved = engine.ComputeCarbonSavedGramsPerMonth(cpuDeltaMilli, memDeltaBytes, replicas, carbonIntensity)
		}

		reward := 0.0
		if rolledBack {
			reward = -1.0
		} else {
			reward = rl.WeightedReward(costSaved, carbonSaved, carbonWeight)
			if post.OOMCount > 0 || (post.CPUUsageCores > 0 && post.CPUThrottledSeconds > post.CPUUsageCores*0.2) {
				reward *= 0.2
			}
		}
		overallReward += reward

		prevState := &rl.WorkloadState{
			CPUUsageCores:         pre.CPUUsageCores,
			CPUThrottledSecs:      pre.CPUThrottledSeconds,
			MemoryWorkingSetBytes: float64(pre.MemoryWorkingSetBytes),
			OOMCount:              pre.OOMCount,
			CurrentCPURequest:     prevCPUReq,
			CurrentMemoryRequest:  prevMemReq,
			Region:                r.WorkloadRegion,
			CarbonWeight:          &carbonWeight,
			JVMBucket:             pre.JVMBucket,
			GPUBucket:             pre.GPUBucket,
		}
		nextState := &rl.WorkloadState{
			CPUUsageCores:         post.CPUUsageCores,
			CPUThrottledSecs:      post.CPUThrottledSeconds,
			MemoryWorkingSetBytes: float64(post.MemoryWorkingSetBytes),
			OOMCount:              post.OOMCount,
			CurrentCPURequest:     nextCPUReq,
			CurrentMemoryRequest:  nextMemReq,
			Region:                r.WorkloadRegion,
			CarbonWeight:          &carbonWeight,
			JVMBucket:             post.JVMBucket,
			GPUBucket:             post.GPUBucket,
		}
		action := &rl.Action{
			Adjustment: map[string]rl.ResourceAdjustment{
				string(corev1.ResourceCPU): {
					ResourceName: corev1.ResourceCPU,
					Percentage:   adjustmentPercent(prevCPUReq, attemptedCPUReq),
				},
				string(corev1.ResourceMemory): {
					ResourceName: corev1.ResourceMemory,
					Percentage:   adjustmentPercent(prevMemReq, attemptedMemReq),
				},
			},
		}

		if uErr := r.RLAgent.Update(ctx, reward, action, prevState, nextState); uErr != nil {
			return fmt.Errorf("container %s: failed RL agent update: %w", containerName, uErr)
		}

		record.Results[containerName] = optimizationv1alpha1.LearningContainerResult{
			CPUAdjustmentPercent:    int(action.Adjustment[string(corev1.ResourceCPU)].Percentage),
			MemoryAdjustmentPercent: int(action.Adjustment[string(corev1.ResourceMemory)].Percentage),
			CostSavedUSD:            quantityFromFloat(costSaved, true),
			CarbonSavedGramsCO2:     quantityFromFloat(carbonSaved, false),
			Reward:                  reward,
			Post:                    post,
		}
	}

	record.Reward = overallReward
	record.Outcome = learningOutcomeCompleted
	if rolledBack {
		record.Outcome = learningOutcomeRollback
	}
	record.Finalized = true
	record.CompletedAt = &metav1.Time{Time: time.Now()}

	ref := proposal.Spec.TargetWorkload
	telemetry.RLUpdatesTotal.WithLabelValues(ref.Namespace, ref.Name, string(proposal.Status.Phase)).Inc()
	telemetry.RLRewardGauge.WithLabelValues(ref.Name, ref.Namespace).Set(overallReward)
	return nil
}

// proposedRequestsFor returns the requests the actuator actually attempts for
// the recommendation type, falling back to the pre-actuation values when a
// resource is not part of the proposal.
func proposedRequestsFor(proposal *optimizationv1alpha1.OptimizationProposal, prevCPU, prevMem resource.Quantity) (resource.Quantity, resource.Quantity) {
	cpu := prevCPU.DeepCopy()
	mem := prevMem.DeepCopy()
	proposed := proposal.Spec.ProposedResources.Requests
	switch proposal.Spec.Recommendation {
	case optimizationv1alpha1.RecommendationRightSize:
		if q, ok := proposed[corev1.ResourceCPU]; ok {
			cpu = q.DeepCopy()
		}
		if q, ok := proposed[corev1.ResourceMemory]; ok {
			mem = q.DeepCopy()
		}
	case optimizationv1alpha1.RecommendationReduceCPURequest:
		if q, ok := proposed[corev1.ResourceCPU]; ok {
			cpu = q.DeepCopy()
		}
	case optimizationv1alpha1.RecommendationReduceMemoryRequest:
		if q, ok := proposed[corev1.ResourceMemory]; ok {
			mem = q.DeepCopy()
		}
	default:
		return cpu, mem
	}
	return cpu, mem
}

// adjustmentPercent snaps an applied resource change onto the agent's action
// grid so the update trains the action that was actually executed.
func adjustmentPercent(prev, next resource.Quantity) float64 {
	prevMilli := prev.MilliValue()
	nextMilli := next.MilliValue()
	if prevMilli == 0 || prevMilli == nextMilli {
		return 0
	}
	pct := (float64(nextMilli) - float64(prevMilli)) / float64(prevMilli) * 100
	snapped := math.Round(pct/10) * 10
	if snapped < -20 {
		return -20
	}
	if snapped > 20 {
		return 20
	}
	return snapped
}

func quantityFromFloat(value float64, milli bool) *resource.Quantity {
	if milli {
		q := resource.NewMilliQuantity(int64(math.Round(value*1000)), resource.DecimalSI)
		return q
	}
	q := resource.NewQuantity(int64(math.Round(value)), resource.DecimalSI)
	return q
}
