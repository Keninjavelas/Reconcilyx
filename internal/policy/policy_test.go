package policy

import (
	"context"
	"testing"
	"time"

	"github.com/reconcilyx/operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	t.Run("Fully Qualified Name Breaks Namespace Ties", func(t *testing.T) {
		shared := v1alpha1.OptimizationPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "policy", Namespace: "z-team"},
			Spec: v1alpha1.OptimizationPolicySpec{
				Mode: v1alpha1.ModePropose,
				TargetSelectors: v1alpha1.TargetSelectors{
					NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
				},
			},
		}
		local := *shared.DeepCopy()
		local.Name = "a-policy"
		local.Namespace = "a-team"

		matched, err := matcher.Match(context.Background(), corev1.ObjectReference{Namespace: "target"}, map[string]string{"env": "prod"}, nil, []v1alpha1.OptimizationPolicy{shared, local})
		require.NoError(t, err)
		require.NotNil(t, matched)
		// The bare names would order a-policy first; the fully qualified
		// names order a-team/a-policy first.
		assert.Equal(t, "a-team/a-policy", matched.Namespace+"/"+matched.Name)
	})

	t.Run("Namespace Local Policy Overrides Foreign Default", func(t *testing.T) {
		foreign := v1alpha1.OptimizationPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "platform"},
			Spec: v1alpha1.OptimizationPolicySpec{
				Mode: v1alpha1.ModeAutoApply,
				TargetSelectors: v1alpha1.TargetSelectors{
					NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
				},
			},
		}
		local := v1alpha1.OptimizationPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "team-a"},
			Spec: v1alpha1.OptimizationPolicySpec{
				Mode: v1alpha1.ModeObserve,
				TargetSelectors: v1alpha1.TargetSelectors{
					NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
				},
			},
		}

		matched, err := matcher.Match(context.Background(), corev1.ObjectReference{Namespace: "team-a"}, map[string]string{"env": "prod"}, nil, []v1alpha1.OptimizationPolicy{foreign, local})
		require.NoError(t, err)
		require.NotNil(t, matched)
		assert.Equal(t, "team-a", matched.Namespace)
	})

	t.Run("Explicit Workload Selector Beats Local Default", func(t *testing.T) {
		global := v1alpha1.OptimizationPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "platform"},
			Spec: v1alpha1.OptimizationPolicySpec{
				Mode: v1alpha1.ModePropose,
				TargetSelectors: v1alpha1.TargetSelectors{
					WorkloadSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
				},
			},
		}
		local := v1alpha1.OptimizationPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "team-a"},
			Spec:       v1alpha1.OptimizationPolicySpec{Mode: v1alpha1.ModeAutoApply},
		}

		matched, err := matcher.Match(context.Background(), corev1.ObjectReference{Namespace: "team-a"}, nil, map[string]string{"app": "web"}, []v1alpha1.OptimizationPolicy{global, local})
		require.NoError(t, err)
		require.NotNil(t, matched)
		assert.Equal(t, "web", matched.Name)
	})

	t.Run("Conservative Mode Breaks Specificity Ties", func(t *testing.T) {
		observe := v1alpha1.OptimizationPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "a-observe"},
			Spec:       v1alpha1.OptimizationPolicySpec{Mode: v1alpha1.ModeObserve},
		}
		auto := v1alpha1.OptimizationPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "z-auto"},
			Spec:       v1alpha1.OptimizationPolicySpec{Mode: v1alpha1.ModeAutoApply},
		}

		matched, err := matcher.Match(context.Background(), corev1.ObjectReference{}, nil, nil, []v1alpha1.OptimizationPolicy{auto, observe})
		require.NoError(t, err)
		require.NotNil(t, matched)
		assert.Equal(t, "a-observe", matched.Name)
	})
}

func TestIndex_MatchesAndRefreshes(t *testing.T) {
	global := v1alpha1.OptimizationPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "global", Namespace: "platform"},
		Spec: v1alpha1.OptimizationPolicySpec{
			Mode: v1alpha1.ModeAutoApply,
		},
	}
	prodOnly := v1alpha1.OptimizationPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "prod", Namespace: "platform"},
		Spec: v1alpha1.OptimizationPolicySpec{
			Mode: v1alpha1.ModePropose,
			TargetSelectors: v1alpha1.TargetSelectors{
				NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
			},
		},
	}
	expressionOnly := v1alpha1.OptimizationPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "expression", Namespace: "platform"},
		Spec: v1alpha1.OptimizationPolicySpec{
			Mode: v1alpha1.ModeObserve,
			TargetSelectors: v1alpha1.TargetSelectors{
				NamespaceSelector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{
					Key:      "env",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"prod"},
				}}},
			},
		},
	}
	webOnly := v1alpha1.OptimizationPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "platform"},
		Spec: v1alpha1.OptimizationPolicySpec{
			Mode: v1alpha1.ModePropose,
			TargetSelectors: v1alpha1.TargetSelectors{
				WorkloadSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
			},
		},
	}
	invalid := v1alpha1.OptimizationPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "invalid", Namespace: "platform"},
		Spec: v1alpha1.OptimizationPolicySpec{
			Mode: v1alpha1.ModeObserve,
			TargetSelectors: v1alpha1.TargetSelectors{
				NamespaceSelector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{
					Key:      "env",
					Operator: "Bogus",
				}}},
			},
		},
	}

	policies := []v1alpha1.OptimizationPolicy{global, prodOnly, expressionOnly, webOnly, invalid}
	index := NewIndex()
	index.Replace(policies)

	cases := []struct {
		name            string
		namespace       string
		namespaceLabels map[string]string
		workloadLabels  map[string]string
		expectedPolicy  string
	}{
		{name: "global fallback", namespace: "dev", namespaceLabels: map[string]string{"env": "dev"}, expectedPolicy: "global"},
		{name: "expression-only selector included", namespace: "team-a", namespaceLabels: map[string]string{"env": "prod"}, expectedPolicy: "expression"},
		{name: "workload selector wins", namespace: "team-a", namespaceLabels: map[string]string{"env": "prod"}, workloadLabels: map[string]string{"app": "web"}, expectedPolicy: "web"},
		{name: "no match", namespace: "team-a", namespaceLabels: map[string]string{"env": "staging"}, workloadLabels: map[string]string{"app": "db"}, expectedPolicy: "global"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matched, err := index.Match(context.Background(), corev1.ObjectReference{Namespace: tc.namespace}, tc.namespaceLabels, tc.workloadLabels)
			require.NoError(t, err)
			require.NotNil(t, matched)
			assert.Equal(t, tc.expectedPolicy, matched.Name)
		})
	}

	t.Run("matches the linear matcher", func(t *testing.T) {
		matcher := NewMatcher()
		namespaceLabels := map[string]string{"env": "prod", "team": "payments"}
		workloadLabels := map[string]string{"app": "api"}
		indexed, err := index.Match(context.Background(), corev1.ObjectReference{Namespace: "team-a"}, namespaceLabels, workloadLabels)
		require.NoError(t, err)
		linear, err := matcher.Match(context.Background(), corev1.ObjectReference{Namespace: "team-a"}, namespaceLabels, workloadLabels, policies)
		require.NoError(t, err)
		require.NotNil(t, indexed)
		require.NotNil(t, linear)
		assert.Equal(t, linear.Namespace+"/"+linear.Name, indexed.Namespace+"/"+indexed.Name)
	})

	t.Run("refresh removes deleted policies", func(t *testing.T) {
		index.Replace([]v1alpha1.OptimizationPolicy{global})
		matched, err := index.Match(context.Background(), corev1.ObjectReference{Namespace: "team-a"}, map[string]string{"env": "prod"}, map[string]string{"app": "web"})
		require.NoError(t, err)
		require.NotNil(t, matched)
		assert.Equal(t, "global", matched.Name)

		index.Replace(nil)
		matched, err = index.Match(context.Background(), corev1.ObjectReference{Namespace: "team-a"}, map[string]string{"env": "prod"}, nil)
		require.NoError(t, err)
		assert.Nil(t, matched)
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
				MinCPU: resource.NewMilliQuantity(100, resource.DecimalSI),  // 100m
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
		result := evaluator.Evaluate(context.Background(), p, policy, containers, nil)
		assert.Equal(t, v1alpha1.ApprovalApproved, result.Decision)
		assert.Contains(t, result.Reason, "Approved")
	})

	t.Run("Records Evidence For Every Check", func(t *testing.T) {
		result := evaluator.Evaluate(context.Background(), validProposal(), policy, []string{"app"}, nil)
		require.Len(t, result.Evidence, 6)

		names := make([]string, 0, len(result.Evidence))
		for _, evidence := range result.Evidence {
			names = append(names, evidence.Constraint)
			assert.True(t, evidence.Allowed)
		}
		assert.Equal(t, []string{
			"ResourceBoundaries",
			"SafetyScores",
			"RecommendationGovernance",
			"CooldownEnforcement",
			"WorkloadProtection",
			"PolicyMode",
		}, names)
		assert.Contains(t, result.Evidence[5].Rationale, "AutoApply")
	})

	t.Run("Rejects Excluded Sidecar", func(t *testing.T) {
		p := validProposal()
		containers := []string{"app", "istio-proxy"}
		result := evaluator.Evaluate(context.Background(), p, policy, containers, nil)
		assert.Equal(t, v1alpha1.ApprovalRejected, result.Decision)
		assert.Contains(t, result.Reason, "protected sidecar/container: 'istio-proxy'")
		assert.False(t, evidenceFor(result.Evidence, "WorkloadProtection").Allowed)
	})

	t.Run("Rejects Recommendation Blocked (ScaleToZero)", func(t *testing.T) {
		p := validProposal()
		p.Spec.Recommendation = v1alpha1.RecommendationScaleZero
		containers := []string{"app"}
		result := evaluator.Evaluate(context.Background(), p, policy, containers, nil)
		assert.Equal(t, v1alpha1.ApprovalRejected, result.Decision)
		assert.Contains(t, result.Reason, "Recommendation 'ScaleToZero' is not in the policy allowed list")
	})

	t.Run("Rejects High Risk", func(t *testing.T) {
		p := validProposal()
		p.Spec.DecisionMetadata.RiskScore = 60 // policy allows 40
		result := evaluator.Evaluate(context.Background(), p, policy, []string{"app"}, nil)
		assert.Equal(t, v1alpha1.ApprovalRejected, result.Decision)
		assert.Contains(t, result.Reason, "Risk Score (60) exceeds configured safety threshold (40)")
	})

	t.Run("Rejects Low Confidence", func(t *testing.T) {
		p := validProposal()
		p.Spec.DecisionMetadata.ConfidenceScore = 70 // policy requires 85
		result := evaluator.Evaluate(context.Background(), p, policy, []string{"app"}, nil)
		assert.Equal(t, v1alpha1.ApprovalRejected, result.Decision)
		assert.Contains(t, result.Reason, "Confidence Score (70) is below configured requirement (85)")
	})

	t.Run("Rejects Cooldown Violation", func(t *testing.T) {
		p := validProposal()
		// last optimization was 5 mins ago, policy requires 10 mins
		lastOpt := &metav1.Time{Time: time.Now().Add(-5 * time.Minute)}
		result := evaluator.Evaluate(context.Background(), p, policy, []string{"app"}, lastOpt)
		assert.Equal(t, v1alpha1.ApprovalRejected, result.Decision)
		assert.Contains(t, result.Reason, "Cooldown active")
	})

	t.Run("Approves Post-Cooldown", func(t *testing.T) {
		p := validProposal()
		// last optimization was 15 mins ago, policy requires 10 mins
		lastOpt := &metav1.Time{Time: time.Now().Add(-15 * time.Minute)}
		result := evaluator.Evaluate(context.Background(), p, policy, []string{"app"}, lastOpt)
		assert.Equal(t, v1alpha1.ApprovalApproved, result.Decision)
		assert.Contains(t, result.Reason, "Approved")
	})
}

func TestResourceConstraint_Limits(t *testing.T) {
	evaluator := NewEvaluator()
	policy := &v1alpha1.OptimizationPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "bounded"},
		Spec: v1alpha1.OptimizationPolicySpec{
			Mode: v1alpha1.ModeAutoApply,
			ResourceConstraints: v1alpha1.ResourceConstraints{
				MinCPU:    resource.NewMilliQuantity(100, resource.DecimalSI),
				MaxCPU:    resource.NewMilliQuantity(1000, resource.DecimalSI),
				MinMemory: resource.NewQuantity(64*1024*1024, resource.BinarySI),
				MaxMemory: resource.NewQuantity(1024*1024*1024, resource.BinarySI),
			},
			RecommendationGovernance: v1alpha1.RecommendationGovernance{
				AllowedRecommendations: []v1alpha1.RecommendationType{v1alpha1.RecommendationRightSize},
			},
		},
	}

	proposalWith := func(resources corev1.ResourceRequirements) *v1alpha1.OptimizationProposal {
		return &v1alpha1.OptimizationProposal{
			Spec: v1alpha1.OptimizationProposalSpec{
				Recommendation:    v1alpha1.RecommendationRightSize,
				ProposedResources: resources,
				DecisionMetadata:  v1alpha1.DecisionMetadata{RiskScore: 10, ConfidenceScore: 95},
			},
		}
	}

	t.Run("Rejects CPU Limit Below Minimum", func(t *testing.T) {
		proposal := proposalWith(corev1.ResourceRequirements{Limits: corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("50m"),
		}})
		result := evaluator.Evaluate(context.Background(), proposal, policy, []string{"app"}, nil)
		assert.Equal(t, v1alpha1.ApprovalRejected, result.Decision)
		assert.Contains(t, result.Reason, "CPU limit (50m) < MinCPU (100m)")
	})

	t.Run("Rejects Memory Limit Above Maximum", func(t *testing.T) {
		proposal := proposalWith(corev1.ResourceRequirements{Limits: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		}})
		result := evaluator.Evaluate(context.Background(), proposal, policy, []string{"app"}, nil)
		assert.Equal(t, v1alpha1.ApprovalRejected, result.Decision)
		assert.Contains(t, result.Reason, "Memory limit (2Gi) > MaxMemory (1Gi)")
	})

	t.Run("Rejects Per Container Bound Violation", func(t *testing.T) {
		proposal := proposalWith(corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m")},
		})
		proposal.Spec.ProposedResourcesByContainer = map[string]corev1.ResourceRequirements{
			"app": {
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m")},
			},
		}
		result := evaluator.Evaluate(context.Background(), proposal, policy, []string{"app"}, nil)
		assert.Equal(t, v1alpha1.ApprovalRejected, result.Decision)
		assert.Contains(t, result.Reason, `CPU container "app" request (50m) < MinCPU (100m)`)
	})

	t.Run("Accepts In-Bounds Limits", func(t *testing.T) {
		proposal := proposalWith(corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m")},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("800m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
		})
		result := evaluator.Evaluate(context.Background(), proposal, policy, []string{"app"}, nil)
		assert.Equal(t, v1alpha1.ApprovalApproved, result.Decision)
	})
}

func TestEvaluator_DryRun(t *testing.T) {
	evaluator := NewEvaluator()
	proposal := &v1alpha1.OptimizationProposal{
		Spec: v1alpha1.OptimizationProposalSpec{
			Recommendation: v1alpha1.RecommendationRightSize,
			ProposedResources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m")},
			},
			DecisionMetadata: v1alpha1.DecisionMetadata{RiskScore: 10, ConfidenceScore: 95},
		},
	}

	policy := func(dryRunMode v1alpha1.PolicyMode) *v1alpha1.OptimizationPolicy {
		return &v1alpha1.OptimizationPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "dry-run-policy"},
			Spec: v1alpha1.OptimizationPolicySpec{
				Mode:       v1alpha1.ModeDryRun,
				DryRunMode: dryRunMode,
				RecommendationGovernance: v1alpha1.RecommendationGovernance{
					AllowedRecommendations: []v1alpha1.RecommendationType{v1alpha1.RecommendationRightSize},
				},
			},
		}
	}

	t.Run("Reports Hypothetical AutoApply Without Deciding", func(t *testing.T) {
		result := evaluator.Evaluate(context.Background(), proposal, policy(v1alpha1.ModeAutoApply), []string{"app"}, nil)
		assert.Equal(t, v1alpha1.ApprovalDryRun, result.Decision)
		assert.Equal(t, v1alpha1.ApprovalApproved, result.WouldBe)
		assert.Contains(t, result.Reason, "No action taken.")
		assert.Contains(t, evidenceFor(result.Evidence, "PolicyMode").Rationale, "hypothetical AutoApply")
	})

	t.Run("Reports Hypothetical Pending", func(t *testing.T) {
		result := evaluator.Evaluate(context.Background(), proposal, policy(v1alpha1.ModePropose), []string{"app"}, nil)
		assert.Equal(t, v1alpha1.ApprovalDryRun, result.Decision)
		assert.Equal(t, v1alpha1.ApprovalPending, result.WouldBe)
	})

	t.Run("Defaults To Propose Hypothetical", func(t *testing.T) {
		result := evaluator.Evaluate(context.Background(), proposal, policy(""), []string{"app"}, nil)
		assert.Equal(t, v1alpha1.ApprovalDryRun, result.Decision)
		assert.Equal(t, v1alpha1.ApprovalPending, result.WouldBe)
	})

	t.Run("Constraint Violations Become Hypothetical Rejections", func(t *testing.T) {
		blocked := proposal.DeepCopy()
		blocked.Spec.Recommendation = v1alpha1.RecommendationScaleZero
		result := evaluator.Evaluate(context.Background(), blocked, policy(v1alpha1.ModeAutoApply), []string{"app"}, nil)
		assert.Equal(t, v1alpha1.ApprovalDryRun, result.Decision)
		assert.Equal(t, v1alpha1.ApprovalRejected, result.WouldBe)
		assert.Contains(t, result.Reason, "No action taken.")
		assert.Contains(t, result.Reason, "ScaleToZero")
	})
}

func evidenceFor(evidence []v1alpha1.DecisionEvidence, constraint string) v1alpha1.DecisionEvidence {
	for _, entry := range evidence {
		if entry.Constraint == constraint {
			return entry
		}
	}
	return v1alpha1.DecisionEvidence{}
}
