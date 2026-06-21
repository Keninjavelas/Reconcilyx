package controller

import (
	"context"
	"fmt"

	"github.com/reconcilyx/operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// RollbackValidator ensures a proposal can be safely rolled back.
type RollbackValidator interface {
	CanRollback(ctx context.Context, proposal *v1alpha1.OptimizationProposal, currentWorkload corev1.ObjectReference) error
}

type defaultRollbackValidator struct{}

func NewRollbackValidator() RollbackValidator {
	return &defaultRollbackValidator{}
}

func (r *defaultRollbackValidator) CanRollback(ctx context.Context, proposal *v1alpha1.OptimizationProposal, currentWorkload corev1.ObjectReference) error {
	if proposal.Status.ExecutionSnapshot == nil {
		return fmt.Errorf("no execution snapshot available for rollback")
	}
	
	// Phase 2 implementation will include compatibility checks
	return nil
}
