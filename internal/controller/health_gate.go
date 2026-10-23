package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// WorkloadHealthGate verifies target workload liveness.
type WorkloadHealthGate interface {
	CheckLiveness(ctx context.Context, target corev1.ObjectReference) error
}

type defaultHealthGate struct {
	client client.Client
}

func NewWorkloadHealthGate(c client.Client) WorkloadHealthGate {
	return &defaultHealthGate{client: c}
}

func (h *defaultHealthGate) CheckLiveness(ctx context.Context, target corev1.ObjectReference) error {
	if target.Kind != "Deployment" {
		return fmt.Errorf("unsupported workload kind %q: only Deployments are supported", target.Kind)
	}
	if target.Name == "" || target.Namespace == "" {
		return fmt.Errorf("target workload name and namespace are required")
	}

	dep := &appsv1.Deployment{}
	if err := h.client.Get(ctx, types.NamespacedName{Name: target.Name, Namespace: target.Namespace}, dep); err != nil {
		return fmt.Errorf("failed to get deployment %s/%s: %w", target.Namespace, target.Name, err)
	}

	if !dep.DeletionTimestamp.IsZero() {
		return fmt.Errorf("deployment %s/%s is being deleted", target.Namespace, target.Name)
	}

	desired := int32(1)
	if dep.Spec.Replicas != nil {
		desired = *dep.Spec.Replicas
	}
	if desired == 0 {
		return fmt.Errorf("deployment %s/%s is scaled to zero", target.Namespace, target.Name)
	}

	if dep.Status.ObservedGeneration < dep.Generation {
		return fmt.Errorf("deployment %s/%s has not observed its latest generation (%d < %d)",
			target.Namespace, target.Name, dep.Status.ObservedGeneration, dep.Generation)
	}

	if dep.Status.ReadyReplicas < desired {
		return fmt.Errorf("deployment %s/%s is not fully ready (%d/%d replicas ready)",
			target.Namespace, target.Name, dep.Status.ReadyReplicas, desired)
	}

	for _, cond := range dep.Status.Conditions {
		if cond.Type == appsv1.DeploymentReplicaFailure && cond.Status == corev1.ConditionTrue {
			return fmt.Errorf("deployment %s/%s reports ReplicaFailure: %s", target.Namespace, target.Name, cond.Message)
		}
	}

	return nil
}
