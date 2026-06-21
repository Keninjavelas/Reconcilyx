package engine

import (
	"github.com/reconcilyx/operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// Finding represents a single rule violation for a specific container.
type Finding struct {
	RuleName        string
	ContainerName   string
	Severity        int32
	ProposedRequest *corev1.ResourceList
	ProposedLimit   *corev1.ResourceList
	Recommendation  v1alpha1.RecommendationType
	Rationale       string
}
