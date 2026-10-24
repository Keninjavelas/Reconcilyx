package controller

import (
	"context"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	optimizationv1alpha1 "github.com/reconcilyx/operator/api/v1alpha1"
)

func policyGuardScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add apps scheme: %v", err)
	}
	if err := optimizationv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add optimization scheme: %v", err)
	}
	return scheme
}

func guardPolicy() *optimizationv1alpha1.OptimizationPolicy {
	return &optimizationv1alpha1.OptimizationPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "guard-policy", Namespace: "default"},
	}
}

func snapshotProposal(name string, age time.Duration) *optimizationv1alpha1.OptimizationProposal {
	return &optimizationv1alpha1.OptimizationProposal{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: optimizationv1alpha1.OptimizationProposalSpec{
			TargetWorkload: corev1.ObjectReference{Kind: "Deployment", Namespace: "default", Name: "demo"},
		},
		Status: optimizationv1alpha1.OptimizationProposalStatus{
			ExecutionSnapshot: &optimizationv1alpha1.ExecutionSnapshot{
				Timestamp: metav1.Time{Time: time.Now().Add(-age)},
			},
		},
	}
}

func guardDeployment(replicas int32, labels map[string]string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
			},
		},
	}
}

func TestPolicyGuardChangeBudget(t *testing.T) {
	current := snapshotProposal("current", 0)
	current.Status.ExecutionSnapshot = nil

	t.Run("disabled budget allows execution", func(t *testing.T) {
		policy := guardPolicy()
		policy.Spec.ChangeBudget.MaxChangesPerWindow = 0
		c := fake.NewClientBuilder().WithScheme(policyGuardScheme(t)).WithObjects(
			guardDeployment(1, nil), current, snapshotProposal("recent", time.Hour),
		).Build()
		if err := NewPolicyGuard(c).Check(context.Background(), current, policy); err != nil {
			t.Fatalf("expected disabled budget to pass, got %v", err)
		}
	})

	t.Run("changes inside the window are counted", func(t *testing.T) {
		policy := guardPolicy()
		policy.Spec.ChangeBudget.MaxChangesPerWindow = 1
		c := fake.NewClientBuilder().WithScheme(policyGuardScheme(t)).WithObjects(
			guardDeployment(1, nil),
			current,
			snapshotProposal("recent", time.Hour),
			snapshotProposal("old", 30*time.Hour),
		).Build()
		if err := NewPolicyGuard(c).Check(context.Background(), current, policy); err == nil {
			t.Fatal("expected the recent change to exhaust a max=1 budget")
		} else if !strings.Contains(err.Error(), "change budget exhausted") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("old changes and other workloads do not count", func(t *testing.T) {
		policy := guardPolicy()
		policy.Spec.ChangeBudget.MaxChangesPerWindow = 1
		other := snapshotProposal("other", time.Hour)
		other.Spec.TargetWorkload.Name = "another"
		c := fake.NewClientBuilder().WithScheme(policyGuardScheme(t)).WithObjects(
			guardDeployment(1, nil),
			current,
			snapshotProposal("old", 30*time.Hour),
			other,
		).Build()
		if err := NewPolicyGuard(c).Check(context.Background(), current, policy); err != nil {
			t.Fatalf("expected old/other changes not to count, got %v", err)
		}
	})

	t.Run("current proposal is excluded from its own budget", func(t *testing.T) {
		policy := guardPolicy()
		policy.Spec.ChangeBudget.MaxChangesPerWindow = 1
		retrying := snapshotProposal("current", time.Minute)
		c := fake.NewClientBuilder().WithScheme(policyGuardScheme(t)).WithObjects(
			guardDeployment(1, nil), retrying,
		).Build()
		if err := NewPolicyGuard(c).Check(context.Background(), retrying, policy); err != nil {
			t.Fatalf("expected the current proposal to be excluded, got %v", err)
		}
	})
}

func TestPolicyGuardCooldown(t *testing.T) {
	proposal := snapshotProposal("current", 0)
	proposal.Status.ExecutionSnapshot = nil

	t.Run("recent change blocks execution", func(t *testing.T) {
		policy := guardPolicy()
		policy.Spec.SafetyConstraints.CooldownPeriod = &metav1.Duration{Duration: 30 * time.Minute}
		c := fake.NewClientBuilder().WithScheme(policyGuardScheme(t)).WithObjects(
			proposal, snapshotProposal("recent", 5*time.Minute),
		).Build()
		err := NewPolicyGuard(c).Check(context.Background(), proposal, policy)
		if err == nil || !strings.Contains(err.Error(), "cooldown active") {
			t.Fatalf("expected cooldown rejection, got %v", err)
		}
	})

	t.Run("change outside the cooldown passes", func(t *testing.T) {
		policy := guardPolicy()
		policy.Spec.SafetyConstraints.CooldownPeriod = &metav1.Duration{Duration: 30 * time.Minute}
		c := fake.NewClientBuilder().WithScheme(policyGuardScheme(t)).WithObjects(
			proposal, snapshotProposal("old", 2*time.Hour),
		).Build()
		if err := NewPolicyGuard(c).Check(context.Background(), proposal, policy); err != nil {
			t.Fatalf("expected stale change to pass, got %v", err)
		}
	})

	t.Run("absent cooldown is disabled", func(t *testing.T) {
		policy := guardPolicy()
		c := fake.NewClientBuilder().WithScheme(policyGuardScheme(t)).WithObjects(
			proposal, snapshotProposal("recent", time.Minute),
		).Build()
		if err := NewPolicyGuard(c).Check(context.Background(), proposal, policy); err != nil {
			t.Fatalf("expected absent cooldown to pass, got %v", err)
		}
	})

	t.Run("current proposal does not block itself", func(t *testing.T) {
		policy := guardPolicy()
		policy.Spec.SafetyConstraints.CooldownPeriod = &metav1.Duration{Duration: 30 * time.Minute}
		retrying := snapshotProposal("current", time.Minute)
		c := fake.NewClientBuilder().WithScheme(policyGuardScheme(t)).WithObjects(retrying).Build()
		if err := NewPolicyGuard(c).Check(context.Background(), retrying, policy); err != nil {
			t.Fatalf("expected the current proposal to be excluded, got %v", err)
		}
	})
}

func TestPolicyGuardBlastRadius(t *testing.T) {
	proposal := snapshotProposal("current", 0)
	proposal.Status.ExecutionSnapshot = nil

	t.Run("replica cap rejects a larger workload", func(t *testing.T) {
		policy := guardPolicy()
		policy.Spec.BlastRadius.MaxReplicasPerChange = 3
		c := fake.NewClientBuilder().WithScheme(policyGuardScheme(t)).WithObjects(guardDeployment(5, nil), proposal).Build()
		err := NewPolicyGuard(c).Check(context.Background(), proposal, policy)
		if err == nil || !strings.Contains(err.Error(), "blast radius exceeded") {
			t.Fatalf("expected blast radius rejection, got %v", err)
		}
	})

	t.Run("replica cap allows a workload at the limit", func(t *testing.T) {
		policy := guardPolicy()
		policy.Spec.BlastRadius.MaxReplicasPerChange = 5
		c := fake.NewClientBuilder().WithScheme(policyGuardScheme(t)).WithObjects(guardDeployment(5, nil), proposal).Build()
		if err := NewPolicyGuard(c).Check(context.Background(), proposal, policy); err != nil {
			t.Fatalf("expected workload at the cap to pass, got %v", err)
		}
	})

	t.Run("canary label is required on the pod template", func(t *testing.T) {
		policy := guardPolicy()
		policy.Spec.BlastRadius.RequireCanaryLabel = true
		c := fake.NewClientBuilder().WithScheme(policyGuardScheme(t)).WithObjects(guardDeployment(1, nil), proposal).Build()
		err := NewPolicyGuard(c).Check(context.Background(), proposal, policy)
		if err == nil || !strings.Contains(err.Error(), "canary label missing") {
			t.Fatalf("expected canary rejection, got %v", err)
		}

		c = fake.NewClientBuilder().WithScheme(policyGuardScheme(t)).WithObjects(
			guardDeployment(1, map[string]string{optimizationv1alpha1.CanaryLabelKey: "true"}),
			proposal,
		).Build()
		if err := NewPolicyGuard(c).Check(context.Background(), proposal, policy); err != nil {
			t.Fatalf("expected canary-labelled workload to pass, got %v", err)
		}
	})

	t.Run("replica cap fails closed for DaemonSets", func(t *testing.T) {
		policy := guardPolicy()
		policy.Spec.BlastRadius.MaxReplicasPerChange = 1
		daemonProposal := proposal.DeepCopy()
		daemonProposal.Spec.TargetWorkload.Kind = "DaemonSet"
		daemonSet := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"}}
		c := fake.NewClientBuilder().WithScheme(policyGuardScheme(t)).WithObjects(daemonSet, daemonProposal).Build()
		err := NewPolicyGuard(c).Check(context.Background(), daemonProposal, policy)
		if err == nil || !strings.Contains(err.Error(), "not supported") {
			t.Fatalf("expected DaemonSet cap rejection, got %v", err)
		}
	})
}
