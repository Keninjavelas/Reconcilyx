package policy

import (
	"context"
	"fmt"
	"strings"

	"github.com/reconcilyx/operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EvaluationResult is the complete outcome of a policy evaluation, including
// per-constraint evidence and the hypothetical state for dry-run policies.
type EvaluationResult struct {
	Decision v1alpha1.ApprovalState
	WouldBe  v1alpha1.ApprovalState
	Reason   string
	Evidence []v1alpha1.DecisionEvidence
}

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

func (e *defaultEvaluator) Evaluate(ctx context.Context, proposal *v1alpha1.OptimizationProposal, policy *v1alpha1.OptimizationPolicy, workloadContainers []string, lastOptimization *metav1.Time) EvaluationResult {
	var rejections []string
	evidence := make([]v1alpha1.DecisionEvidence, 0, len(e.constraints)+1)

	for _, c := range e.constraints {
		safe, rationale := c.Evaluate(ctx, proposal, policy, workloadContainers, lastOptimization)
		evidence = append(evidence, v1alpha1.DecisionEvidence{
			Constraint: c.Name(),
			Allowed:    safe,
			Rationale:  rationale,
		})
		if !safe {
			rejections = append(rejections, fmt.Sprintf("[%s]: %s", c.Name(), rationale))
		}
	}

	if policy.Spec.Mode == v1alpha1.ModeDryRun {
		hypothetical := policy.Spec.DryRunMode
		if hypothetical == "" || hypothetical == v1alpha1.ModeDryRun {
			hypothetical = v1alpha1.ModePropose
		}

		wouldBe, wouldBeReason := decisionForMode(hypothetical, policy.Name)
		if len(rejections) > 0 {
			wouldBe = v1alpha1.ApprovalRejected
			wouldBeReason = fmt.Sprintf("Rejected by Policy '%s'. Violations: %s", policy.Name, strings.Join(rejections, " | "))
		}
		evidence = append(evidence, v1alpha1.DecisionEvidence{
			Constraint: "PolicyMode",
			Allowed:    true,
			Rationale:  fmt.Sprintf("dry run: hypothetical %s mode would produce %s", hypothetical, wouldBe),
		})
		return EvaluationResult{
			Decision: v1alpha1.ApprovalDryRun,
			WouldBe:  wouldBe,
			Reason:   fmt.Sprintf("Dry run (hypothetical %s): %s No action taken.", hypothetical, wouldBeReason),
			Evidence: evidence,
		}
	}

	if len(rejections) > 0 {
		return EvaluationResult{
			Decision: v1alpha1.ApprovalRejected,
			Reason:   fmt.Sprintf("Rejected by Policy '%s'. Violations: %s", policy.Name, strings.Join(rejections, " | ")),
			Evidence: evidence,
		}
	}

	decision, reason := decisionForMode(policy.Spec.Mode, policy.Name)
	evidence = append(evidence, v1alpha1.DecisionEvidence{
		Constraint: "PolicyMode",
		Allowed:    true,
		Rationale:  fmt.Sprintf("mode %s produced %s", policy.Spec.Mode, decision),
	})
	return EvaluationResult{
		Decision: decision,
		Reason:   reason,
		Evidence: evidence,
	}
}

func decisionForMode(mode v1alpha1.PolicyMode, policyName string) (v1alpha1.ApprovalState, string) {
	switch mode {
	case v1alpha1.ModeObserve:
		return v1alpha1.ApprovalPending, fmt.Sprintf("Requires Review: Policy '%s' is in Observe mode. Constraints passed.", policyName)
	case v1alpha1.ModePropose:
		return v1alpha1.ApprovalPending, fmt.Sprintf("Requires Review: Policy '%s' is in Propose mode. Constraints passed.", policyName)
	case v1alpha1.ModeAutoApply:
		return v1alpha1.ApprovalApproved, fmt.Sprintf("Approved: Policy '%s' is in AutoApply mode and all constraints passed.", policyName)
	default:
		return v1alpha1.ApprovalPending, "Requires Review: Unknown Policy mode."
	}
}
