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

type defaultExecutionGate struct{
	actuationControl ActuationControl
	healthGate       WorkloadHealthGate
}

func NewExecutionGate(ac ActuationControl, hg WorkloadHealthGate) ExecutionGate {
	return &defaultExecutionGate{
		actuationControl: ac,
		healthGate:       hg,
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

	// 4. Restrict Execution Scope (Phase 3A constraint)
	if proposal.Spec.Recommendation != v1alpha1.RecommendationReduceCPURequest &&
		proposal.Spec.Recommendation != v1alpha1.RecommendationReduceMemoryRequest {
		return ExecutionReject, fmt.Errorf("recommendation type %s is forbidden in Phase 3A actuation", proposal.Spec.Recommendation)
	}

	// 5. Verify Target Liveness
	if err := g.healthGate.CheckLiveness(ctx, proposal.Spec.TargetWorkload); err != nil {
		return ExecutionReject, fmt.Errorf("workload liveness check failed: %w", err)
	}

	return ExecutionExecute, nil
}
