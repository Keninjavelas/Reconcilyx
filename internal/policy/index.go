package policy

import (
	"context"
	"sync"

	"github.com/reconcilyx/operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// Index is an informer-backed policy matcher. Replace is called from the
// policy watch (and from the initial cache list); Match then evaluates only
// the policies that can match the target namespace's labels instead of
// scanning every policy on every proposal.
type Index struct {
	mu               sync.RWMutex
	compiled         []compiledPolicy
	global           []int
	dynamicNamespace []int
	byNamespaceLabel map[string][]int
}

type compiledPolicy struct {
	policy       v1alpha1.OptimizationPolicy
	namespaceSel labels.Selector
	workloadSel  labels.Selector
	valid        bool
}

func NewIndex() *Index {
	return &Index{
		byNamespaceLabel: map[string][]int{},
	}
}

// Replace atomically installs a new policy snapshot.
func (i *Index) Replace(policies []v1alpha1.OptimizationPolicy) {
	compiled := make([]compiledPolicy, 0, len(policies))
	global := make([]int, 0, len(policies))
	dynamicNamespace := make([]int, 0)
	byNamespaceLabel := map[string][]int{}

	for _, p := range policies {
		entry := compiledPolicy{
			policy:       p,
			namespaceSel: labels.Everything(),
			workloadSel:  labels.Everything(),
			valid:        true,
		}
		if p.Spec.TargetSelectors.NamespaceSelector != nil {
			selector, err := metav1.LabelSelectorAsSelector(p.Spec.TargetSelectors.NamespaceSelector)
			if err != nil {
				entry.valid = false
			} else {
				entry.namespaceSel = selector
			}
		}
		if entry.valid && p.Spec.TargetSelectors.WorkloadSelector != nil {
			selector, err := metav1.LabelSelectorAsSelector(p.Spec.TargetSelectors.WorkloadSelector)
			if err != nil {
				entry.valid = false
			} else {
				entry.workloadSel = selector
			}
		}
		index := len(compiled)
		compiled = append(compiled, entry)
		if !entry.valid {
			continue
		}

		ns := p.Spec.TargetSelectors.NamespaceSelector
		if ns == nil || len(ns.MatchLabels) == 0 && len(ns.MatchExpressions) == 0 {
			global = append(global, index)
			continue
		}
		if len(ns.MatchLabels) == 0 {
			dynamicNamespace = append(dynamicNamespace, index)
			continue
		}
		for key, value := range ns.MatchLabels {
			bucket := key + "=" + value
			byNamespaceLabel[bucket] = append(byNamespaceLabel[bucket], index)
		}
	}

	i.mu.Lock()
	i.compiled = compiled
	i.global = global
	i.dynamicNamespace = dynamicNamespace
	i.byNamespaceLabel = byNamespaceLabel
	i.mu.Unlock()
}

// Match returns the highest-precedence policy for the workload, or nil when
// no policy matches. The candidate set is the union of global policies,
// policies with expression-only namespace selectors, and policies indexed
// under one of the namespace's labels.
func (i *Index) Match(ctx context.Context, workload corev1.ObjectReference, namespaceLabels map[string]string, workloadLabels map[string]string) (*v1alpha1.OptimizationPolicy, error) {
	_ = ctx
	i.mu.RLock()
	defer i.mu.RUnlock()

	seen := make([]bool, len(i.compiled))
	candidates := make([]int, 0, len(i.compiled))
	add := func(index int) {
		if !seen[index] {
			seen[index] = true
			candidates = append(candidates, index)
		}
	}
	for _, index := range i.global {
		add(index)
	}
	for _, index := range i.dynamicNamespace {
		add(index)
	}
	for key, value := range namespaceLabels {
		for _, index := range i.byNamespaceLabel[key+"="+value] {
			add(index)
		}
	}

	matched := make([]v1alpha1.OptimizationPolicy, 0, len(candidates))
	for _, index := range candidates {
		entry := i.compiled[index]
		if entry.namespaceSel.Matches(labels.Set(namespaceLabels)) &&
			entry.workloadSel.Matches(labels.Set(workloadLabels)) {
			matched = append(matched, entry.policy)
		}
	}

	return selectBest(matched, workload.Namespace), nil
}
