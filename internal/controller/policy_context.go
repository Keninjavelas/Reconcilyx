package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	optimizationv1alpha1 "github.com/reconcilyx/operator/api/v1alpha1"
)

// workloadNamespace resolves the namespace that holds the target workload.
// ObjectReference allows an empty namespace, in which case the proposal's own
// namespace is the implicit target.
func workloadNamespace(proposal *optimizationv1alpha1.OptimizationProposal) string {
	if proposal == nil {
		return ""
	}
	if ns := proposal.Spec.TargetWorkload.Namespace; ns != "" {
		return ns
	}
	return proposal.Namespace
}

func sameWorkloadTarget(a, b *optimizationv1alpha1.OptimizationProposal) bool {
	return a.Spec.TargetWorkload.Kind == b.Spec.TargetWorkload.Kind &&
		a.Spec.TargetWorkload.Name == b.Spec.TargetWorkload.Name &&
		workloadNamespace(a) == workloadNamespace(b)
}

// lastExecutionForWorkload returns the most recent recorded actuation for the
// same workload, ignoring the proposal being reconciled.
func lastExecutionForWorkload(ctx context.Context, c client.Client, proposal *optimizationv1alpha1.OptimizationProposal) (*metav1.Time, error) {
	if c == nil || proposal == nil {
		return nil, nil
	}
	var proposals optimizationv1alpha1.OptimizationProposalList
	if err := c.List(ctx, &proposals); err != nil {
		return nil, fmt.Errorf("list proposals for cooldown: %w", err)
	}
	var latest *metav1.Time
	for i := range proposals.Items {
		other := &proposals.Items[i]
		if other.Name == proposal.Name && other.Namespace == proposal.Namespace {
			continue
		}
		if !sameWorkloadTarget(other, proposal) || other.Status.ExecutionSnapshot == nil {
			continue
		}
		at := other.Status.ExecutionSnapshot.Timestamp
		if at.IsZero() {
			continue
		}
		if latest == nil || at.After(latest.Time) {
			latest = &metav1.Time{Time: at.Time}
		}
	}
	return latest, nil
}

// workloadContainerNames lists the containers the actuator may resize. A
// missing or unsupported workload yields no names rather than an error, because
// the health gate is responsible for refusing a dead target before execution.
func workloadContainerNames(ctx context.Context, c client.Client, proposal *optimizationv1alpha1.OptimizationProposal) ([]string, error) {
	if c == nil || proposal == nil {
		return nil, nil
	}
	key := types.NamespacedName{
		Namespace: workloadNamespace(proposal),
		Name:      proposal.Spec.TargetWorkload.Name,
	}
	var podSpec *corev1.PodSpec
	switch proposal.Spec.TargetWorkload.Kind {
	case "Deployment":
		dep := &appsv1.Deployment{}
		if err := c.Get(ctx, key, dep); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, nil
			}
			return nil, fmt.Errorf("get Deployment %s: %w", key, err)
		}
		podSpec = &dep.Spec.Template.Spec
	case "StatefulSet":
		statefulSet := &appsv1.StatefulSet{}
		if err := c.Get(ctx, key, statefulSet); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, nil
			}
			return nil, fmt.Errorf("get StatefulSet %s: %w", key, err)
		}
		podSpec = &statefulSet.Spec.Template.Spec
	case "DaemonSet":
		daemonSet := &appsv1.DaemonSet{}
		if err := c.Get(ctx, key, daemonSet); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, nil
			}
			return nil, fmt.Errorf("get DaemonSet %s: %w", key, err)
		}
		podSpec = &daemonSet.Spec.Template.Spec
	default:
		return nil, nil
	}

	names := make([]string, 0, len(podSpec.Containers))
	for _, container := range podSpec.Containers {
		names = append(names, container.Name)
	}
	return names, nil
}
