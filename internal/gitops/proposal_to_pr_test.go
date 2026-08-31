package gitops

import (
	"context"
	"strings"
	"testing"

	"github.com/reconcilyx/operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPRManager_CreateProposalPR(t *testing.T) {
	mockProvider := NewMockProvider()
	prManager := NewPRManager(mockProvider)

	carbonSavedQty := resource.MustParse("12400")
	proposal := &v1alpha1.OptimizationProposal{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-proposal",
			Namespace: "default",
		},
		Spec: v1alpha1.OptimizationProposalSpec{
			TargetWorkload: corev1.ObjectReference{
				Name:      "test-workload",
				Namespace: "default",
			},
			Recommendation: v1alpha1.RecommendationRightSize,
			ProposedResources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("512Mi"),
				},
			},
			ExpectedImpact: v1alpha1.ExpectedImpact{
				EstimatedSavingsUSD:   func() *resource.Quantity { q := resource.MustParse("50"); return &q }(),
				CarbonSavedGramsCO2:   &carbonSavedQty,
			},
			DecisionMetadata: v1alpha1.DecisionMetadata{
				ConfidenceScore: 85,
				RiskScore:       15,
				Rationale:       "Test rationale",
			},
		},
	}

	currentResources := map[string]corev1.ResourceRequirements{
		"test-container": {
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1000m"),
				corev1.ResourceMemory: resource.MustParse("1Gi"),
			},
		},
	}

	prID, err := prManager.CreateProposalPR(context.Background(), proposal, currentResources)
	assert.NoError(t, err)
	assert.NotEmpty(t, prID)

	assert.Len(t, mockProvider.CreatedPRs, 1)
	assert.Equal(t, "Optimize resources for default/test-workload", mockProvider.CreatedPRs[0].Title)

	prBody := mockProvider.CreatedPRs[0].Description
	assert.Contains(t, prBody, "carbon reduction", "PR description should mention carbon reduction")
	assert.Contains(t, prBody, "12.4 kg CO₂", "PR description should include formatted carbon savings 12.4kg")

	assert.Len(t, mockProvider.AddedComments[prID], 1)
	costComment := mockProvider.AddedComments[prID][0].Body
	assert.Contains(t, costComment, "Carbon Reduction", "Cost comment should include carbon reduction row")
	assert.Contains(t, costComment, "12.4 kg CO₂", "Cost comment should include human carbon figure")

	assert.Len(t, mockProvider.MergedPRs, 1)
	assert.Contains(t, mockProvider.MergedPRs, prID)
}

func TestFormatCarbonImpact(t *testing.T) {
	tests := []struct {
		name       string
		qty        func() *resource.Quantity
		wantSubstr string
		wantEmpty  bool
	}{
		{"nil", func() *resource.Quantity { return nil }, "", true},
		{"zero grams", func() *resource.Quantity { q := resource.MustParse("0"); return &q }, "", true},
		{"150 grams", func() *resource.Quantity { q := resource.MustParse("150"); return &q }, "150 g CO₂", false},
		{"12400 grams → 12.4 kg", func() *resource.Quantity { q := resource.MustParse("12400"); return &q }, "12.4 kg CO₂", false},
		{"1.5 million grams → 1.50 tonnes", func() *resource.Quantity { q := resource.MustParse("1500000"); return &q }, "1.50 tonnes CO₂", false},
		{"999 grams", func() *resource.Quantity { q := resource.MustParse("999"); return &q }, "999 g CO₂", false},
		{"999999 grams just under tonne", func() *resource.Quantity { q := resource.MustParse("999999"); return &q }, "1000.0 kg CO₂", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatCarbonImpact(tc.qty())
			if tc.wantEmpty {
				assert.Empty(t, got, "expected empty string for case %s", tc.name)
				return
			}
			assert.True(t, strings.Contains(got, tc.wantSubstr),
				"formatCarbonImpact() = %q, want substring %q", got, tc.wantSubstr)
		})
	}
}
