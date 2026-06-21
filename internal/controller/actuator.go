package controller

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/reconcilyx/operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ExecutionLockAnnotation = "reconcilyx.io/execution-lock"
	LastActionAnnotation    = "reconcilyx.io/last-action"
	FieldManagerName        = "reconcilyx-optimizer"
)

// Actuator executes safe mutations on workloads.
type Actuator interface {
	AcquireLock(ctx context.Context, proposal *v1alpha1.OptimizationProposal) (bool, error)
	ReleaseLock(ctx context.Context, proposal *v1alpha1.OptimizationProposal) error
	CaptureSnapshot(ctx context.Context, proposal *v1alpha1.OptimizationProposal) (*v1alpha1.ExecutionSnapshot, error)
	ApplyMutation(ctx context.Context, proposal *v1alpha1.OptimizationProposal) error
}

type defaultActuator struct {
	client client.Client
}

func NewActuator(client client.Client) Actuator {
	return &defaultActuator{client: client}
}

func (a *defaultActuator) AcquireLock(ctx context.Context, proposal *v1alpha1.OptimizationProposal) (bool, error) {
	// Only supports Deployments for now
	if proposal.Spec.TargetWorkload.Kind != "Deployment" {
		return false, fmt.Errorf("unsupported workload kind: %s", proposal.Spec.TargetWorkload.Kind)
	}

	dep := &appsv1.Deployment{}
	err := a.client.Get(ctx, types.NamespacedName{Name: proposal.Spec.TargetWorkload.Name, Namespace: proposal.Spec.TargetWorkload.Namespace}, dep)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil // Target deleted
		}
		return false, err
	}

	// Check if lock exists
	if dep.Annotations != nil {
		lockVal, exists := dep.Annotations[ExecutionLockAnnotation]
		if exists && lockVal != proposal.Name {
			// Another proposal holds the lock
			return false, nil
		}
	} else {
		dep.Annotations = make(map[string]string)
	}

	// Acquire lock
	dep.Annotations[ExecutionLockAnnotation] = proposal.Name
	if err := a.client.Update(ctx, dep); err != nil {
		return false, err
	}

	return true, nil
}

func (a *defaultActuator) ReleaseLock(ctx context.Context, proposal *v1alpha1.OptimizationProposal) error {
	if proposal.Spec.TargetWorkload.Kind != "Deployment" {
		return nil
	}

	dep := &appsv1.Deployment{}
	err := a.client.Get(ctx, types.NamespacedName{Name: proposal.Spec.TargetWorkload.Name, Namespace: proposal.Spec.TargetWorkload.Namespace}, dep)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if dep.Annotations != nil {
		lockVal, exists := dep.Annotations[ExecutionLockAnnotation]
		if exists && lockVal == proposal.Name {
			delete(dep.Annotations, ExecutionLockAnnotation)
			return a.client.Update(ctx, dep)
		}
	}
	return nil
}

func (a *defaultActuator) CaptureSnapshot(ctx context.Context, proposal *v1alpha1.OptimizationProposal) (*v1alpha1.ExecutionSnapshot, error) {
	if proposal.Spec.TargetWorkload.Kind != "Deployment" {
		return nil, fmt.Errorf("unsupported workload kind")
	}

	dep := &appsv1.Deployment{}
	err := a.client.Get(ctx, types.NamespacedName{Name: proposal.Spec.TargetWorkload.Name, Namespace: proposal.Spec.TargetWorkload.Namespace}, dep)
	if err != nil {
		return nil, err
	}

	snapshot := &v1alpha1.ExecutionSnapshot{
		ResourceVersion:    dep.ResourceVersion,
		Generation:         dep.Generation,
		ReplicaCount:       *dep.Spec.Replicas,
		Annotations:        make(map[string]string),
		ContainerResources: make(map[string]v1alpha1.ContainerResourceState),
		Timestamp:          metav1.Now(),
	}

	for k, v := range dep.Annotations {
		snapshot.Annotations[k] = v
	}

	for _, c := range dep.Spec.Template.Spec.Containers {
		snapshot.ContainerResources[c.Name] = v1alpha1.ContainerResourceState{
			Resources: *c.Resources.DeepCopy(),
		}
	}

	return snapshot, nil
}

func (a *defaultActuator) ApplyMutation(ctx context.Context, proposal *v1alpha1.OptimizationProposal) error {
	if proposal.Spec.Recommendation != v1alpha1.RecommendationReduceCPURequest &&
		proposal.Spec.Recommendation != v1alpha1.RecommendationReduceMemoryRequest {
		return fmt.Errorf("forbidden recommendation type: %s", proposal.Spec.Recommendation)
	}

	// We use unstructured to ensure we only send EXACTLY the fields we want to own.
	patch := &unstructured.Unstructured{}
	patch.SetAPIVersion("apps/v1")
	patch.SetKind("Deployment")
	patch.SetName(proposal.Spec.TargetWorkload.Name)
	patch.SetNamespace(proposal.Spec.TargetWorkload.Namespace)

	// Annotations
	annotations := map[string]string{
		LastActionAnnotation: "execute",
	}
	patch.SetAnnotations(annotations)

	// Build the minimal pod template spec for the containers we are mutating
	var containers []interface{}

	if proposal.Status.ExecutionSnapshot == nil || len(proposal.Status.ExecutionSnapshot.ContainerResources) == 0 {
		return fmt.Errorf("no execution snapshot available or it contains no containers")
	}

	requests := map[string]interface{}{}
	if proposal.Spec.Recommendation == v1alpha1.RecommendationReduceCPURequest {
		if cpu, ok := proposal.Spec.ProposedResources.Requests[corev1.ResourceCPU]; ok {
			requests["cpu"] = cpu.String()
		}
	} else if proposal.Spec.Recommendation == v1alpha1.RecommendationReduceMemoryRequest {
		if mem, ok := proposal.Spec.ProposedResources.Requests[corev1.ResourceMemory]; ok {
			requests["memory"] = mem.String()
		}
	}

	if len(requests) == 0 {
		return fmt.Errorf("no requests formulated for SSA payload")
	}

	// Apply the patch dynamically to all containers tracked in the snapshot
	for containerName := range proposal.Status.ExecutionSnapshot.ContainerResources {
		container := map[string]interface{}{
			"name": containerName,
			"resources": map[string]interface{}{
				"requests": requests,
			},
		}
		containers = append(containers, container)
	}

	templateSpec := map[string]interface{}{
		"containers": containers,
	}
	
	template := map[string]interface{}{
		"spec": templateSpec,
	}

	spec := map[string]interface{}{
		"template": template,
	}

	err := unstructured.SetNestedMap(patch.Object, spec, "spec")
	if err != nil {
		return err
	}

	// Apply using Server-Side Apply
	// Force=false guarantees we don't overwrite manual human edits
	patchOpts := &client.PatchOptions{
		FieldManager: FieldManagerName,
		Force:        func() *bool { b := true; return &b }(),
	}

	// Marshal unstructured object to JSON to apply
	data, err := json.Marshal(patch.Object)
	if err != nil {
		return err
	}

	return a.client.Patch(ctx, patch, client.RawPatch(types.ApplyPatchType, data), patchOpts)
}
