package controller

import (
	"context"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	optimizationv1alpha1 "github.com/reconcilyx/operator/api/v1alpha1"
	"github.com/reconcilyx/operator/internal/policy"
)

func deploymentWithContainers(name, namespace string, containers ...string) *appsv1.Deployment {
	specs := make([]corev1.Container, 0, len(containers))
	for _, container := range containers {
		specs = append(specs, corev1.Container{Name: container})
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: specs}},
		},
	}
}

func TestLastExecutionForWorkload(t *testing.T) {
	base := time.Now().Add(-3 * time.Hour).Truncate(time.Second)
	proposal := &optimizationv1alpha1.OptimizationProposal{
		ObjectMeta: metav1.ObjectMeta{Name: "current", Namespace: "default"},
		Spec: optimizationv1alpha1.OptimizationProposalSpec{
			TargetWorkload: corev1.ObjectReference{Kind: "Deployment", Namespace: "default", Name: "demo"},
		},
	}
	executed := func(name string, at time.Time) *optimizationv1alpha1.OptimizationProposal {
		other := proposal.DeepCopy()
		other.Name = name
		other.Status.ExecutionSnapshot = &optimizationv1alpha1.ExecutionSnapshot{Timestamp: metav1.Time{Time: at}}
		return other
	}
	otherWorkload := executed("other", base.Add(2*time.Hour))
	otherWorkload.Spec.TargetWorkload.Name = "another"

	c := fake.NewClientBuilder().WithScheme(policyGuardScheme(t)).WithObjects(
		proposal,
		executed("older", base),
		executed("newest", base.Add(time.Hour)),
		otherWorkload,
	).Build()

	last, err := lastExecutionForWorkload(context.Background(), c, proposal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if last == nil || !last.Time.Equal(base.Add(time.Hour)) {
		t.Fatalf("expected the newest other-workload-independent execution, got %v", last)
	}
}

func TestLastExecutionForWorkloadFallsBackToProposalNamespace(t *testing.T) {
	at := time.Now().Add(-10 * time.Minute).Truncate(time.Second)
	proposal := &optimizationv1alpha1.OptimizationProposal{
		ObjectMeta: metav1.ObjectMeta{Name: "current", Namespace: "default"},
		Spec: optimizationv1alpha1.OptimizationProposalSpec{
			TargetWorkload: corev1.ObjectReference{Kind: "Deployment", Name: "demo"},
		},
	}
	other := proposal.DeepCopy()
	other.Name = "previous"
	other.Status.ExecutionSnapshot = &optimizationv1alpha1.ExecutionSnapshot{Timestamp: metav1.Time{Time: at}}

	c := fake.NewClientBuilder().WithScheme(policyGuardScheme(t)).WithObjects(proposal, other).Build()
	last, err := lastExecutionForWorkload(context.Background(), c, proposal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if last == nil || !last.Time.Equal(at) {
		t.Fatalf("expected an execution matched via the proposal namespace, got %v", last)
	}
}

func TestWorkloadContainerNames(t *testing.T) {
	proposal := &optimizationv1alpha1.OptimizationProposal{
		ObjectMeta: metav1.ObjectMeta{Name: "current", Namespace: "default"},
		Spec: optimizationv1alpha1.OptimizationProposalSpec{
			TargetWorkload: corev1.ObjectReference{Kind: "Deployment", Name: "demo"},
		},
	}

	t.Run("deployment containers", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(policyGuardScheme(t)).WithObjects(
			deploymentWithContainers("demo", "default", "web", "istio-proxy"),
		).Build()
		names, err := workloadContainerNames(context.Background(), c, proposal)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(names) != 2 || names[0] != "web" || names[1] != "istio-proxy" {
			t.Fatalf("unexpected container names: %v", names)
		}
	})

	t.Run("statefulset containers", func(t *testing.T) {
		statefulSet := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"},
			Spec: appsv1.StatefulSetSpec{Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "db"}}},
			}},
		}
		ssProposal := proposal.DeepCopy()
		ssProposal.Spec.TargetWorkload.Kind = "StatefulSet"
		c := fake.NewClientBuilder().WithScheme(policyGuardScheme(t)).WithObjects(statefulSet).Build()
		names, err := workloadContainerNames(context.Background(), c, ssProposal)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(names) != 1 || names[0] != "db" {
			t.Fatalf("unexpected container names: %v", names)
		}
	})

	t.Run("missing workload yields no names", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(policyGuardScheme(t)).Build()
		names, err := workloadContainerNames(context.Background(), c, proposal)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(names) != 0 {
			t.Fatalf("expected no names, got %v", names)
		}
	})

	t.Run("unsupported kind yields no names", func(t *testing.T) {
		other := proposal.DeepCopy()
		other.Spec.TargetWorkload.Kind = "Job"
		c := fake.NewClientBuilder().WithScheme(policyGuardScheme(t)).Build()
		names, err := workloadContainerNames(context.Background(), c, other)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(names) != 0 {
			t.Fatalf("expected no names, got %v", names)
		}
	})
}

func TestWorkloadMatchLabels(t *testing.T) {
	proposal := &optimizationv1alpha1.OptimizationProposal{
		ObjectMeta: metav1.ObjectMeta{Name: "current", Namespace: "default"},
		Spec: optimizationv1alpha1.OptimizationProposalSpec{
			TargetWorkload: corev1.ObjectReference{Kind: "Deployment", Namespace: "team-a", Name: "demo"},
		},
	}

	t.Run("namespace and deployment labels", func(t *testing.T) {
		namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name:   "team-a",
			Labels: map[string]string{"env": "prod"},
		}}
		dep := deploymentWithContainers("demo", "team-a", "app")
		dep.Labels = map[string]string{"app": "web"}
		c := fake.NewClientBuilder().WithScheme(policyGuardScheme(t)).WithObjects(namespace, dep).Build()

		nsLabels, workloadLabels, err := workloadMatchLabels(context.Background(), c, proposal)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if nsLabels["env"] != "prod" {
			t.Fatalf("expected namespace label env=prod, got %v", nsLabels)
		}
		if workloadLabels["app"] != "web" {
			t.Fatalf("expected workload label app=web, got %v", workloadLabels)
		}
	})

	t.Run("missing target yields empty labels", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(policyGuardScheme(t)).Build()
		nsLabels, workloadLabels, err := workloadMatchLabels(context.Background(), c, proposal)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(nsLabels) != 0 || len(workloadLabels) != 0 {
			t.Fatalf("expected empty labels, got namespace=%v workload=%v", nsLabels, workloadLabels)
		}
	})
}

func evaluationProposal() *optimizationv1alpha1.OptimizationProposal {
	return &optimizationv1alpha1.OptimizationProposal{
		ObjectMeta: metav1.ObjectMeta{Name: "current", Namespace: "default"},
		Spec: optimizationv1alpha1.OptimizationProposalSpec{
			TargetWorkload:   corev1.ObjectReference{Kind: "Deployment", Namespace: "default", Name: "demo"},
			Recommendation:   optimizationv1alpha1.RecommendationRightSize,
			DecisionMetadata: optimizationv1alpha1.DecisionMetadata{RiskScore: 10, ConfidenceScore: 90},
		},
	}
}

func evaluationPolicy() *optimizationv1alpha1.OptimizationPolicy {
	pol := guardPolicy()
	pol.Spec.RecommendationGovernance.AllowedRecommendations = []optimizationv1alpha1.RecommendationType{
		optimizationv1alpha1.RecommendationRightSize,
	}
	return pol
}

func TestEvaluatePolicyUsesExcludedContainers(t *testing.T) {
	proposal := evaluationProposal()
	pol := evaluationPolicy()
	pol.Spec.WorkloadProtection.ExcludedContainers = []string{"istio-proxy"}
	c := fake.NewClientBuilder().WithScheme(policyGuardScheme(t)).WithObjects(
		deploymentWithContainers("demo", "default", "web", "istio-proxy"),
	).Build()
	reconciler := &OptimizationProposalReconciler{Client: c, Evaluator: policy.NewEvaluator()}

	decision, reason, err := reconciler.evaluatePolicy(context.Background(), proposal, pol)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision != optimizationv1alpha1.ApprovalRejected || !strings.Contains(reason, "protected sidecar") {
		t.Fatalf("expected exclusion rejection, got %s: %s", decision, reason)
	}
}

func TestEvaluatePolicyUsesCooldown(t *testing.T) {
	proposal := evaluationProposal()
	pol := evaluationPolicy()
	pol.Spec.SafetyConstraints.CooldownPeriod = &metav1.Duration{Duration: 30 * time.Minute}
	previous := proposal.DeepCopy()
	previous.Name = "previous"
	previous.Status.ExecutionSnapshot = &optimizationv1alpha1.ExecutionSnapshot{
		Timestamp: metav1.Time{Time: time.Now().Add(-5 * time.Minute)},
	}
	c := fake.NewClientBuilder().WithScheme(policyGuardScheme(t)).WithObjects(
		deploymentWithContainers("demo", "default", "web"), previous,
	).Build()
	reconciler := &OptimizationProposalReconciler{Client: c, Evaluator: policy.NewEvaluator()}

	decision, reason, err := reconciler.evaluatePolicy(context.Background(), proposal, pol)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision != optimizationv1alpha1.ApprovalRejected || !strings.Contains(reason, "Cooldown active") {
		t.Fatalf("expected cooldown rejection, got %s: %s", decision, reason)
	}
}
