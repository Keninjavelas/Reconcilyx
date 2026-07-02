package controller

import (
	"context"
	"fmt"

	"github.com/reconcilyx/operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	if proposal.Status.ExecutionSnapshot == nil || len(proposal.Status.ExecutionSnapshot.ContainerResources) == 0 {
		return fmt.Errorf("no execution snapshot available or it contains no containers")
	}

	// 1. Get the Deployment to find its Pod selector
	dep := &appsv1.Deployment{}
	if err := a.client.Get(ctx, types.NamespacedName{
		Name:      proposal.Spec.TargetWorkload.Name,
		Namespace: proposal.Spec.TargetWorkload.Namespace,
	}, dep); err != nil {
		return fmt.Errorf("failed to get target deployment: %w", err)
	}

	selector, err := metav1.LabelSelectorAsSelector(dep.Spec.Selector)
	if err != nil {
		return fmt.Errorf("failed to parse deployment selector: %w", err)
	}

	// 2. List all running Pods belonging to the Deployment
	podList := &corev1.PodList{}
	if err := a.client.List(ctx, podList, client.InNamespace(dep.Namespace), client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	// 3. Patch each Pod directly in-place (InPlacePodVerticalScaling)
	for _, p := range podList.Items {
		pod := p // copy loop variable
		patch := client.MergeFrom(pod.DeepCopy())
		mutated := false

		// Apply the resources from the proposal to the pod's containers
		for i := range pod.Spec.Containers {
			c := &pod.Spec.Containers[i]
			// Only mutate containers that were tracked in the execution snapshot
			if _, tracked := proposal.Status.ExecutionSnapshot.ContainerResources[c.Name]; tracked {
				if c.Resources.Requests == nil {
					c.Resources.Requests = corev1.ResourceList{}
				}
				if proposal.Spec.Recommendation == v1alpha1.RecommendationReduceCPURequest {
					if cpu, ok := proposal.Spec.ProposedResources.Requests[corev1.ResourceCPU]; ok {
						c.Resources.Requests[corev1.ResourceCPU] = cpu
						mutated = true
					}
				} else if proposal.Spec.Recommendation == v1alpha1.RecommendationReduceMemoryRequest {
					if mem, ok := proposal.Spec.ProposedResources.Requests[corev1.ResourceMemory]; ok {
						c.Resources.Requests[corev1.ResourceMemory] = mem
						mutated = true
					}
				}
			}
		}

		if mutated {
			if err := a.client.Patch(ctx, &pod, patch); err != nil {
				return fmt.Errorf("failed to execute in-place resize on pod %s: %w", pod.Name, err)
			}
		}
	}

	return nil
}
