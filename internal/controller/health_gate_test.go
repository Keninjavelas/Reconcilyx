package controller

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func healthTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := appsv1.AddToScheme(s); err != nil {
		t.Fatalf("add appsv1 to scheme: %v", err)
	}
	return s
}

func healthTestDeployment() *appsv1.Deployment {
	replicas := int32(2)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "demo",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: appsv1.DeploymentSpec{Replicas: &replicas},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 1,
			ReadyReplicas:      2,
			AvailableReplicas:  2,
		},
	}
}

func deploymentRef() corev1.ObjectReference {
	return corev1.ObjectReference{Kind: "Deployment", Namespace: "default", Name: "demo", APIVersion: "apps/v1"}
}

func TestWorkloadHealthGate(t *testing.T) {
	t.Run("healthy deployment passes", func(t *testing.T) {
		gate := NewWorkloadHealthGate(fake.NewClientBuilder().WithScheme(healthTestScheme(t)).WithObjects(healthTestDeployment()).Build())
		if err := gate.CheckLiveness(context.Background(), deploymentRef()); err != nil {
			t.Fatalf("expected healthy deployment to pass, got: %v", err)
		}
	})

	t.Run("missing deployment fails", func(t *testing.T) {
		gate := NewWorkloadHealthGate(fake.NewClientBuilder().WithScheme(healthTestScheme(t)).Build())
		if err := gate.CheckLiveness(context.Background(), deploymentRef()); err == nil {
			t.Fatal("expected missing deployment to fail")
		}
	})

	t.Run("unsupported kind fails", func(t *testing.T) {
		gate := NewWorkloadHealthGate(fake.NewClientBuilder().WithScheme(healthTestScheme(t)).Build())
		ref := deploymentRef()
		ref.Kind = "StatefulSet"
		err := gate.CheckLiveness(context.Background(), ref)
		if err == nil || !strings.Contains(err.Error(), "unsupported workload kind") {
			t.Fatalf("expected unsupported kind error, got: %v", err)
		}
	})

	t.Run("partially ready deployment fails", func(t *testing.T) {
		dep := healthTestDeployment()
		dep.Status.ReadyReplicas = 1
		gate := NewWorkloadHealthGate(fake.NewClientBuilder().WithScheme(healthTestScheme(t)).WithObjects(dep).Build())
		err := gate.CheckLiveness(context.Background(), deploymentRef())
		if err == nil || !strings.Contains(err.Error(), "not fully ready") {
			t.Fatalf("expected not-fully-ready error, got: %v", err)
		}
	})

	t.Run("zero ready replicas fails", func(t *testing.T) {
		dep := healthTestDeployment()
		dep.Status.ReadyReplicas = 0
		gate := NewWorkloadHealthGate(fake.NewClientBuilder().WithScheme(healthTestScheme(t)).WithObjects(dep).Build())
		err := gate.CheckLiveness(context.Background(), deploymentRef())
		if err == nil || !strings.Contains(err.Error(), "not fully ready") {
			t.Fatalf("expected not-fully-ready error, got: %v", err)
		}
	})

	t.Run("scaled to zero fails", func(t *testing.T) {
		dep := healthTestDeployment()
		zero := int32(0)
		dep.Spec.Replicas = &zero
		gate := NewWorkloadHealthGate(fake.NewClientBuilder().WithScheme(healthTestScheme(t)).WithObjects(dep).Build())
		err := gate.CheckLiveness(context.Background(), deploymentRef())
		if err == nil || !strings.Contains(err.Error(), "scaled to zero") {
			t.Fatalf("expected scaled-to-zero error, got: %v", err)
		}
	})

	t.Run("stale observed generation fails", func(t *testing.T) {
		dep := healthTestDeployment()
		dep.Generation = 3
		dep.Status.ObservedGeneration = 2
		gate := NewWorkloadHealthGate(fake.NewClientBuilder().WithScheme(healthTestScheme(t)).WithObjects(dep).Build())
		err := gate.CheckLiveness(context.Background(), deploymentRef())
		if err == nil || !strings.Contains(err.Error(), "has not observed") {
			t.Fatalf("expected stale-generation error, got: %v", err)
		}
	})

	t.Run("replica failure condition fails", func(t *testing.T) {
		dep := healthTestDeployment()
		dep.Status.Conditions = []appsv1.DeploymentCondition{{
			Type:    appsv1.DeploymentReplicaFailure,
			Status:  corev1.ConditionTrue,
			Message: "pods are crashlooping",
		}}
		gate := NewWorkloadHealthGate(fake.NewClientBuilder().WithScheme(healthTestScheme(t)).WithObjects(dep).Build())
		err := gate.CheckLiveness(context.Background(), deploymentRef())
		if err == nil || !strings.Contains(err.Error(), "ReplicaFailure") {
			t.Fatalf("expected ReplicaFailure error, got: %v", err)
		}
	})
}
