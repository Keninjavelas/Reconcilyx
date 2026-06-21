package controller

import (
	"context"

	"github.com/reconcilyx/operator/api/v1alpha1"
)

// Auditor records critical execution decisions for tracing.
type Auditor interface {
	RecordExecutionAttempt(ctx context.Context, proposal *v1alpha1.OptimizationProposal, decision string) error
	RecordEvaluation(ctx context.Context, proposal *v1alpha1.OptimizationProposal, policy *v1alpha1.OptimizationPolicy, decision string) error
}

type defaultAuditor struct{}

func NewAuditor() Auditor {
	return &defaultAuditor{}
}

func (a *defaultAuditor) RecordExecutionAttempt(ctx context.Context, proposal *v1alpha1.OptimizationProposal, decision string) error {
	// TODO: Phase 3 - Implement ConfigMap or Event audit writing
	return nil
}

func (a *defaultAuditor) RecordEvaluation(ctx context.Context, proposal *v1alpha1.OptimizationProposal, policy *v1alpha1.OptimizationPolicy, decision string) error {
	// TODO: Phase 3 - Implement ConfigMap or Event audit writing
	return nil
}
