package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/reconcilyx/operator/api/v1alpha1"
)

// ExecutionDecision defines the outcome of a JIT validation.
type ExecutionDecision string

const (
	ExecutionExecute    ExecutionDecision = "Execute"
	ExecutionReject     ExecutionDecision = "Reject"
	ExecutionExpire     ExecutionDecision = "Expire"
	ExecutionRetryLater ExecutionDecision = "RetryLater"
)

// ExecutionGate ensures a proposal is safe to execute just-in-time.
type ExecutionGate interface {
	ValidateJIT(ctx context.Context, proposal *v1alpha1.OptimizationProposal, policy *v1alpha1.OptimizationPolicy) (ExecutionDecision, error)
}

type defaultExecutionGate struct {
	actuationControl ActuationControl
	healthGate       WorkloadHealthGate
	autoscalerGate   AutoscalerGate
}

func NewExecutionGate(ac ActuationControl, hg WorkloadHealthGate, ag AutoscalerGate) ExecutionGate {
	return &defaultExecutionGate{
		actuationControl: ac,
		healthGate:       hg,
		autoscalerGate:   ag,
	}
}

func (g *defaultExecutionGate) ValidateJIT(ctx context.Context, proposal *v1alpha1.OptimizationProposal, policy *v1alpha1.OptimizationPolicy) (ExecutionDecision, error) {
	// 1. Verify Kill Switch
	paused, err := g.actuationControl.IsExecutionPaused(ctx)
	if err != nil {
		return ExecutionRetryLater, err
	}
	if paused {
		return ExecutionRetryLater, nil
	}

	// 2. Verify Phase
	if proposal.Status.Phase != v1alpha1.PhaseScheduled {
		return ExecutionReject, fmt.Errorf("proposal is not in Scheduled phase")
	}

	// 3. Verify Freshness / TTL
	if proposal.Status.ExpirationTime != nil && time.Now().After(proposal.Status.ExpirationTime.Time) {
		return ExecutionExpire, nil
	}

	// 4. Restrict execution scope to what the in-place resize actuator supports.
	if !isActuatableRecommendation(proposal.Spec.Recommendation) {
		return ExecutionReject, fmt.Errorf("recommendation type %s is not supported by in-place resize actuation", proposal.Spec.Recommendation)
	}

	// 5. Verify no other autoscaler owns resource sizing for this workload.
	if g.autoscalerGate != nil {
		if err := g.autoscalerGate.Check(ctx, proposal.Spec.TargetWorkload); err != nil {
			return ExecutionReject, err
		}
	}

	// 6. Verify Target Liveness
	if err := g.healthGate.CheckLiveness(ctx, proposal.Spec.TargetWorkload); err != nil {
		return ExecutionReject, fmt.Errorf("workload liveness check failed: %w", err)
	}

	return ExecutionExecute, nil
}
