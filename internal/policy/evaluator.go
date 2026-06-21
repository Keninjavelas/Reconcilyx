package policy

import (
	"context"
	"fmt"
	"strings"

	"github.com/reconcilyx/operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type defaultEvaluator struct {
	constraints []Constraint
}

func NewEvaluator() Evaluator {
	return &defaultEvaluator{
		constraints: []Constraint{
			&ResourceConstraint{},
			&SafetyConstraint{},
			&RecommendationConstraint{},
			&CooldownConstraint{},
			&WorkloadProtectionConstraint{},
		},
	}
}

func (e *defaultEvaluator) Evaluate(ctx context.Context, proposal *v1alpha1.OptimizationProposal, policy *v1alpha1.OptimizationPolicy, workloadContainers []string, lastOptimization *metav1.Time) (v1alpha1.ApprovalState, string) {
	var rejections []string

	for _, c := range e.constraints {
		safe, rationale := c.Evaluate(ctx, proposal, policy, workloadContainers, lastOptimization)
		if !safe {
			rejections = append(rejections, fmt.Sprintf("[%s]: %s", c.Name(), rationale))
		}
	}

	if len(rejections) > 0 {
		return v1alpha1.ApprovalRejected, fmt.Sprintf("Rejected by Policy '%s'. Violations: %s", policy.Name, strings.Join(rejections, " | "))
	}

	// Constraints passed, now evaluate Mode
	switch policy.Spec.Mode {
	case v1alpha1.ModeObserve:
		return v1alpha1.ApprovalPending, fmt.Sprintf("Requires Review: Policy '%s' is in Observe mode. Constraints passed.", policy.Name)
	case v1alpha1.ModePropose:
		return v1alpha1.ApprovalPending, fmt.Sprintf("Requires Review: Policy '%s' is in Propose mode. Constraints passed.", policy.Name)
	case v1alpha1.ModeAutoApply:
		return v1alpha1.ApprovalApproved, fmt.Sprintf("Approved: Policy '%s' is in AutoApply mode and all constraints passed.", policy.Name)
	default:
		return v1alpha1.ApprovalPending, "Requires Review: Unknown Policy mode."
	}
}
