package controller

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/reconcilyx/operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
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

// isActuatableRecommendation reports whether the in-place resize actuator can
// execute the recommendation. Keep this as the single source of truth shared by
// the JIT execution gate and the actuator itself.
func isActuatableRecommendation(rec v1alpha1.RecommendationType) bool {
	switch rec {
	case v1alpha1.RecommendationRightSize,
		v1alpha1.RecommendationReduceCPURequest,
		v1alpha1.RecommendationReduceMemoryRequest:
		return true
	default:
		return false
	}
}

// Actuator executes safe mutations on workloads.
type Actuator interface {
	AcquireLock(ctx context.Context, proposal *v1alpha1.OptimizationProposal) (bool, error)
	ReleaseLock(ctx context.Context, proposal *v1alpha1.OptimizationProposal) error
	CaptureSnapshot(ctx context.Context, proposal *v1alpha1.OptimizationProposal) (*v1alpha1.ExecutionSnapshot, error)
	ApplyMutation(ctx context.Context, proposal *v1alpha1.OptimizationProposal) error
}

type defaultActuator struct {
	client client.Client
	reader client.Reader
}

func NewActuator(c client.Client, reader client.Reader) Actuator {
	if reader == nil {
		reader = c
	}
	return &defaultActuator{client: c, reader: reader}
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
	// Read through the uncached reader: AcquireLock writes the lock annotation
	// just before this call, and a cache-backed read can return the pre-lock
	// generation, which the Verifying phase would misread as external drift.
	err := a.reader.Get(ctx, types.NamespacedName{Name: proposal.Spec.TargetWorkload.Name, Namespace: proposal.Spec.TargetWorkload.Namespace}, dep)
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

	selector, err := metav1.LabelSelectorAsSelector(dep.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("failed to parse deployment selector: %w", err)
	}

	podList := &corev1.PodList{}
	if err := a.reader.List(ctx, podList, client.InNamespace(dep.Namespace), client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return nil, fmt.Errorf("failed to list pods for snapshot: %w", err)
	}

	activePods := make([]corev1.Pod, 0, len(podList.Items))
	for _, pod := range podList.Items {
		if pod.DeletionTimestamp != nil {
			continue
		}
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		activePods = append(activePods, pod)
	}
	if len(activePods) == 0 {
		return nil, fmt.Errorf("no active pods matched deployment %s/%s", dep.Namespace, dep.Name)
	}
	sort.Slice(activePods, func(i, j int) bool { return activePods[i].Name < activePods[j].Name })

	reference := activePods[0]
	for _, pod := range activePods[1:] {
		if !sameContainerResources(reference, pod) {
			return nil, fmt.Errorf("pods %s and %s have divergent container resources; refusing to capture an ambiguous rollback snapshot", reference.Name, pod.Name)
		}
	}

	for i := range reference.Spec.Containers {
		c := &reference.Spec.Containers[i]
		snapshot.ContainerResources[c.Name] = v1alpha1.ContainerResourceState{
			Resources: *c.Resources.DeepCopy(),
		}
	}

	return snapshot, nil
}

// sameContainerResources reports whether two pods carry an identical container
// layout and resource configuration. A snapshot is a single rollback target, so
// pods with divergent resources cannot be captured safely.
func sameContainerResources(a, b corev1.Pod) bool {
	if len(a.Spec.Containers) != len(b.Spec.Containers) {
		return false
	}
	for i := range a.Spec.Containers {
		if a.Spec.Containers[i].Name != b.Spec.Containers[i].Name {
			return false
		}
		if !equality.Semantic.DeepEqual(a.Spec.Containers[i].Resources, b.Spec.Containers[i].Resources) {
			return false
		}
	}
	return true
}

// resizeContainers projects desiredByContainer onto the pod's existing
// container order. The pods/resize subresource rejects patches that rename or
// reorder containers, so iterating a map here would randomly fail.
func resizeContainers(pod *corev1.Pod, desiredByContainer map[string]corev1.ResourceRequirements) []corev1.Container {
	containers := make([]corev1.Container, 0, len(desiredByContainer))
	for i := range pod.Spec.Containers {
		c := pod.Spec.Containers[i]
		resources, ok := desiredByContainer[c.Name]
		if !ok {
			continue
		}
		containers = append(containers, corev1.Container{
			Name:      c.Name,
			Resources: resources,
		})
	}
	return containers
}

// applyPodResizeSSA builds and applies a Server-Side Apply patch against the
// pods/resize subresource. The patch object lists every container in the pod
// so an apply does not delete fields this manager owns from earlier resizes;
// desiredByContainer supplies the resources to assert, with unchanged
// containers kept at their live values. This helper is shared by the actuation
// and rollback paths.
func applyPodResizeSSA(
	ctx context.Context,
	c client.Client,
	pod *corev1.Pod,
	desiredByContainer map[string]corev1.ResourceRequirements,
) error {
	if len(desiredByContainer) == 0 {
		return nil
	}

	patch := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
		Spec: corev1.PodSpec{
			Containers: resizeContainers(pod, desiredByContainer),
		},
	}

	if err := c.SubResource("resize").Patch(ctx, patch,
		client.Apply,
		client.ForceOwnership,
		client.FieldOwner(ResizeFieldOwner),
	); err != nil {
		return wrapResizeError(err, pod.Name, pod.Namespace)
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
	if !isActuatableRecommendation(proposal.Spec.Recommendation) {
		return fmt.Errorf("unsupported recommendation type: %s", proposal.Spec.Recommendation)
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

	snapshot := proposal.Status.ExecutionSnapshot
	if err := validateProposedContainers(proposal, snapshot); err != nil {
		return err
	}

	podList := &corev1.PodList{}
	if err := a.client.List(ctx, podList, client.InNamespace(dep.Namespace), client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	resizedPods := 0
	for _, p := range podList.Items {
		pod := p
		desired := desiredResizeResources(&pod, proposal, snapshot)
		if len(desired) == 0 {
			continue
		}
		if err := applyPodResizeSSA(ctx, a.client, &pod, desired); err != nil {
			return err
		}
		resizedPods++
	}

	if resizedPods == 0 {
		return fmt.Errorf("no running pods matched the proposal's target containers")
	}
	return nil
}

// validateProposedContainers fails closed when a proposal cannot be mapped to
// the workload's containers without guessing.
func validateProposedContainers(proposal *v1alpha1.OptimizationProposal, snapshot *v1alpha1.ExecutionSnapshot) error {
	if len(proposal.Spec.ProposedResourcesByContainer) > 0 {
		for name := range proposal.Spec.ProposedResourcesByContainer {
			if _, ok := snapshot.ContainerResources[name]; !ok {
				return fmt.Errorf("proposedResourcesByContainer references container %q, which is not in the execution snapshot", name)
			}
		}
		return nil
	}
	if len(snapshot.ContainerResources) > 1 {
		return fmt.Errorf("proposal has no proposedResourcesByContainer and the workload has %d containers; refusing to apply one request set to every container", len(snapshot.ContainerResources))
	}
	return nil
}

// proposedResourcesForContainer returns the resources proposed for a specific
// container. A per-container map wins; otherwise the legacy single resource
// set applies only to single-container workloads.
func proposedResourcesForContainer(proposal *v1alpha1.OptimizationProposal, container string) (corev1.ResourceRequirements, bool) {
	if len(proposal.Spec.ProposedResourcesByContainer) > 0 {
		resources, ok := proposal.Spec.ProposedResourcesByContainer[container]
		return resources, ok
	}
	return proposal.Spec.ProposedResources, true
}

// desiredResizeResources returns the resources to assert for every container
// in the pod. Containers outside the proposal keep their live values so the
// server-side apply remains a complete description of the container list.
func desiredResizeResources(
	pod *corev1.Pod,
	proposal *v1alpha1.OptimizationProposal,
	snapshot *v1alpha1.ExecutionSnapshot,
) map[string]corev1.ResourceRequirements {
	desired := make(map[string]corev1.ResourceRequirements, len(pod.Spec.Containers))
	for i := range pod.Spec.Containers {
		c := &pod.Spec.Containers[i]
		resources := *c.Resources.DeepCopy()

		proposed, selected := proposedResourcesForContainer(proposal, c.Name)
		if _, tracked := snapshot.ContainerResources[c.Name]; !selected || !tracked {
			desired[c.Name] = resources
			continue
		}

		if resources.Requests == nil {
			resources.Requests = corev1.ResourceList{}
		}
		switch proposal.Spec.Recommendation {
		case v1alpha1.RecommendationRightSize:
			if cpu, ok := proposed.Requests[corev1.ResourceCPU]; ok {
				resources.Requests[corev1.ResourceCPU] = cpu
			}
			if mem, ok := proposed.Requests[corev1.ResourceMemory]; ok {
				resources.Requests[corev1.ResourceMemory] = mem
			}
		case v1alpha1.RecommendationReduceCPURequest:
			if cpu, ok := proposed.Requests[corev1.ResourceCPU]; ok {
				resources.Requests[corev1.ResourceCPU] = cpu
			}
		case v1alpha1.RecommendationReduceMemoryRequest:
			if mem, ok := proposed.Requests[corev1.ResourceMemory]; ok {
				resources.Requests[corev1.ResourceMemory] = mem
			}
		}
		desired[c.Name] = resources
	}
	return desired
}
