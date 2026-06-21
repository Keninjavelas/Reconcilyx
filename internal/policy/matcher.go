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

	if len(matched) == 0 {
		return nil, nil // No policy applies
	}

	// 3. Sort by Precedence
	// Higher score = more specific.
	sort.SliceStable(matched, func(i, j int) bool {
		scoreI := specificityScore(matched[i])
		scoreJ := specificityScore(matched[j])
		
		if scoreI == scoreJ {
			modeI := modePrecedence(matched[i].Spec.Mode)
			modeJ := modePrecedence(matched[j].Spec.Mode)
			if modeI == modeJ {
				// Deterministic alphabetical tie-breaker
				return matched[i].Name < matched[j].Name
			}
			// Resolve conflict using conservative mode: Observe > Propose > AutoApply
			return modeI > modeJ
		}
		return scoreI > scoreJ
	})

	return &matched[0], nil
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

func modePrecedence(mode v1alpha1.PolicyMode) int {
	switch mode {
	case v1alpha1.ModeObserve:
		return 3
	case v1alpha1.ModePropose:
		return 2
	case v1alpha1.ModeAutoApply:
		return 1
	default:
		return 0
	}
}
