package policy

import (
	"context"

	"github.com/reconcilyx/operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Matcher selects the most applicable policy for a given workload.
type Matcher interface {
	Match(ctx context.Context, workload corev1.ObjectReference, namespaceLabels map[string]string, workloadLabels map[string]string, policies []v1alpha1.OptimizationPolicy) (*v1alpha1.OptimizationPolicy, error)
}

// Constraint evaluates whether a proposal is safe under a given policy.
type Constraint interface {
	Name() string
	// Evaluate returns true if safe, false if the constraint is violated, along with a rationale.
	Evaluate(ctx context.Context, proposal *v1alpha1.OptimizationProposal, policy *v1alpha1.OptimizationPolicy, workloadContainers []string, lastOptimization *metav1.Time) (bool, string)
}

// Evaluator coordinates the policy evaluation pipeline.
type Evaluator interface {
	Evaluate(ctx context.Context, proposal *v1alpha1.OptimizationProposal, policy *v1alpha1.OptimizationPolicy, workloadContainers []string, lastOptimization *metav1.Time) EvaluationResult
}
