package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	optimizationv1alpha1 "github.com/reconcilyx/operator/api/v1alpha1"
)

func TestDryRunProposalLifecycleIsImmutable(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, optimizationv1alpha1.AddToScheme(scheme))

	proposal := &optimizationv1alpha1.OptimizationProposal{
		ObjectMeta: metav1.ObjectMeta{Name: "dry-run-proposal", Namespace: "default"},
		Spec: optimizationv1alpha1.OptimizationProposalSpec{
			Recommendation: optimizationv1alpha1.RecommendationRightSize,
		},
		Status: optimizationv1alpha1.OptimizationProposalStatus{
			Phase:         optimizationv1alpha1.PhaseDryRun,
			ApprovalState: optimizationv1alpha1.ApprovalDryRun,
			DryRun: &optimizationv1alpha1.DryRunResult{
				WouldBe:     optimizationv1alpha1.ApprovalApproved,
				Reason:      "Dry run (hypothetical AutoApply)",
				EvaluatedAt: metav1.Now(),
			},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(proposal).
		WithStatusSubresource(&optimizationv1alpha1.OptimizationProposal{}).
		Build()
	server := NewServer(":0", "", ServerDeps{Client: c})
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	t.Run("approve is rejected", func(t *testing.T) {
		resp := doRequest(t, ts, http.MethodPost, "/api/proposals/dry-run-proposal/approve?namespace=default", nil)
		assert.Equal(t, http.StatusConflict, resp.StatusCode)
		assert.Contains(t, readResponseBody(t, resp), "dry-run")
	})

	t.Run("reject is rejected", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/proposals/dry-run-proposal/reject?namespace=default", strings.NewReader(`{"reason":"dismiss"}`))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		resp, err := ts.Client().Do(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusConflict, resp.StatusCode)
		assert.Contains(t, readResponseBody(t, resp), "observational")
	})

	var stored optimizationv1alpha1.OptimizationProposal
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: proposal.Name, Namespace: proposal.Namespace}, &stored))
	assert.Equal(t, optimizationv1alpha1.PhaseDryRun, stored.Status.Phase)
	assert.Equal(t, optimizationv1alpha1.ApprovalDryRun, stored.Status.ApprovalState)
}
