package controller

import (
	"context"
	"fmt"

	"github.com/reconcilyx/operator/api/v1alpha1"
	"github.com/reconcilyx/operator/internal/telemetry"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RollbackManager ensures a proposal can be safely rolled back if stress is detected.
type RollbackManager interface {
	CheckMonitoringWindow(ctx context.Context, proposal *v1alpha1.OptimizationProposal) (bool, error)
	ExecuteRollback(ctx context.Context, proposal *v1alpha1.OptimizationProposal) error
}

type defaultRollbackManager struct {
	client    client.Client
	telemetry telemetry.TelemetryProvider
}

func NewRollbackManager(c client.Client, provider telemetry.TelemetryProvider) RollbackManager {
	return &defaultRollbackManager{
		client:    c,
		telemetry: provider,
	}
}

func (r *defaultRollbackManager) CheckMonitoringWindow(ctx context.Context, proposal *v1alpha1.OptimizationProposal) (bool, error) {
	if proposal.Status.ExecutionSnapshot == nil {
		return false, fmt.Errorf("no execution snapshot available for rollback check")
	}

	metrics, err := r.telemetry.GetWorkloadMetrics(ctx, proposal.Spec.TargetWorkload)
	if err != nil {
		return false, fmt.Errorf("failed to gather post-resize metrics: %w", err)
	}

	for containerName, cm := range metrics.Containers {
		if _, tracked := proposal.Status.ExecutionSnapshot.ContainerResources[containerName]; tracked {
			if cm.Memory.OOMCount > 0 {
				RollbackTotal.WithLabelValues(proposal.Spec.TargetWorkload.Namespace, proposal.Spec.TargetWorkload.Name, "OOM").Inc()
				return true, fmt.Errorf("OOM detected in container %s", containerName)
			}
			if cm.CPU.LimitedCores > 0 && cm.CPU.ThrottledSecs > (cm.CPU.LimitedCores*0.2) {
				RollbackTotal.WithLabelValues(proposal.Spec.TargetWorkload.Namespace, proposal.Spec.TargetWorkload.Name, "Throttling").Inc()
				return true, fmt.Errorf("severe CPU throttling detected in container %s", containerName)
			}
		}
	}

	return false, nil
}

func (r *defaultRollbackManager) ExecuteRollback(ctx context.Context, proposal *v1alpha1.OptimizationProposal) error {
	if proposal.Status.ExecutionSnapshot == nil || len(proposal.Status.ExecutionSnapshot.ContainerResources) == 0 {
		return fmt.Errorf("no execution snapshot available for rollback")
	}

	dep := &appsv1.Deployment{}
	if err := r.client.Get(ctx, client.ObjectKey{
		Name:      proposal.Spec.TargetWorkload.Name,
		Namespace: proposal.Spec.TargetWorkload.Namespace,
	}, dep); err != nil {
		return fmt.Errorf("failed to get target deployment for rollback: %w", err)
	}

	selector, err := metav1.LabelSelectorAsSelector(dep.Spec.Selector)
	if err != nil {
		return fmt.Errorf("failed to parse deployment selector: %w", err)
	}

	podList := &corev1.PodList{}
	if err := r.client.List(ctx, podList, client.InNamespace(dep.Namespace), client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return fmt.Errorf("failed to list pods for rollback: %w", err)
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
			desired[c.Name] = *snapState.Resources.DeepCopy()
		}

		if err := applyPodResizeSSA(ctx, r.client, &pod, desired); err != nil {
			return fmt.Errorf("rollback failed: %w", err)
		}
	}

	return nil
}
