package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	optimizationv1alpha1 "github.com/reconcilyx/operator/api/v1alpha1"
	"github.com/reconcilyx/operator/internal/gitops"
)

type stubGitFactory struct {
	provider gitops.GitProvider
	err      error
}

func (f stubGitFactory) ForPolicy(*optimizationv1alpha1.OptimizationPolicy) (gitops.GitProvider, error) {
	return f.provider, f.err
}

func approvalTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, optimizationv1alpha1.AddToScheme(s))
	require.NoError(t, appsv1.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))
	return s
}

func externalApprovalProposal() *optimizationv1alpha1.OptimizationProposal {
	return &optimizationv1alpha1.OptimizationProposal{
		ObjectMeta: metav1.ObjectMeta{Name: "proposal", Namespace: "default"},
		Spec: optimizationv1alpha1.OptimizationProposalSpec{
			TargetWorkload: corev1.ObjectReference{Kind: "Deployment", Name: "demo", Namespace: "default"},
		},
		Status: optimizationv1alpha1.OptimizationProposalStatus{
			Phase:          optimizationv1alpha1.PhaseAwaitingExternalApproval,
			PolicyApplied:  "policy",
			ExternalPRID:   "42",
			ExpirationTime: &metav1.Time{Time: time.Now().Add(time.Hour)},
		},
	}
}

func approvalTestPolicy() *optimizationv1alpha1.OptimizationPolicy {
	return &optimizationv1alpha1.OptimizationPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "policy", Namespace: "default"},
		Spec: optimizationv1alpha1.OptimizationPolicySpec{
			GitOps: optimizationv1alpha1.GitOpsConfig{
				Enabled:       true,
				Provider:      optimizationv1alpha1.GitProviderMock,
				ManifestsPath: "manifests",
				AutoMerge:     true,
			},
		},
	}
}

func TestReconcileExternalApproval(t *testing.T) {
	tests := []struct {
		name         string
		status       *gitops.PullRequestStatus
		wantPhase    optimizationv1alpha1.OptimizationProposalPhase
		wantRequeue  bool
		wantApproval optimizationv1alpha1.ApprovalState
	}{
		{
			name:      "merged completes the proposal",
			status:    &gitops.PullRequestStatus{State: "merged", Merged: true},
			wantPhase: optimizationv1alpha1.PhaseCompleted,
		},
		{
			name:         "closed rejects the proposal",
			status:       &gitops.PullRequestStatus{State: "closed"},
			wantPhase:    optimizationv1alpha1.PhaseRejected,
			wantApproval: optimizationv1alpha1.ApprovalRejected,
		},
		{
			name:        "open keeps polling",
			status:      &gitops.PullRequestStatus{State: "open", Mergeable: true},
			wantPhase:   optimizationv1alpha1.PhaseAwaitingExternalApproval,
			wantRequeue: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := gitops.NewMockProvider()
			provider.PRStatuses["42"] = tc.status

			c := fake.NewClientBuilder().
				WithScheme(approvalTestScheme(t)).
				WithStatusSubresource(&optimizationv1alpha1.OptimizationProposal{}).
				WithObjects(externalApprovalProposal(), approvalTestPolicy()).
				Build()
			r := &OptimizationProposalReconciler{
				Client:     c,
				GitFactory: stubGitFactory{provider: provider},
				Recorder:   record.NewFakeRecorder(10),
			}

			proposal := &optimizationv1alpha1.OptimizationProposal{}
			require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "proposal"}, proposal))
			result, err := r.reconcileExternalApproval(context.Background(), proposal)
			require.NoError(t, err)

			updated := &optimizationv1alpha1.OptimizationProposal{}
			require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "proposal"}, updated))
			assert.Equal(t, tc.wantPhase, updated.Status.Phase, "phase; reason=%s", updated.Status.Reason)
			if tc.wantApproval != "" {
				assert.Equal(t, tc.wantApproval, updated.Status.ApprovalState)
			}
			if tc.wantRequeue {
				assert.Greater(t, result.RequeueAfter, time.Duration(0))
			}
		})
	}
}

func TestReconcileExternalApproval_MissingPolicyRejects(t *testing.T) {
	c := fake.NewClientBuilder().
		WithScheme(approvalTestScheme(t)).
		WithStatusSubresource(&optimizationv1alpha1.OptimizationProposal{}).
		WithObjects(externalApprovalProposal()).
		Build()
	r := &OptimizationProposalReconciler{
		Client:     c,
		GitFactory: stubGitFactory{provider: gitops.NewMockProvider()},
		Recorder:   record.NewFakeRecorder(10),
	}

	proposal := &optimizationv1alpha1.OptimizationProposal{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "proposal"}, proposal))
	_, err := r.reconcileExternalApproval(context.Background(), proposal)
	require.NoError(t, err)

	updated := &optimizationv1alpha1.OptimizationProposal{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "proposal"}, updated))
	assert.Equal(t, optimizationv1alpha1.PhaseRejected, updated.Status.Phase)
	assert.Equal(t, optimizationv1alpha1.ApprovalRejected, updated.Status.ApprovalState)
	assert.Contains(t, updated.Status.Reason, "no longer exists")
}

func TestCreateGitOpsPR_UsesManifestsPathAndPolicyAutoMerge(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "app",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("1"),
								corev1.ResourceMemory: resource.MustParse("1Gi"),
							},
						},
					}},
				},
			},
		},
	}
	provider := gitops.NewMockProvider()
	provider.Files["manifests/demo.yaml"] = []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo
  namespace: default
spec:
  template:
    spec:
      containers:
        - name: app
          resources:
            requests:
              cpu: "1"
              memory: 1Gi
`)

	c := fake.NewClientBuilder().WithScheme(approvalTestScheme(t)).WithObjects(dep).Build()
	r := &OptimizationProposalReconciler{
		Client:     c,
		GitFactory: stubGitFactory{provider: provider},
		Recorder:   record.NewFakeRecorder(10),
	}

	proposal := &optimizationv1alpha1.OptimizationProposal{
		ObjectMeta: metav1.ObjectMeta{Name: "proposal", Namespace: "default"},
		Spec: optimizationv1alpha1.OptimizationProposalSpec{
			TargetWorkload: corev1.ObjectReference{Kind: "Deployment", Name: "demo", Namespace: "default"},
			Recommendation: optimizationv1alpha1.RecommendationRightSize,
			ProposedResources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("512Mi"),
				},
			},
			DecisionMetadata: optimizationv1alpha1.DecisionMetadata{ConfidenceScore: 90, RiskScore: 10},
		},
	}

	prID, err := r.createGitOpsPR(context.Background(), proposal, approvalTestPolicy())
	require.NoError(t, err)
	assert.Equal(t, "pr-1", prID)
	require.Len(t, provider.CreatedPRs, 1)
	assert.Equal(t, "manifests/demo.yaml", provider.CreatedPRs[0].FilePath)
	assert.Equal(t, "main", provider.CreatedPRs[0].BaseBranch)
	assert.Contains(t, provider.CreatedPRs[0].Diff, "cpu: 500m")
	assert.Contains(t, provider.MergedPRs, "pr-1", "policy autoMerge=true should merge high-confidence PRs")
}
