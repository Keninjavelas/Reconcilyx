package controller

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/reconcilyx/operator/api/v1alpha1"
)

type stubActuationControl struct {
	paused bool
	err    error
}

func (s stubActuationControl) IsExecutionPaused(context.Context) (bool, error) {
	return s.paused, s.err
}

type stubHealthGate struct {
	err error
}

func (s stubHealthGate) CheckLiveness(context.Context, corev1.ObjectReference) error {
	return s.err
}

type stubAutoscalerGate struct {
	err error
}

func (s stubAutoscalerGate) Check(context.Context, corev1.ObjectReference) error {
	return s.err
}

type stubPolicyGuard struct {
	err error
}

func (s stubPolicyGuard) Check(context.Context, *v1alpha1.OptimizationProposal, *v1alpha1.OptimizationPolicy) error {
	return s.err
}

func scheduledProposal(rec v1alpha1.RecommendationType) *v1alpha1.OptimizationProposal {
	return &v1alpha1.OptimizationProposal{
		Spec: v1alpha1.OptimizationProposalSpec{
			Recommendation: rec,
			TargetWorkload: corev1.ObjectReference{
				Kind:       "Deployment",
				Namespace:  "default",
				Name:       "demo",
				APIVersion: "apps/v1",
			},
		},
		Status: v1alpha1.OptimizationProposalStatus{
			Phase:          v1alpha1.PhaseScheduled,
			ExpirationTime: &metav1.Time{Time: time.Now().Add(time.Hour)},
		},
	}
}

func TestExecutionGateActuatableRecommendations(t *testing.T) {
	gate := NewExecutionGate(stubActuationControl{}, stubHealthGate{}, stubAutoscalerGate{}, nil)

	for _, rec := range []v1alpha1.RecommendationType{
		v1alpha1.RecommendationRightSize,
		v1alpha1.RecommendationReduceCPURequest,
		v1alpha1.RecommendationReduceMemoryRequest,
	} {
		t.Run(string(rec), func(t *testing.T) {
			decision, err := gate.ValidateJIT(context.Background(), scheduledProposal(rec), nil)
			if err != nil {
				t.Fatalf("expected %s to pass JIT validation, got error: %v", rec, err)
			}
			if decision != ExecutionExecute {
				t.Fatalf("expected %s to return %q, got %q", rec, ExecutionExecute, decision)
			}
		})
	}

	for _, rec := range []v1alpha1.RecommendationType{
		v1alpha1.RecommendationScaleZero,
		v1alpha1.RecommendationMigrateCloud,
	} {
		t.Run(string(rec)+"_rejected", func(t *testing.T) {
			decision, err := gate.ValidateJIT(context.Background(), scheduledProposal(rec), nil)
			if err == nil {
				t.Fatalf("expected %s to be rejected", rec)
			}
			if decision != ExecutionReject {
				t.Fatalf("expected %q, got %q", ExecutionReject, decision)
			}
			if !strings.Contains(err.Error(), "not supported by in-place resize actuation") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestExecutionGateSafetyConditions(t *testing.T) {
	proposal := scheduledProposal(v1alpha1.RecommendationRightSize)

	t.Run("paused kill switch defers execution", func(t *testing.T) {
		gate := NewExecutionGate(stubActuationControl{paused: true}, stubHealthGate{}, stubAutoscalerGate{}, nil)
		decision, err := gate.ValidateJIT(context.Background(), proposal, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if decision != ExecutionRetryLater {
			t.Fatalf("expected %q, got %q", ExecutionRetryLater, decision)
		}
	})

	t.Run("unhealthy workload is rejected", func(t *testing.T) {
		gate := NewExecutionGate(stubActuationControl{}, stubHealthGate{err: errors.New("deployment has no ready replicas")}, stubAutoscalerGate{}, nil)
		decision, err := gate.ValidateJIT(context.Background(), proposal, nil)
		if err == nil || decision != ExecutionReject {
			t.Fatalf("expected rejection, got decision=%q err=%v", decision, err)
		}
	})

	t.Run("wrong phase is rejected", func(t *testing.T) {
		p := proposal.DeepCopy()
		p.Status.Phase = v1alpha1.PhaseApproved
		gate := NewExecutionGate(stubActuationControl{}, stubHealthGate{}, stubAutoscalerGate{}, nil)
		decision, err := gate.ValidateJIT(context.Background(), p, nil)
		if err == nil || decision != ExecutionReject {
			t.Fatalf("expected rejection, got decision=%q err=%v", decision, err)
		}
	})

	t.Run("expired proposal expires", func(t *testing.T) {
		p := proposal.DeepCopy()
		p.Status.ExpirationTime = &metav1.Time{Time: time.Now().Add(-time.Minute)}
		gate := NewExecutionGate(stubActuationControl{}, stubHealthGate{}, stubAutoscalerGate{}, nil)
		decision, err := gate.ValidateJIT(context.Background(), p, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if decision != ExecutionExpire {
			t.Fatalf("expected %q, got %q", ExecutionExpire, decision)
		}
	})

	t.Run("competing autoscaler is rejected", func(t *testing.T) {
		gate := NewExecutionGate(stubActuationControl{}, stubHealthGate{}, stubAutoscalerGate{
			err: errors.New("autoscaler conflict: HorizontalPodAutoscaler default/demo targets Deployment/demo"),
		}, nil)
		decision, err := gate.ValidateJIT(context.Background(), proposal, nil)
		if err == nil || decision != ExecutionReject {
			t.Fatalf("expected rejection, got decision=%q err=%v", decision, err)
		}
		if !strings.Contains(err.Error(), "autoscaler conflict") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("policy guard failure is rejected", func(t *testing.T) {
		gate := NewExecutionGate(stubActuationControl{}, stubHealthGate{}, stubAutoscalerGate{}, stubPolicyGuard{
			err: errors.New("change budget exhausted: 2 change(s) in the last 24h (max 2)"),
		})
		decision, err := gate.ValidateJIT(context.Background(), proposal, &v1alpha1.OptimizationPolicy{})
		if err == nil || decision != ExecutionReject {
			t.Fatalf("expected rejection, got decision=%q err=%v", decision, err)
		}
		if !strings.Contains(err.Error(), "change budget exhausted") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
