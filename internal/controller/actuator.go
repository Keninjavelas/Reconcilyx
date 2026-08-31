package controller

import (
	"context"
	"fmt"
	"strings"

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
	FieldManagerName        = "reconcilyx-operator"
)

// ResizeFieldOwner is the stable field manager used for all Server-Side Apply
// patches against the pods/resize subresource. Keeping this value fixed
// ensures field ownership conflicts are detected between Reconcilyx and any
// other actor (VPA, HPA, Argo CD, human kubectl apply, etc.).
const ResizeFieldOwner = FieldManagerName

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
	if proposal.Spec.TargetWorkload.Kind != "Deployment" {
		return false, fmt.Errorf("unsupported workload kind: %s", proposal.Spec.TargetWorkload.Kind)
	}

	dep := &appsv1.Deployment{}
	err := a.client.Get(ctx, types.NamespacedName{Name: proposal.Spec.TargetWorkload.Name, Namespace: proposal.Spec.TargetWorkload.Namespace}, dep)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	if dep.Annotations != nil {
		lockVal, exists := dep.Annotations[ExecutionLockAnnotation]
		if exists && lockVal != proposal.Name {
			return false, nil
		}
	} else {
		dep.Annotations = make(map[string]string)
	}

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

// applyPodResizeSSA builds and applies a Server-Side Apply patch against the
// pods/resize subresource. The patch object carries only the containers
// explicitly listed in desiredByContainer, preserving field ownership for
// every other field on the Pod. This helper is shared by the actuation and
// rollback paths.
func applyPodResizeSSA(
	ctx context.Context,
	c client.Client,
	podName, namespace string,
	desiredByContainer map[string]corev1.ResourceRequirements,
) error {
	if len(desiredByContainer) == 0 {
		return nil
	}

	containers := make([]corev1.Container, 0, len(desiredByContainer))
	for name, resources := range desiredByContainer {
		containers = append(containers, corev1.Container{
			Name:      name,
			Resources: resources,
		})
	}

	patch := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			Containers: containers,
		},
	}

	if err := c.SubResource("resize").Patch(ctx, patch,
		client.Apply,
		client.ForceOwnership,
		client.FieldOwner(ResizeFieldOwner),
	); err != nil {
		return wrapResizeError(err, podName, namespace)
	}
	return nil
}

// wrapResizeError enriches errors returned from the /resize subresource with a
// feature-gate hint when they look like the cluster does not support in-place
// pod vertical scaling (InPlacePodVerticalScaling).
func wrapResizeError(err error, podName, namespace string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	featureGateHint := ""
	switch {
	case errors.IsNotFound(err),
		strings.Contains(lower, "the server could not find the requested resource"),
		strings.Contains(lower, "subresource \"resize\""),
		strings.Contains(lower, "not supported"),
		strings.Contains(lower, "resize") && strings.Contains(lower, "unknown"):
		featureGateHint = " (hint: ensure InPlacePodVerticalScaling feature gate is enabled on kube-apiserver and kubelet, and the cluster version supports in-place pod resource updates)"
	}
	return fmt.Errorf("failed to apply in-place resize to pod %s/%s via /resize subresource: %w%s", namespace, podName, err, featureGateHint)
}

func (a *defaultActuator) ApplyMutation(ctx context.Context, proposal *v1alpha1.OptimizationProposal) error {
	switch proposal.Spec.Recommendation {
	case v1alpha1.RecommendationRightSize,
		v1alpha1.RecommendationReduceCPURequest,
		v1alpha1.RecommendationReduceMemoryRequest:
	default:
		return fmt.Errorf("forbidden recommendation type: %s", proposal.Spec.Recommendation)
	}

	if proposal.Status.ExecutionSnapshot == nil || len(proposal.Status.ExecutionSnapshot.ContainerResources) == 0 {
		return fmt.Errorf("no execution snapshot available or it contains no containers")
	}

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

	podList := &corev1.PodList{}
	if err := a.client.List(ctx, podList, client.InNamespace(dep.Namespace), client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	for _, p := range podList.Items {
		pod := p
		desired := make(map[string]corev1.ResourceRequirements)

		for i := range pod.Spec.Containers {
			c := &pod.Spec.Containers[i]
			snapState, tracked := proposal.Status.ExecutionSnapshot.ContainerResources[c.Name]
			if !tracked {
				continue
			}
			requests := corev1.ResourceList{}
			for k, v := range snapState.Resources.Requests {
				requests[k] = v.DeepCopy()
			}
			limits := corev1.ResourceList{}
			for k, v := range snapState.Resources.Limits {
				limits[k] = v.DeepCopy()
			}

			switch proposal.Spec.Recommendation {
			case v1alpha1.RecommendationRightSize:
				if cpu, ok := proposal.Spec.ProposedResources.Requests[corev1.ResourceCPU]; ok {
					requests[corev1.ResourceCPU] = cpu
				}
				if mem, ok := proposal.Spec.ProposedResources.Requests[corev1.ResourceMemory]; ok {
					requests[corev1.ResourceMemory] = mem
				}
			case v1alpha1.RecommendationReduceCPURequest:
				if cpu, ok := proposal.Spec.ProposedResources.Requests[corev1.ResourceCPU]; ok {
					requests[corev1.ResourceCPU] = cpu
				}
			case v1alpha1.RecommendationReduceMemoryRequest:
				if mem, ok := proposal.Spec.ProposedResources.Requests[corev1.ResourceMemory]; ok {
					requests[corev1.ResourceMemory] = mem
				}
			}

			desired[c.Name] = corev1.ResourceRequirements{
				Requests: requests,
				Limits:   limits,
			}
		}

		if err := applyPodResizeSSA(ctx, a.client, pod.Name, pod.Namespace, desired); err != nil {
			return err
		}
	}

	return nil
}
