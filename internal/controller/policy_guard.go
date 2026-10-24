package controller

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	optimizationv1alpha1 "github.com/reconcilyx/operator/api/v1alpha1"
)

const defaultChangeBudgetWindow = 24 * time.Hour

// PolicyGuard enforces execution-time policy constraints that depend on live
// cluster state: change budgets and blast-radius limits.
type PolicyGuard interface {
	Check(ctx context.Context, proposal *optimizationv1alpha1.OptimizationProposal, policy *optimizationv1alpha1.OptimizationPolicy) error
}

type defaultPolicyGuard struct {
	client client.Client
}

func NewPolicyGuard(c client.Client) PolicyGuard {
	return &defaultPolicyGuard{client: c}
}

func (g *defaultPolicyGuard) Check(ctx context.Context, proposal *optimizationv1alpha1.OptimizationProposal, policy *optimizationv1alpha1.OptimizationPolicy) error {
	if policy == nil {
		return nil
	}
	if g == nil || g.client == nil {
		if policyHasGuardrails(policy) {
			return fmt.Errorf("policy guard is not configured")
		}
		return nil
	}
	if err := g.checkCooldown(ctx, proposal, policy); err != nil {
		return err
	}
	if err := g.checkChangeBudget(ctx, proposal, policy); err != nil {
		return err
	}
	return g.checkBlastRadius(ctx, proposal, policy)
}

func policyHasGuardrails(policy *optimizationv1alpha1.OptimizationPolicy) bool {
	return policy.Spec.SafetyConstraints.CooldownPeriod != nil ||
		policy.Spec.ChangeBudget.MaxChangesPerWindow > 0 ||
		policy.Spec.BlastRadius.MaxReplicasPerChange > 0 ||
		policy.Spec.BlastRadius.RequireCanaryLabel
}

func (g *defaultPolicyGuard) checkCooldown(ctx context.Context, proposal *optimizationv1alpha1.OptimizationProposal, policy *optimizationv1alpha1.OptimizationPolicy) error {
	cooldown := policy.Spec.SafetyConstraints.CooldownPeriod
	if cooldown == nil || cooldown.Duration <= 0 {
		return nil
	}
	last, err := lastExecutionForWorkload(ctx, g.client, proposal)
	if err != nil {
		return fmt.Errorf("cooldown: %w", err)
	}
	if last == nil {
		return nil
	}
	elapsed := time.Since(last.Time)
	if elapsed < cooldown.Duration {
		return fmt.Errorf("cooldown active: %s since the last change to %s, requires %s",
			elapsed.Round(time.Second), probeWorkloadName(proposal), cooldown.Duration)
	}
	return nil
}

func (g *defaultPolicyGuard) checkChangeBudget(ctx context.Context, proposal *optimizationv1alpha1.OptimizationProposal, policy *optimizationv1alpha1.OptimizationPolicy) error {
	maxChanges := policy.Spec.ChangeBudget.MaxChangesPerWindow
	if maxChanges <= 0 {
		return nil
	}
	window := defaultChangeBudgetWindow
	if configured := policy.Spec.ChangeBudget.Window; configured != nil {
		if configured.Duration <= 0 {
			return fmt.Errorf("change budget window must be positive")
		}
		window = configured.Duration
	}

	var proposals optimizationv1alpha1.OptimizationProposalList
	if err := g.client.List(ctx, &proposals); err != nil {
		return fmt.Errorf("change budget: list proposals: %w", err)
	}
	cutoff := time.Now().Add(-window)
	recent := int32(0)
	for i := range proposals.Items {
		other := &proposals.Items[i]
		if other.Name == proposal.Name && other.Namespace == proposal.Namespace {
			continue
		}
		if !sameWorkloadTarget(other, proposal) {
			continue
		}
		if other.Status.ExecutionSnapshot == nil {
			continue
		}
		at := other.Status.ExecutionSnapshot.Timestamp.Time
		if at.IsZero() || at.Before(cutoff) {
			continue
		}
		recent++
	}
	if recent >= maxChanges {
		return fmt.Errorf("change budget exhausted: %d change(s) to %s/%s in the last %s (max %d)",
			recent, proposal.Spec.TargetWorkload.Kind, probeWorkloadName(proposal), window, maxChanges)
	}
	return nil
}

func probeWorkloadName(proposal *optimizationv1alpha1.OptimizationProposal) string {
	if ns := workloadNamespace(proposal); ns != "" {
		return ns + "/" + proposal.Spec.TargetWorkload.Name
	}
	return proposal.Spec.TargetWorkload.Name
}

func (g *defaultPolicyGuard) checkBlastRadius(ctx context.Context, proposal *optimizationv1alpha1.OptimizationProposal, policy *optimizationv1alpha1.OptimizationPolicy) error {
	br := policy.Spec.BlastRadius
	if br.MaxReplicasPerChange <= 0 && !br.RequireCanaryLabel {
		return nil
	}

	replicas, canary, replicasSupported, err := g.workloadState(ctx, proposal.Spec.TargetWorkload.Kind, proposal.Spec.TargetWorkload.Namespace, proposal.Spec.TargetWorkload.Name)
	if err != nil {
		return fmt.Errorf("blast radius check failed: %w", err)
	}
	workload := fmt.Sprintf("%s/%s", proposal.Spec.TargetWorkload.Kind, probeWorkloadName(proposal))

	if br.MaxReplicasPerChange > 0 {
		if !replicasSupported {
			return fmt.Errorf("blast radius check failed: maxReplicasPerChange is not supported for %s", workload)
		}
		if replicas > br.MaxReplicasPerChange {
			return fmt.Errorf("blast radius exceeded: %s has %d replicas (policy max %d)", workload, replicas, br.MaxReplicasPerChange)
		}
	}
	if br.RequireCanaryLabel && !canary {
		return fmt.Errorf("blast radius canary label missing: %s or its pod template must carry %s=true", workload, optimizationv1alpha1.CanaryLabelKey)
	}
	return nil
}

func (g *defaultPolicyGuard) workloadState(ctx context.Context, kind, namespace, name string) (int32, bool, bool, error) {
	key := types.NamespacedName{Namespace: namespace, Name: name}
	switch kind {
	case "Deployment":
		deployment := &appsv1.Deployment{}
		if err := g.client.Get(ctx, key, deployment); err != nil {
			return 0, false, true, fmt.Errorf("get Deployment %s/%s: %w", namespace, name, err)
		}
		replicas := int32(1)
		if deployment.Spec.Replicas != nil {
			replicas = *deployment.Spec.Replicas
		}
		return replicas, hasCanaryLabel(deployment.Labels, deployment.Spec.Template.Labels), true, nil
	case "StatefulSet":
		statefulSet := &appsv1.StatefulSet{}
		if err := g.client.Get(ctx, key, statefulSet); err != nil {
			return 0, false, true, fmt.Errorf("get StatefulSet %s/%s: %w", namespace, name, err)
		}
		replicas := int32(1)
		if statefulSet.Spec.Replicas != nil {
			replicas = *statefulSet.Spec.Replicas
		}
		return replicas, hasCanaryLabel(statefulSet.Labels, statefulSet.Spec.Template.Labels), true, nil
	case "DaemonSet":
		daemonSet := &appsv1.DaemonSet{}
		if err := g.client.Get(ctx, key, daemonSet); err != nil {
			return 0, false, false, fmt.Errorf("get DaemonSet %s/%s: %w", namespace, name, err)
		}
		return 0, hasCanaryLabel(daemonSet.Labels, daemonSet.Spec.Template.Labels), false, nil
	default:
		return 0, false, false, fmt.Errorf("unsupported workload kind %q", kind)
	}
}

func hasCanaryLabel(workloadLabels, podLabels map[string]string) bool {
	if workloadLabels[optimizationv1alpha1.CanaryLabelKey] == "true" {
		return true
	}
	return podLabels[optimizationv1alpha1.CanaryLabelKey] == "true"
}
