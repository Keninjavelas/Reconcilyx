package policy

import (
	"context"
	"testing"
	"time"

	"github.com/reconcilyx/operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMatcher_DeterminismAndPrecedence(t *testing.T) {
	matcher := NewMatcher()

	policies := []v1alpha1.OptimizationPolicy{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "c-global"},
			Spec: v1alpha1.OptimizationPolicySpec{
				Mode: v1alpha1.ModeAutoApply,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "a-namespace"},
			Spec: v1alpha1.OptimizationPolicySpec{
				Mode: v1alpha1.ModePropose,
				TargetSelectors: v1alpha1.TargetSelectors{
					NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "b-namespace-tied"},
			Spec: v1alpha1.OptimizationPolicySpec{
				Mode: v1alpha1.ModePropose, // Same specificity, same mode as a-namespace
				TargetSelectors: v1alpha1.TargetSelectors{
					NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "z-workload"},
			Spec: v1alpha1.OptimizationPolicySpec{
				Mode: v1alpha1.ModeObserve,
				TargetSelectors: v1alpha1.TargetSelectors{
					NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
					WorkloadSelector:  &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
				},
			},
		},
	}

	t.Run("Matches Most Specific Workload", func(t *testing.T) {
		matched, err := matcher.Match(context.Background(), corev1.ObjectReference{}, map[string]string{"env": "prod"}, map[string]string{"app": "web"}, policies)
		assert.NoError(t, err)
		assert.NotNil(t, matched)
		assert.Equal(t, "z-workload", matched.Name)
	})

	t.Run("Deterministic Tie-Breaker (Alphabetical)", func(t *testing.T) {
		// Both a-namespace and b-namespace-tied match. Both have mode Propose.
		matched, err := matcher.Match(context.Background(), corev1.ObjectReference{}, map[string]string{"env": "prod"}, map[string]string{"app": "database"}, policies)
		assert.NoError(t, err)
		assert.NotNil(t, matched)
		assert.Equal(t, "a-namespace", matched.Name) // 'a' comes before 'b'
	})

	t.Run("Falls Back To Global", func(t *testing.T) {
		matched, err := matcher.Match(context.Background(), corev1.ObjectReference{}, map[string]string{"env": "dev"}, map[string]string{"app": "database"}, policies)
		assert.NoError(t, err)
		assert.NotNil(t, matched)
		assert.Equal(t, "c-global", matched.Name)
	})
}

func TestEvaluator_Constraints(t *testing.T) {
	evaluator := NewEvaluator()

	riskTol := int32(40)
	confReq := int32(85)
	cooldown := metav1.Duration{Duration: 10 * time.Minute}

	policy := &v1alpha1.OptimizationPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "test-policy"},
		Spec: v1alpha1.OptimizationPolicySpec{
			Mode: v1alpha1.ModeAutoApply,
			ResourceConstraints: v1alpha1.ResourceConstraints{
				MinCPU: resource.NewMilliQuantity(100, resource.DecimalSI), // 100m
				MaxCPU: resource.NewMilliQuantity(1000, resource.DecimalSI), // 1000m
			},
			SafetyConstraints: v1alpha1.SafetyConstraints{
				RiskTolerance:         &riskTol,
				ConfidenceRequirement: &confReq,
				CooldownPeriod:        &cooldown,
			},
			RecommendationGovernance: v1alpha1.RecommendationGovernance{
				AllowedRecommendations: []v1alpha1.RecommendationType{v1alpha1.RecommendationRightSize},
			},
			WorkloadProtection: v1alpha1.WorkloadProtection{
				ExcludedContainers: []string{"istio-proxy"},
			},
		},
	}

	validProposal := func() *v1alpha1.OptimizationProposal {
		return &v1alpha1.OptimizationProposal{
			Spec: v1alpha1.OptimizationProposalSpec{
				Recommendation: v1alpha1.RecommendationRightSize,
				ProposedResources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("500m"),
					},
				},
				DecisionMetadata: v1alpha1.DecisionMetadata{
					RiskScore:       30,
					ConfidenceScore: 90,
				},
			},
		}
	}

	t.Run("Approves Safe Proposal", func(t *testing.T) {
		p := validProposal()
		containers := []string{"app"}
		state, rationale := evaluator.Evaluate(context.Background(), p, policy, containers, nil)
		assert.Equal(t, v1alpha1.ApprovalApproved, state)
		assert.Contains(t, rationale, "Approved")
	})

	t.Run("Rejects Excluded Sidecar", func(t *testing.T) {
		p := validProposal()
		containers := []string{"app", "istio-proxy"}
		state, rationale := evaluator.Evaluate(context.Background(), p, policy, containers, nil)
		assert.Equal(t, v1alpha1.ApprovalRejected, state)
		assert.Contains(t, rationale, "protected sidecar/container: 'istio-proxy'")
	})

	t.Run("Rejects Recommendation Blocked (ScaleToZero)", func(t *testing.T) {
		p := validProposal()
		p.Spec.Recommendation = v1alpha1.RecommendationScaleZero
		containers := []string{"app"}
		state, rationale := evaluator.Evaluate(context.Background(), p, policy, containers, nil)
		assert.Equal(t, v1alpha1.ApprovalRejected, state)
		assert.Contains(t, rationale, "Recommendation 'ScaleToZero' is not in the policy allowed list")
	})

	t.Run("Rejects High Risk", func(t *testing.T) {
		p := validProposal()
		p.Spec.DecisionMetadata.RiskScore = 60 // policy allows 40
		state, rationale := evaluator.Evaluate(context.Background(), p, policy, []string{"app"}, nil)
		assert.Equal(t, v1alpha1.ApprovalRejected, state)
		assert.Contains(t, rationale, "Risk Score (60) exceeds configured safety threshold (40)")
	})

	t.Run("Rejects Low Confidence", func(t *testing.T) {
		p := validProposal()
		p.Spec.DecisionMetadata.ConfidenceScore = 70 // policy requires 85
		state, rationale := evaluator.Evaluate(context.Background(), p, policy, []string{"app"}, nil)
		assert.Equal(t, v1alpha1.ApprovalRejected, state)
		assert.Contains(t, rationale, "Confidence Score (70) is below configured requirement (85)")
	})

	t.Run("Rejects Cooldown Violation", func(t *testing.T) {
		p := validProposal()
		// last optimization was 5 mins ago, policy requires 10 mins
		lastOpt := &metav1.Time{Time: time.Now().Add(-5 * time.Minute)}
		state, rationale := evaluator.Evaluate(context.Background(), p, policy, []string{"app"}, lastOpt)
		assert.Equal(t, v1alpha1.ApprovalRejected, state)
		assert.Contains(t, rationale, "Cooldown active")
	})

	t.Run("Approves Post-Cooldown", func(t *testing.T) {
		p := validProposal()
		// last optimization was 15 mins ago, policy requires 10 mins
		lastOpt := &metav1.Time{Time: time.Now().Add(-15 * time.Minute)}
		state, rationale := evaluator.Evaluate(context.Background(), p, policy, []string{"app"}, lastOpt)
		assert.Equal(t, v1alpha1.ApprovalApproved, state)
		assert.Contains(t, rationale, "Approved")
	})
}
