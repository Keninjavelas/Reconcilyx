package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

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
