package controller

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/reconcilyx/operator/api/v1alpha1"
)

func TestResizeContainersPreservesPodOrder(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "istio-proxy"},
				{Name: "app"},
				{Name: "metrics"},
			},
		},
	}
	desired := map[string]corev1.ResourceRequirements{
		"metrics": {},
		"app": {
			Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("250m")},
		},
	}

	got := resizeContainers(pod, desired)
	if len(got) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(got))
	}
	if got[0].Name != "app" || got[1].Name != "metrics" {
		t.Fatalf("expected pod order [app metrics], got [%s %s]", got[0].Name, got[1].Name)
	}
	if got[0].Resources.Requests.Cpu().String() != "250m" {
		t.Fatalf("expected app request 250m, got %s", got[0].Resources.Requests.Cpu().String())
	}
}

func TestResizeContainersSkipsUntrackedContainers(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app"}, {Name: "sidecar"}},
		},
	}

	got := resizeContainers(pod, map[string]corev1.ResourceRequirements{"app": {}})
	if len(got) != 1 || got[0].Name != "app" {
		t.Fatalf("expected only app in the resize patch, got %#v", got)
	}
}

func TestDesiredResizeResourcesKeepsUnselectedContainers(t *testing.T) {
	// Regression: omitting a container from the SSA patch drops requests the
	// operator's field manager already owns, and the API server rejects the
	// resize with "resource requests cannot be removed".
	pod := resizePod("250m", "192Mi", "250m", "192Mi")
	proposal := &v1alpha1.OptimizationProposal{
		Spec: v1alpha1.OptimizationProposalSpec{
			Recommendation: v1alpha1.RecommendationRightSize,
			ProposedResourcesByContainer: map[string]corev1.ResourceRequirements{
				"app": snapshotResources("200m", "160Mi"),
			},
		},
	}
	snapshot := &v1alpha1.ExecutionSnapshot{
		ContainerResources: map[string]v1alpha1.ContainerResourceState{
			"app": {}, "istio-proxy": {},
		},
	}

	desired := desiredResizeResources(pod, proposal, snapshot)
	if len(desired) != len(pod.Spec.Containers) {
		t.Fatalf("expected every pod container in the resize patch, got %d", len(desired))
	}
	app := desired["app"].Requests
	if app.Cpu().String() != "200m" || app.Memory().String() != "160Mi" {
		t.Fatalf("app requests = %s/%s, want 200m/160Mi", app.Cpu(), app.Memory())
	}
	proxy := desired["istio-proxy"].Requests
	if proxy.Cpu().String() != "250m" || proxy.Memory().String() != "192Mi" {
		t.Fatalf("sidecar requests = %s/%s, want its live 250m/192Mi preserved", proxy.Cpu(), proxy.Memory())
	}
}

func TestDesiredResizeResourcesPreservesUntouchedFields(t *testing.T) {
	pod := resizePod("250m", "192Mi", "100m", "64Mi")
	pod.Spec.Containers[0].Resources.Limits = corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("500m"),
		corev1.ResourceMemory: resource.MustParse("256Mi"),
	}
	proposal := &v1alpha1.OptimizationProposal{
		Spec: v1alpha1.OptimizationProposalSpec{
			Recommendation: v1alpha1.RecommendationReduceCPURequest,
			ProposedResourcesByContainer: map[string]corev1.ResourceRequirements{
				"app": {Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("200m")}},
			},
		},
	}
	snapshot := &v1alpha1.ExecutionSnapshot{
		ContainerResources: map[string]v1alpha1.ContainerResourceState{"app": {}, "istio-proxy": {}},
	}

	app := desiredResizeResources(pod, proposal, snapshot)["app"]
	if app.Requests.Cpu().String() != "200m" {
		t.Fatalf("expected cpu request 200m, got %s", app.Requests.Cpu())
	}
	if app.Requests.Memory().String() != "192Mi" {
		t.Fatalf("expected memory request 192Mi to be preserved, got %s", app.Requests.Memory())
	}
	if app.Limits.Cpu().String() != "500m" || app.Limits.Memory().String() != "256Mi" {
		t.Fatalf("expected limits 500m/256Mi to be preserved, got %s/%s", app.Limits.Cpu(), app.Limits.Memory())
	}
}

func TestDesiredResizeResourcesAppliesLegacySingleContainerProposal(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Resources: snapshotResources("500m", "256Mi")}}},
	}
	proposal := &v1alpha1.OptimizationProposal{
		Spec: v1alpha1.OptimizationProposalSpec{
			Recommendation:    v1alpha1.RecommendationRightSize,
			ProposedResources: snapshotResources("400m", "200Mi"),
		},
	}
	snapshot := &v1alpha1.ExecutionSnapshot{
		ContainerResources: map[string]v1alpha1.ContainerResourceState{"app": {}},
	}

	desired := desiredResizeResources(pod, proposal, snapshot)
	if len(desired) != 1 {
		t.Fatalf("expected one container in the resize patch, got %d", len(desired))
	}
	app := desired["app"].Requests
	if app.Cpu().String() != "400m" || app.Memory().String() != "200Mi" {
		t.Fatalf("app requests = %s/%s, want the legacy proposal's 400m/200Mi", app.Cpu(), app.Memory())
	}
}

func TestResizeContainersApplyCompleteDesiredMap(t *testing.T) {
	pod := resizePod("200m", "160Mi", "250m", "192Mi")
	desired := map[string]corev1.ResourceRequirements{
		"app":         snapshotResources("200m", "160Mi"),
		"istio-proxy": snapshotResources("250m", "192Mi"),
	}

	got := resizeContainers(pod, desired)
	if len(got) != 2 || got[0].Name != "app" || got[1].Name != "istio-proxy" {
		t.Fatalf("expected both containers in pod order, got %#v", got)
	}
}

func resizePod(appCPU, appMemory, proxyCPU, proxyMemory string) *corev1.Pod {
	return &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", Resources: snapshotResources(appCPU, appMemory)},
				{Name: "istio-proxy", Resources: snapshotResources(proxyCPU, proxyMemory)},
			},
		},
	}
}

func TestProposedResourcesForContainer(t *testing.T) {
	proposal := &v1alpha1.OptimizationProposal{
		Spec: v1alpha1.OptimizationProposalSpec{
			ProposedResources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("9")},
			},
			ProposedResourcesByContainer: map[string]corev1.ResourceRequirements{
				"app": {Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("250m")}},
			},
		},
	}

	app, ok := proposedResourcesForContainer(proposal, "app")
	if !ok || app.Requests.Cpu().String() != "250m" {
		t.Fatalf("expected app to select its 250m request, got %#v (selected=%v)", app, ok)
	}
	if _, ok := proposedResourcesForContainer(proposal, "istio-proxy"); ok {
		t.Fatal("expected the sidecar to be excluded when a per-container map is set")
	}

	proposal.Spec.ProposedResourcesByContainer = nil
	legacy, ok := proposedResourcesForContainer(proposal, "only-container")
	if !ok || legacy.Requests.Cpu().String() != "9" {
		t.Fatalf("expected the legacy single resource set to apply, got %#v (selected=%v)", legacy, ok)
	}
}

func TestValidateProposedContainers(t *testing.T) {
	snapshot := &v1alpha1.ExecutionSnapshot{
		ContainerResources: map[string]v1alpha1.ContainerResourceState{
			"app": {}, "istio-proxy": {},
		},
	}
	legacy := &v1alpha1.OptimizationProposal{}
	if err := validateProposedContainers(legacy, snapshot); err == nil {
		t.Fatal("expected a multi-container proposal without a per-container map to fail closed")
	}

	withMap := &v1alpha1.OptimizationProposal{
		Spec: v1alpha1.OptimizationProposalSpec{
			ProposedResourcesByContainer: map[string]corev1.ResourceRequirements{"app": {}},
		},
	}
	if err := validateProposedContainers(withMap, snapshot); err != nil {
		t.Fatalf("expected a per-container proposal to validate, got %v", err)
	}

	unknown := &v1alpha1.OptimizationProposal{
		Spec: v1alpha1.OptimizationProposalSpec{
			ProposedResourcesByContainer: map[string]corev1.ResourceRequirements{"missing": {}},
		},
	}
	if err := validateProposedContainers(unknown, snapshot); err == nil {
		t.Fatal("expected an unknown container name to be rejected")
	}
}

func TestCaptureSnapshotUsesLivePodResources(t *testing.T) {
	dep := snapshotDeployment()
	unrelated := snapshotPod("other-pod", "1", "1Gi")
	unrelated.Labels = map[string]string{"app": "other"}

	c := fake.NewClientBuilder().WithScheme(actuatorTestScheme(t)).WithObjects(
		dep,
		snapshotPod("checkout-a", "250m", "192Mi"),
		snapshotPod("checkout-b", "250m", "192Mi"),
		unrelated,
	).Build()

	snapshot, err := NewActuator(c, c).CaptureSnapshot(context.Background(), actuatorSnapshotProposal())
	if err != nil {
		t.Fatalf("expected snapshot to succeed, got %v", err)
	}
	if snapshot.Generation != 7 || snapshot.ReplicaCount != 2 {
		t.Fatalf("unexpected deployment metadata in snapshot: generation=%d replicas=%d", snapshot.Generation, snapshot.ReplicaCount)
	}
	app := snapshot.ContainerResources["app"].Resources.Requests
	if app.Cpu().String() != "250m" || app.Memory().String() != "192Mi" {
		t.Fatalf("snapshot app requests = %s/%s, want live pod values 250m/192Mi", app.Cpu(), app.Memory())
	}
	proxy := snapshot.ContainerResources["istio-proxy"].Resources.Requests
	if proxy.Cpu().String() != "100m" || proxy.Memory().String() != "64Mi" {
		t.Fatalf("snapshot sidecar requests = %s/%s, want 100m/64Mi", proxy.Cpu(), proxy.Memory())
	}
}

func TestCaptureSnapshotFailsWithoutActivePods(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(actuatorTestScheme(t)).WithObjects(snapshotDeployment()).Build()

	_, err := NewActuator(c, c).CaptureSnapshot(context.Background(), actuatorSnapshotProposal())
	if err == nil || !strings.Contains(err.Error(), "no active pods") {
		t.Fatalf("expected no-active-pods failure, got %v", err)
	}
}

func TestCaptureSnapshotFailsOnDivergentPods(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(actuatorTestScheme(t)).WithObjects(
		snapshotDeployment(),
		snapshotPod("checkout-a", "250m", "192Mi"),
		snapshotPod("checkout-b", "300m", "200Mi"),
	).Build()

	_, err := NewActuator(c, c).CaptureSnapshot(context.Background(), actuatorSnapshotProposal())
	if err == nil || !strings.Contains(err.Error(), "divergent container resources") {
		t.Fatalf("expected divergent-resources failure, got %v", err)
	}
}

func actuatorTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add appsv1 to scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 to scheme: %v", err)
	}
	return scheme
}

func snapshotDeployment() *appsv1.Deployment {
	replicas := int32(2)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "checkout",
			Namespace:   "live-demo",
			Generation:  7,
			Annotations: map[string]string{ExecutionLockAnnotation: "checkout-learning-1"},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "checkout"}},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app", Resources: snapshotResources("500m", "256Mi")},
						{Name: "istio-proxy", Resources: snapshotResources("100m", "64Mi")},
					},
				},
			},
		},
	}
}

func snapshotPod(name, cpu, memory string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "live-demo",
			Labels:    map[string]string{"app": "checkout"},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", Resources: snapshotResources(cpu, memory)},
				{Name: "istio-proxy", Resources: snapshotResources("100m", "64Mi")},
			},
		},
	}
}

func snapshotResources(cpu, memory string) corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(cpu),
			corev1.ResourceMemory: resource.MustParse(memory),
		},
	}
}

func actuatorSnapshotProposal() *v1alpha1.OptimizationProposal {
	return &v1alpha1.OptimizationProposal{
		Spec: v1alpha1.OptimizationProposalSpec{
			TargetWorkload: corev1.ObjectReference{
				Kind:      "Deployment",
				Name:      "checkout",
				Namespace: "live-demo",
			},
		},
	}
}
