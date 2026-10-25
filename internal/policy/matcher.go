package policy

import (
	"context"
	"sort"

	"github.com/reconcilyx/operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type defaultMatcher struct{}

func NewMatcher() Matcher {
	return &defaultMatcher{}
}

func (m *defaultMatcher) Match(ctx context.Context, workload corev1.ObjectReference, namespaceLabels map[string]string, workloadLabels map[string]string, policies []v1alpha1.OptimizationPolicy) (*v1alpha1.OptimizationPolicy, error) {
	var matched []v1alpha1.OptimizationPolicy

	for _, policy := range policies {
		// 1. Check Namespace Selector
		if policy.Spec.TargetSelectors.NamespaceSelector != nil {
			nsSelector, err := metav1.LabelSelectorAsSelector(policy.Spec.TargetSelectors.NamespaceSelector)
			if err != nil {
				continue // Skip invalid selectors
			}
			if !nsSelector.Matches(labels.Set(namespaceLabels)) {
				continue
			}
		}

		// 2. Check Workload Selector
		if policy.Spec.TargetSelectors.WorkloadSelector != nil {
			wlSelector, err := metav1.LabelSelectorAsSelector(policy.Spec.TargetSelectors.WorkloadSelector)
			if err != nil {
				continue
			}
			if !wlSelector.Matches(labels.Set(workloadLabels)) {
				continue
			}
		}

		// Matches both or neither (meaning global match)
		matched = append(matched, policy)
	}

	return selectBest(matched, workload.Namespace), nil
}

// selectBest applies the deterministic precedence order: selector
// specificity, namespace-local affinity, conservative mode, then the fully
// qualified policy name.
func selectBest(matched []v1alpha1.OptimizationPolicy, targetNamespace string) *v1alpha1.OptimizationPolicy {
	if len(matched) == 0 {
		return nil
	}

	sort.SliceStable(matched, func(i, j int) bool {
		scoreI := precedenceScore(matched[i], targetNamespace)
		scoreJ := precedenceScore(matched[j], targetNamespace)

		if scoreI == scoreJ {
			modeI := modePrecedence(matched[i].Spec.Mode)
			modeJ := modePrecedence(matched[j].Spec.Mode)
			if modeI == modeJ {
				// Deterministic tie-breaker across namespaces.
				return policyKey(matched[i]) < policyKey(matched[j])
			}
			// Resolve conflict using conservative mode: Observe > Propose >
			// DryRun > AutoApply.
			return modeI > modeJ
		}
		return scoreI > scoreJ
	})

	return &matched[0]
}

func specificityScore(p v1alpha1.OptimizationPolicy) int {
	score := 0
	if p.Spec.TargetSelectors.WorkloadSelector != nil {
		score += 10
	}
	if p.Spec.TargetSelectors.NamespaceSelector != nil {
		score += 5
	}
	return score
}

// namespaceAffinityScore makes a policy created in the target namespace
// outrank a foreign or cluster-wide policy with the same selector
// specificity. This is the per-namespace override: a namespace-local
// default (score 6) beats a global default (score 0), while an explicit
// workload selector in a global policy (score 10) still wins.
func namespaceAffinityScore(p v1alpha1.OptimizationPolicy, targetNamespace string) int {
	if targetNamespace != "" && p.Namespace == targetNamespace {
		return 6
	}
	return 0
}

func precedenceScore(p v1alpha1.OptimizationPolicy, targetNamespace string) int {
	return specificityScore(p) + namespaceAffinityScore(p, targetNamespace)
}

func policyKey(p v1alpha1.OptimizationPolicy) string {
	return p.Namespace + "/" + p.Name
}

func modePrecedence(mode v1alpha1.PolicyMode) int {
	switch mode {
	case v1alpha1.ModeObserve:
		return 4
	case v1alpha1.ModePropose:
		return 3
	case v1alpha1.ModeDryRun:
		return 2
	case v1alpha1.ModeAutoApply:
		return 1
	default:
		return 0
	}
}
