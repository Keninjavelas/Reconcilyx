package controller

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	optimizationv1alpha1 "github.com/reconcilyx/operator/api/v1alpha1"
	"github.com/reconcilyx/operator/internal/carbon"
	"github.com/reconcilyx/operator/internal/engine"
	"github.com/reconcilyx/operator/internal/engine/forecasting"
	"github.com/reconcilyx/operator/internal/engine/rules"
	"github.com/reconcilyx/operator/internal/telemetry"
	corev1 "k8s.io/api/core/v1"
)

type OptimizationPolicyReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Recorder  record.EventRecorder
	Analyzer  engine.Analyzer
	BatchSize int
}

// +kubebuilder:rbac:groups=optimization.reconcilyx.io,resources=optimizationpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=optimization.reconcilyx.io,resources=optimizationpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=optimization.reconcilyx.io,resources=optimizationpolicies/finalizers,verbs=update
// +kubebuilder:rbac:groups=optimization.reconcilyx.io,resources=optimizationproposals,verbs=get;list;watch;create
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch

func (r *OptimizationPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	policy := &optimizationv1alpha1.OptimizationPolicy{}
	err := r.Get(ctx, req.NamespacedName, policy)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get OptimizationPolicy")
		return ctrl.Result{}, err
	}

	if len(policy.Status.Conditions) == 0 {
		condition := metav1.Condition{
			Type:               "Validated",
			Status:             metav1.ConditionTrue,
			Reason:             "PolicyValid",
			Message:            "Policy syntax and constraints validated",
			LastTransitionTime: metav1.Time{Time: time.Now()},
		}
		policy.Status.Conditions = []metav1.Condition{condition}
		if err := r.Status().Update(ctx, policy); err != nil {
			logger.Error(err, "Failed to update OptimizationPolicy status")
			return ctrl.Result{}, err
		}

		r.Recorder.Event(policy, "Normal", "PolicyValidated", "Policy syntax and constraints validated")
		logger.Info("Policy validated", "Policy", policy.Name)
	}

	deps, err := r.findMatchingDeployments(ctx, policy)
	if err != nil {
		logger.Error(err, "Failed to find matching deployments")
		return ctrl.Result{RequeueAfter: 2 * time.Minute}, nil
	}

	matched := int32(len(deps))
	PolicyWorkloadsMatched.WithLabelValues(policy.Namespace, policy.Name).Set(float64(matched))

	if policy.Spec.Mode == optimizationv1alpha1.ModeObserve {
		logger.V(1).Info("Policy in Observe mode, skipping proposal creation", "matched", matched)
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	batchSize := r.BatchSize
	if batchSize <= 0 {
		batchSize = 10
	}
	processed := 0
	for i := range deps {
		if processed >= batchSize {
			break
		}
		dep := deps[i]

		hasRecent, recentErr := r.hasRecentProposalFor(ctx, &dep)
		if recentErr != nil {
			logger.V(1).Info("Could not check recent proposals", "deployment", dep.Name, "err", recentErr)
			continue
		}
		if hasRecent {
			continue
		}

		currentResources := extractCurrentResources(&dep)
		ref := corev1.ObjectReference{
			Kind:       "Deployment",
			Namespace:  dep.Namespace,
			Name:       dep.Name,
			UID:        dep.UID,
			APIVersion: "apps/v1",
		}

		proposal, analyzeErr := r.Analyzer.Analyze(ctx, ref, currentResources)
		if analyzeErr != nil {
			logger.V(1).Info("Analyzer failed", "deployment", dep.Name, "err", analyzeErr)
			continue
		}
		if proposal == nil {
			continue
		}

		confidence := proposal.Spec.DecisionMetadata.ConfidenceScore
		risk := proposal.Spec.DecisionMetadata.RiskScore
		minConf := int32(0)
		maxRisk := int32(100)
		if policy.Spec.SafetyConstraints.ConfidenceRequirement != nil {
			minConf = *policy.Spec.SafetyConstraints.ConfidenceRequirement
		}
		if policy.Spec.SafetyConstraints.RiskTolerance != nil {
			maxRisk = *policy.Spec.SafetyConstraints.RiskTolerance
		}
		if confidence < minConf || risk > maxRisk {
			r.Recorder.Eventf(policy, "Normal", "ProposalSkipped",
				"Proposal for %s/%s skipped (confidence=%d < %d or risk=%d > %d)",
				dep.Namespace, dep.Name, confidence, minConf, risk, maxRisk)
			continue
		}

		proposal.Namespace = dep.Namespace
		proposal.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: optimizationv1alpha1.GroupVersion.String(),
				Kind:       "OptimizationPolicy",
				Name:       policy.Name,
				UID:        policy.UID,
			},
		}
		ttl := 1 * time.Hour
		if policy.Spec.SafetyConstraints.CooldownPeriod != nil {
			ttl = policy.Spec.SafetyConstraints.CooldownPeriod.Duration
		}
		expiresAt := metav1.NewTime(time.Now().Add(ttl))
		proposal.Status.ExpirationTime = &expiresAt
		proposal.Status.Phase = optimizationv1alpha1.PhaseDetected
		proposal.Status.ApprovalState = optimizationv1alpha1.ApprovalPending

		if createErr := r.Create(ctx, proposal); createErr != nil {
			if apierrors.IsAlreadyExists(createErr) {
				continue
			}
			logger.Error(createErr, "Failed to create OptimizationProposal", "deployment", dep.Name)
			continue
		}

		telemetry.ProposalCreatedTotal.WithLabelValues(policy.Namespace, policy.Name, dep.Name).Inc()
		r.Recorder.Eventf(policy, "Normal", "ProposalCreated",
			"Created OptimizationProposal %s for %s/%s (confidence=%d, risk=%d)",
			proposal.Name, dep.Namespace, dep.Name, confidence, risk)
		processed++
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *OptimizationPolicyReconciler) findMatchingDeployments(ctx context.Context, policy *optimizationv1alpha1.OptimizationPolicy) ([]appsv1.Deployment, error) {
	var allDeps appsv1.DeploymentList
	if err := r.List(ctx, &allDeps, client.InNamespace(policy.Namespace)); err != nil {
		return nil, err
	}

	var matched []appsv1.Deployment
	workloadSel := policy.Spec.TargetSelectors.WorkloadSelector
	for i := range allDeps.Items {
		dep := allDeps.Items[i]
		if workloadSel != nil {
			sel, err := metav1.LabelSelectorAsSelector(workloadSel)
			if err != nil {
				continue
			}
			if !sel.Matches(labels.Set(dep.Labels)) {
				continue
			}
		}
		matched = append(matched, dep)
	}
	return matched, nil
}

func (r *OptimizationPolicyReconciler) hasRecentProposalFor(ctx context.Context, dep *appsv1.Deployment) (bool, error) {
	var existing optimizationv1alpha1.OptimizationProposalList
	if err := r.List(ctx, &existing, client.InNamespace(dep.Namespace)); err != nil {
		return false, err
	}
	for i := range existing.Items {
		p := existing.Items[i]
		if p.Spec.TargetWorkload.Name != dep.Name || p.Spec.TargetWorkload.Kind != "Deployment" {
			continue
		}
		switch p.Status.Phase {
		case optimizationv1alpha1.PhaseDetected,
			optimizationv1alpha1.PhaseEvaluating,
			optimizationv1alpha1.PhaseApproved,
			optimizationv1alpha1.PhaseScheduled,
			optimizationv1alpha1.PhaseExecuting,
			optimizationv1alpha1.PhaseVerifying,
			optimizationv1alpha1.PhaseStabilizing,
			optimizationv1alpha1.PhaseAwaitingExternalApproval:
			return true, nil
		}
		if p.Status.ExpirationTime != nil && time.Now().Before(p.Status.ExpirationTime.Time) {
			return true, nil
		}
	}
	return false, nil
}

func extractCurrentResources(dep *appsv1.Deployment) map[string]corev1.ResourceRequirements {
	out := make(map[string]corev1.ResourceRequirements)
	for _, c := range dep.Spec.Template.Spec.Containers {
		out[c.Name] = *c.Resources.DeepCopy()
	}
	return out
}

func BuildDefaultAnalyzer(promURL string, carbonProv carbon.Provider) (engine.Analyzer, error) {
	promClient, err := telemetry.NewPrometheusClient(promURL)
	if err != nil {
		return nil, fmt.Errorf("prometheus client: %w", err)
	}
	retriable := telemetry.NewRetriableMetricsClient(promClient)
	tp := telemetry.NewTelemetryProvider(retriable)
	scorer := engine.NewScorer()
	registry := engine.NewRuleRegistry()
	registry.Register(&rules.CPUOverprovisionedRule{})
	registry.Register(&rules.MemOverprovisionedRule{})
	registry.Register(&rules.IdleWorkloadRule{})
	predictor := forecasting.NewPredictor(retriable)
	opts := []engine.AnalyzerOption{}
	if carbonProv != nil {
		opts = append(opts, engine.WithCarbonProvider(carbonProv))
	}
	return engine.NewAnalyzer(tp, registry, scorer, predictor, opts...), nil
}

func (r *OptimizationPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("optimizationpolicy-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&optimizationv1alpha1.OptimizationPolicy{}).
		Named("optimizationpolicy").
		Complete(r)
}
