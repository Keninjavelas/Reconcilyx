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

const testManifest = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-workload
  namespace: default
  labels:
    app: test-workload
spec:
  replicas: 2
  selector:
    matchLabels:
      app: test-workload
  template:
    metadata:
      labels:
        app: test-workload
    spec:
      containers:
        - name: test-container
          image: nginx:1.27
          resources:
            requests:
              cpu: 1000m
              memory: 1Gi
            limits:
              cpu: 2
              memory: 2Gi
        - name: sidecar
          image: busybox:1.36
          resources:
            requests:
              cpu: 50m
              memory: 64Mi
`

func testProposal() *v1alpha1.OptimizationProposal {
	carbonSavedQty := resource.MustParse("12400")
	return &v1alpha1.OptimizationProposal{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-proposal",
			Namespace: "default",
		},
		Spec: v1alpha1.OptimizationProposalSpec{
			TargetWorkload: corev1.ObjectReference{
				Kind:      "Deployment",
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
				EstimatedSavingsUSD: func() *resource.Quantity { q := resource.MustParse("50"); return &q }(),
				CarbonSavedGramsCO2: &carbonSavedQty,
			},
			DecisionMetadata: v1alpha1.DecisionMetadata{
				ConfidenceScore: 85,
				RiskScore:       15,
				Rationale:       "Test rationale",
			},
		},
	}
}

func testCurrentResources() map[string]corev1.ResourceRequirements {
	return map[string]corev1.ResourceRequirements{
		"test-container": {
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1000m"),
				corev1.ResourceMemory: resource.MustParse("1Gi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2"),
				corev1.ResourceMemory: resource.MustParse("2Gi"),
			},
		},
	}
}

func TestPRManager_CreateProposalPR(t *testing.T) {
	mockProvider := NewMockProvider()
	mockProvider.Files["apps/production/test-workload.yaml"] = []byte(testManifest)
	prManager := NewPRManager(mockProvider)

	proposal := testProposal()
	result, err := prManager.CreateProposalPR(context.Background(), proposal, testCurrentResources(), PROptions{
		ManifestsPath: "apps/production",
		BaseBranch:    "main",
		AutoMerge:     true,
	})
	assert.NoError(t, err)
	assert.NotEmpty(t, result.ID)
	assert.Equal(t, "apps/production/test-workload.yaml", result.FilePath)
	assert.True(t, result.AutoMerged)

	assert.Len(t, mockProvider.CreatedPRs, 1)
	pr := mockProvider.CreatedPRs[0]
	assert.Equal(t, "Optimize resources for default/test-workload", pr.Title)
	assert.Equal(t, "apps/production/test-workload.yaml", pr.FilePath)
	assert.Equal(t, "main", pr.BaseBranch)
	assert.NotContains(t, pr.Diff, "diff: update resources", "diff must be a full manifest, not a placeholder")

	// The tracked container is patched; the sidecar is left untouched.
	assert.Contains(t, pr.Diff, "cpu: 500m")
	assert.Contains(t, pr.Diff, "memory: 512Mi")
	assert.Contains(t, pr.Diff, "cpu: 50m", "sidecar resources must be preserved")
	assert.Contains(t, pr.Diff, "reconcilyx.io/last-proposal: test-proposal")
	assert.Contains(t, pr.Diff, "replicas: 2", "unrelated fields must be preserved")

	prBody := pr.Description
	assert.Contains(t, prBody, "carbon reduction", "PR description should mention carbon reduction")
	assert.Contains(t, prBody, "12.4 kg CO₂", "PR description should include formatted carbon savings 12.4kg")
	assert.Contains(t, prBody, autoMergeMarker, "auto-merge policy must be recorded in the PR body")

	assert.Len(t, mockProvider.AddedComments[result.ID], 1)
	costComment := mockProvider.AddedComments[result.ID][0].Body
	assert.Contains(t, costComment, "Carbon Reduction", "Cost comment should include carbon reduction row")
	assert.Contains(t, costComment, "12.4 kg CO₂", "Cost comment should include human carbon figure")

	assert.Len(t, mockProvider.MergedPRs, 1)
	assert.Contains(t, mockProvider.MergedPRs, result.ID)
}

func TestPRManager_AutoMergeRespectsPolicy(t *testing.T) {
	mockProvider := NewMockProvider()
	mockProvider.Files["manifests/test-workload.yaml"] = []byte(testManifest)
	prManager := NewPRManager(mockProvider)

	result, err := prManager.CreateProposalPR(context.Background(), testProposal(), testCurrentResources(), PROptions{
		ManifestsPath: "manifests",
		AutoMerge:     false,
	})
	assert.NoError(t, err)
	assert.False(t, result.AutoMerged)
	assert.Empty(t, mockProvider.MergedPRs)
	assert.NotContains(t, mockProvider.CreatedPRs[0].Description, autoMergeMarker)
}

func TestPRManager_MissingManifestFails(t *testing.T) {
	mockProvider := NewMockProvider()
	prManager := NewPRManager(mockProvider)

	_, err := prManager.CreateProposalPR(context.Background(), testProposal(), testCurrentResources(), PROptions{
		ManifestsPath: "apps/production",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no Deployment manifest")
	assert.Empty(t, mockProvider.CreatedPRs, "no PR should be opened without a manifest patch")
}

func TestPRManager_LowConfidenceDoesNotAutoMerge(t *testing.T) {
	mockProvider := NewMockProvider()
	mockProvider.Files["manifests/test-workload.yaml"] = []byte(testManifest)
	prManager := NewPRManager(mockProvider)

	proposal := testProposal()
	proposal.Spec.DecisionMetadata.ConfidenceScore = 60
	result, err := prManager.CreateProposalPR(context.Background(), proposal, testCurrentResources(), PROptions{
		ManifestsPath: "manifests",
		AutoMerge:     true,
	})
	assert.NoError(t, err)
	assert.False(t, result.AutoMerged)
	assert.Empty(t, mockProvider.MergedPRs)
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
