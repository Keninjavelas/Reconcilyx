package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	optimizationv1alpha1 "github.com/reconcilyx/operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func apiTestClient(t *testing.T, objects ...*optimizationv1alpha1.OptimizationProposal) *fake.ClientBuilder {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, optimizationv1alpha1.AddToScheme(scheme))
	runtimeObjects := make([]runtime.Object, 0, len(objects))
	for _, obj := range objects {
		runtimeObjects = append(runtimeObjects, obj)
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(runtimeObjects...)
}

func proposalWithPhase(name string, phase optimizationv1alpha1.OptimizationProposalPhase, usd string) *optimizationv1alpha1.OptimizationProposal {
	p := &optimizationv1alpha1.OptimizationProposal{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "live-demo",
			CreationTimestamp: metav1.NewTime(time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)),
		},
		Status: optimizationv1alpha1.OptimizationProposalStatus{
			Phase:         phase,
			ApprovalState: optimizationv1alpha1.ApprovalApproved,
		},
	}
	if usd != "" {
		q := resource.MustParse(usd)
		p.Spec.ExpectedImpact.EstimatedSavingsUSD = &q
	}
	return p
}

func TestSavingsExcludesNonRealizedPhases(t *testing.T) {
	completed := proposalWithPhase("completed", optimizationv1alpha1.PhaseCompleted, "10")
	failed := proposalWithPhase("failed", optimizationv1alpha1.PhaseFailed, "20")
	rolledBack := proposalWithPhase("rolled-back", optimizationv1alpha1.PhaseRolledBack, "7")
	rejected := proposalWithPhase("rejected", optimizationv1alpha1.PhaseRejected, "5")
	stabilizing := proposalWithPhase("stabilizing", optimizationv1alpha1.PhaseStabilizing, "5")
	approved := proposalWithPhase("approved", "", "3")

	cl := apiTestClient(t, completed, failed, rolledBack, rejected, stabilizing, approved).Build()
	ts := httptest.NewServer(NewServer(":0", "", ServerDeps{Client: cl}).Handler())
	defer ts.Close()

	resp, err := ts.Client().Get(ts.URL + "/api/metrics/savings")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body savingsResp
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.NotNil(t, body.TotalUSD)
	assert.InDelta(t, 18.0, *body.TotalUSD, 0.01, "only completed, stabilizing, and approved proposals contribute")
	assert.Len(t, body.Series, 3)
}

func TestProposalListUsesExecutionSnapshotBaseline(t *testing.T) {
	withSnapshot := &optimizationv1alpha1.OptimizationProposal{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "learning",
			Namespace:         "live-demo",
			CreationTimestamp: metav1.NewTime(time.Date(2026, 10, 24, 14, 18, 0, 0, time.UTC)),
		},
		Spec: optimizationv1alpha1.OptimizationProposalSpec{
			ProposedResources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("200m"),
					corev1.ResourceMemory: resource.MustParse("160Mi"),
				},
			},
			ProposedResourcesByContainer: map[string]corev1.ResourceRequirements{
				"app": {
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("160Mi"),
					},
				},
			},
		},
		Status: optimizationv1alpha1.OptimizationProposalStatus{
			Phase:         optimizationv1alpha1.PhaseCompleted,
			ApprovalState: optimizationv1alpha1.ApprovalApproved,
			ExecutionSnapshot: &optimizationv1alpha1.ExecutionSnapshot{
				ContainerResources: map[string]optimizationv1alpha1.ContainerResourceState{
					"app": {Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("250m"),
						corev1.ResourceMemory: resource.MustParse("192Mi"),
					}}},
					"istio-proxy": {Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("250m"),
						corev1.ResourceMemory: resource.MustParse("192Mi"),
					}}},
				},
			},
		},
	}

	withoutBaseline := &optimizationv1alpha1.OptimizationProposal{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "no-baseline",
			Namespace:         "live-demo",
			CreationTimestamp: metav1.NewTime(time.Date(2026, 10, 24, 17, 2, 0, 0, time.UTC)),
		},
		Spec: optimizationv1alpha1.OptimizationProposalSpec{
			ProposedResources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("20m"),
					corev1.ResourceMemory: resource.MustParse("20Mi"),
				},
			},
		},
		Status: optimizationv1alpha1.OptimizationProposalStatus{Phase: optimizationv1alpha1.PhaseRejected},
	}

	withObserved := &optimizationv1alpha1.OptimizationProposal{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "observed",
			Namespace:         "live-demo",
			CreationTimestamp: metav1.NewTime(time.Date(2026, 10, 24, 11, 7, 0, 0, time.UTC)),
		},
		Spec: optimizationv1alpha1.OptimizationProposalSpec{
			ObservedResources: optimizationv1alpha1.ResourceObservation{
				CurrentResources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
			},
		},
		Status: optimizationv1alpha1.OptimizationProposalStatus{Phase: optimizationv1alpha1.PhaseCompleted, ApprovalState: optimizationv1alpha1.ApprovalApproved},
	}

	cl := apiTestClient(t, withSnapshot, withoutBaseline, withObserved).Build()
	ts := httptest.NewServer(NewServer(":0", "", ServerDeps{Client: cl}).Handler())
	defer ts.Close()

	resp, err := ts.Client().Get(ts.URL + "/api/proposals")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Items []proposalListItem `json:"items"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body.Items, 3)

	byName := map[string]proposalListItem{}
	for _, item := range body.Items {
		byName[item.Name] = item
	}
	assert.Equal(t, "250m", byName["learning"].CPUBefore)
	assert.Equal(t, "192Mi", byName["learning"].MemoryBefore)
	assert.Empty(t, byName["no-baseline"].CPUBefore, "a missing baseline must not be rendered as zero")
	assert.Empty(t, byName["no-baseline"].MemoryBefore)
	assert.Equal(t, "500m", byName["observed"].CPUBefore)
	assert.Equal(t, "256Mi", byName["observed"].MemoryBefore)
}
