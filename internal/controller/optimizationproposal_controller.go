package controller

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	optimizationv1alpha1 "github.com/reconcilyx/operator/api/v1alpha1"
	"github.com/reconcilyx/operator/internal/carbon"
	"github.com/reconcilyx/operator/internal/engine/rl"
	"github.com/reconcilyx/operator/internal/gitops"
	"github.com/reconcilyx/operator/internal/policy"
	"github.com/reconcilyx/operator/internal/telemetry"
)

type OptimizationProposalReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	Recorder        record.EventRecorder
	Matcher         policy.Matcher
	Evaluator       policy.Evaluator
	ExecutionGate   ExecutionGate
	Actuator        Actuator
	RollbackManager RollbackManager
	GitFactory      *gitops.ProviderFactory
	// RL Learning Loop dependencies
	RLAgent           rl.Agent
	TelemetryProvider telemetry.TelemetryProvider
	CarbonProvider    carbon.Provider
	WorkloadRegion    string
	CarbonWeight      float64
}

// +kubebuilder:rbac:groups=optimization.reconcilyx.io,resources=optimizationproposals,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=optimization.reconcilyx.io,resources=optimizationproposals/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=optimization.reconcilyx.io,resources=optimizationproposals/finalizers,verbs=update
// +kubebuilder:rbac:groups=optimization.reconcilyx.io,resources=optimizationpolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods/resize,verbs=update;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch

func (r *OptimizationProposalReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	proposal := &optimizationv1alpha1.OptimizationProposal{}
	if err := r.Get(ctx, req.NamespacedName, proposal); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get OptimizationProposal")
		return ctrl.Result{}, err
	}

	workload := proposal.Spec.TargetWorkload.Name
	ns := proposal.Spec.TargetWorkload.Namespace

	if proposal.Status.Phase == optimizationv1alpha1.PhaseRejected ||
		proposal.Status.Phase == optimizationv1alpha1.PhaseExpired ||
		proposal.Status.Phase == optimizationv1alpha1.PhaseCompleted ||
		proposal.Status.Phase == optimizationv1alpha1.PhaseFailed ||
		proposal.Status.Phase == optimizationv1alpha1.PhaseRolledBack {

		if (proposal.Status.Phase == optimizationv1alpha1.PhaseCompleted ||
			proposal.Status.Phase == optimizationv1alpha1.PhaseRolledBack) &&
			r.needsRLLearning(proposal) {
			err := r.applyRLLearning(ctx, proposal)
			if err == nil && proposal.Status.Learning != nil {
				err = r.Status().Update(ctx, proposal)
			}
			if err != nil {
				logger.Error(err, "RL learning could not be applied", "proposal", proposal.Name)
			}
		}

		if r.Actuator != nil {
			_ = r.Actuator.ReleaseLock(ctx, proposal)
		}
		return ctrl.Result{}, nil
	}

	if proposal.Status.ExpirationTime != nil && time.Now().After(proposal.Status.ExpirationTime.Time) {
		logger.Info("Proposal expired", "Proposal", proposal.Name)
		proposal.Status.Phase = optimizationv1alpha1.PhaseExpired
		if proposal.Status.ApprovalState == optimizationv1alpha1.ApprovalPending {
			proposal.Status.ApprovalState = optimizationv1alpha1.ApprovalRejected
		}
		proposal.Status.Reason = "Proposal exceeded its Time-To-Live (TTL)"
		if err := r.Status().Update(ctx, proposal); err != nil {
			return ctrl.Result{}, err
		}
		r.Recorder.Event(proposal, "Warning", "ProposalExpired", "Proposal exceeded TTL and was expired")
		return ctrl.Result{}, nil
	}

	if proposal.Status.Phase == optimizationv1alpha1.PhaseDetected || proposal.Status.Phase == "" {
		var policyList optimizationv1alpha1.OptimizationPolicyList
		if err := r.List(ctx, &policyList); err != nil {
			return ctrl.Result{}, err
		}

		matchedPolicy, err := r.Matcher.Match(ctx, proposal.Spec.TargetWorkload, map[string]string{}, map[string]string{}, policyList.Items)
		if err != nil {
			return ctrl.Result{}, err
		}

		if matchedPolicy == nil {
			proposal.Status.Phase = optimizationv1alpha1.PhaseRejected
			proposal.Status.ApprovalState = optimizationv1alpha1.ApprovalRejected
			proposal.Status.Reason = "No matching OptimizationPolicy found for workload"
			if err := r.Status().Update(ctx, proposal); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}

		proposal.Status.Phase = optimizationv1alpha1.PhaseEvaluating
		decision, reason := r.Evaluator.Evaluate(ctx, proposal, matchedPolicy, []string{}, nil)

		proposal.Status.ApprovalState = decision
		proposal.Status.Reason = reason
		proposal.Status.PolicyApplied = matchedPolicy.Name
		proposal.Status.LastEvaluationTime = &metav1.Time{Time: time.Now()}

		if decision == optimizationv1alpha1.ApprovalApproved {
			proposal.Status.Phase = optimizationv1alpha1.PhaseApproved
			telemetry.ProposalApprovedTotal.WithLabelValues(ns, matchedPolicy.Name, workload).Inc()
			r.Recorder.Event(proposal, "Normal", "ProposalApproved", "Policy Engine approved proposal")

			if matchedPolicy.Spec.GitOps.Enabled {
				prID, prErr := r.createGitOpsPR(ctx, proposal, matchedPolicy)
				if prErr != nil {
					logger.Error(prErr, "GitOps PR creation failed, falling back to local actuation")
					r.Recorder.Event(proposal, "Warning", "GitOPSError", "PR creation failed: "+prErr.Error())
				} else {
					proposal.Status.ExternalPRID = prID
					proposal.Status.Phase = optimizationv1alpha1.PhaseAwaitingExternalApproval
					proposal.Status.Reason = "GitOps PR created: " + prID
					if err := r.Status().Update(ctx, proposal); err != nil {
						return ctrl.Result{}, err
					}
					r.Recorder.Event(proposal, "Normal", "GitOpsPRCreated", "Created pull/merge request: "+prID)
					return ctrl.Result{}, nil
				}
			}
		} else {
			proposal.Status.Phase = optimizationv1alpha1.PhaseRejected
			r.Recorder.Event(proposal, "Warning", "ProposalRejected", reason)
		}

		if err := r.Status().Update(ctx, proposal); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	if proposal.Status.Phase == optimizationv1alpha1.PhaseApproved {
		proposal.Status.Phase = optimizationv1alpha1.PhaseScheduled
		if err := r.Status().Update(ctx, proposal); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	if proposal.Spec.TargetClusterName != "" {
		logger.V(1).Info("Delegating execution to Spoke Agent", "cluster", proposal.Spec.TargetClusterName)
		return ctrl.Result{}, nil
	}

	if proposal.Status.Phase == optimizationv1alpha1.PhaseScheduled {
		var pol optimizationv1alpha1.OptimizationPolicy
		decision, err := r.ExecutionGate.ValidateJIT(ctx, proposal, &pol)
		if err != nil {
			logger.Error(err, "JIT validation encountered an error")
		}

		switch decision {
		case ExecutionReject:
			proposal.Status.Phase = optimizationv1alpha1.PhaseFailed
			proposal.Status.ExecutionState = optimizationv1alpha1.ExecutionFailed
			proposal.Status.Reason = "JIT Validation Rejected: " + err.Error()
			_ = r.Status().Update(ctx, proposal)
			r.Recorder.Event(proposal, "Warning", "ExecutionFailed", proposal.Status.Reason)
			return ctrl.Result{}, nil
		case ExecutionExpire:
			proposal.Status.Phase = optimizationv1alpha1.PhaseExpired
			_ = r.Status().Update(ctx, proposal)
			return ctrl.Result{}, nil
		case ExecutionRetryLater:
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		case ExecutionExecute:
			proposal.Status.Phase = optimizationv1alpha1.PhaseExecuting
			proposal.Status.ExecutionState = optimizationv1alpha1.ExecutionApplying
			if err := r.Status().Update(ctx, proposal); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, nil
		}
	}

	if proposal.Status.Phase == optimizationv1alpha1.PhaseExecuting {
		acquired, err := r.Actuator.AcquireLock(ctx, proposal)
		if err != nil {
			return ctrl.Result{}, err
		}
		if !acquired {
			logger.Info("Target workload locked by another proposal, yielding")
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}

		if proposal.Status.ExecutionSnapshot == nil {
			snapshot, err := r.Actuator.CaptureSnapshot(ctx, proposal)
			if err != nil {
				return ctrl.Result{}, err
			}
			proposal.Status.ExecutionSnapshot = snapshot
			if proposal.Status.Learning == nil {
				proposal.Status.Learning = r.captureLearningPreState(ctx, proposal)
			}
			if err := r.Status().Update(ctx, proposal); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, nil
		}

		r.Recorder.Event(proposal, "Normal", "ExecutionStarted", "Acquired lock and captured execution snapshot")

		if err := r.Actuator.ApplyMutation(ctx, proposal); err != nil {
			logger.Error(err, "Actuation failed")
			telemetry.SSAConflictsTotal.WithLabelValues(workload, ns).Inc()
			proposal.Status.Phase = optimizationv1alpha1.PhaseFailed
			proposal.Status.ExecutionState = optimizationv1alpha1.ExecutionFailed
			proposal.Status.Reason = "In-Place Pod Resize Failed: " + err.Error()
			_ = r.Status().Update(ctx, proposal)
			r.Recorder.Event(proposal, "Warning", "ExecutionFailed", proposal.Status.Reason)
			_ = r.Actuator.ReleaseLock(ctx, proposal)
			return ctrl.Result{}, nil
		}

		r.Recorder.Event(proposal, "Normal", "PodResize", "In-Place Pod Resize patch applied successfully")

		proposal.Status.Phase = optimizationv1alpha1.PhaseVerifying
		proposal.Status.ExecutionState = optimizationv1alpha1.ExecutionStabilizing
		if err := r.Status().Update(ctx, proposal); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	if proposal.Status.Phase == optimizationv1alpha1.PhaseVerifying {
		dep := &appsv1.Deployment{}
		if err := r.Client.Get(ctx, types.NamespacedName{
			Name:      proposal.Spec.TargetWorkload.Name,
			Namespace: proposal.Spec.TargetWorkload.Namespace,
		}, dep); err != nil {
			return ctrl.Result{}, err
		}

		if proposal.Status.ExecutionSnapshot != nil && dep.Generation != proposal.Status.ExecutionSnapshot.Generation {
			proposal.Status.YieldCount++
			telemetry.GitOpsYieldsTotal.WithLabelValues(workload, ns).Inc()

			backoffDuration := 5 * time.Minute
			if proposal.Status.YieldCount == 2 {
				backoffDuration = 30 * time.Minute
			} else if proposal.Status.YieldCount >= 3 {
				backoffDuration = 24 * time.Hour
				r.Recorder.Event(proposal, "Warning", "GitOpsConflict", "Execution reverted 3 times. Yielding for 24h.")
			} else {
				r.Recorder.Event(proposal, "Warning", "GitOpsConflict", fmt.Sprintf("Execution reverted %d times. Yielding for %v.", proposal.Status.YieldCount, backoffDuration))
			}

			logger.Info("GitOps conflict detected, yielding", "yieldCount", proposal.Status.YieldCount, "backoff", backoffDuration)

			proposal.Status.Phase = optimizationv1alpha1.PhaseExecuting
			proposal.Status.ExecutionState = optimizationv1alpha1.ExecutionPending
			proposal.Status.ExecutionSnapshot = nil

			if err := r.Status().Update(ctx, proposal); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: backoffDuration}, nil
		}

		proposal.Status.Phase = optimizationv1alpha1.PhaseStabilizing
		if err := r.Status().Update(ctx, proposal); err != nil {
			return ctrl.Result{}, err
		}
		r.Recorder.Event(proposal, "Normal", "ExecutionVerified", "Mutation matches intended state")
		return ctrl.Result{Requeue: true}, nil
	}

	if proposal.Status.Phase == optimizationv1alpha1.PhaseStabilizing {
		if time.Since(proposal.Status.ExecutionSnapshot.Timestamp.Time) < 5*time.Minute {
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}

		shouldRollback, err := r.RollbackManager.CheckMonitoringWindow(ctx, proposal)
		if err != nil {
			logger.Error(err, "Rollback check encountered an error, extending stabilization phase")
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}

		if shouldRollback {
			logger.Info("Executing 3-Strike Rollback")
			if err := r.RollbackManager.ExecuteRollback(ctx, proposal); err != nil {
				return ctrl.Result{}, err
			}
			proposal.Status.Phase = optimizationv1alpha1.PhaseRolledBack
			proposal.Status.ExecutionState = optimizationv1alpha1.ExecutionReverted
			learningErr := r.applyRLLearning(ctx, proposal)
			if err := r.Status().Update(ctx, proposal); err != nil {
				return ctrl.Result{}, err
			}
			r.Recorder.Event(proposal, "Warning", "ExecutionReverted", "3-Strike Rollback Triggered due to workload stress")
			_ = r.Actuator.ReleaseLock(ctx, proposal)
			if learningErr != nil {
				logger.Error(learningErr, "RL reward pending after rollback, will retry")
				return ctrl.Result{RequeueAfter: time.Minute}, nil
			}
			return ctrl.Result{}, nil
		}

		proposal.Status.Phase = optimizationv1alpha1.PhaseCompleted
		proposal.Status.ExecutionState = optimizationv1alpha1.ExecutionVerified
		learningErr := r.applyRLLearning(ctx, proposal)
		if err := r.Status().Update(ctx, proposal); err != nil {
			return ctrl.Result{}, err
		}

		r.Recorder.Event(proposal, "Normal", "ExecutionCompleted", "Workload stabilized")
		_ = r.Actuator.ReleaseLock(ctx, proposal)

		if learningErr != nil {
			logger.Error(learningErr, "RL reward pending after completion, will retry")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

func (r *OptimizationProposalReconciler) createGitOpsPR(ctx context.Context, proposal *optimizationv1alpha1.OptimizationProposal, policy *optimizationv1alpha1.OptimizationPolicy) (string, error) {
	if r.GitFactory == nil {
		return "", fmt.Errorf("git provider factory not configured")
	}
	provider, err := r.GitFactory.ForPolicy(policy)
	if err != nil {
		return "", err
	}
	title := fmt.Sprintf("Reconcilyx: optimize %s/%s", proposal.Namespace, proposal.Spec.TargetWorkload.Name)
	body := fmt.Sprintf(`## Reconcilyx Optimization Proposal

**Workload:** %s/%s  
**Recommendation:** %s  
**Confidence:** %d  
**Risk:** %d  
**Est. Monthly Savings:** $%s

### Rationale
%s
`,
		proposal.Spec.TargetWorkload.Namespace,
		proposal.Spec.TargetWorkload.Name,
		proposal.Spec.Recommendation,
		proposal.Spec.DecisionMetadata.ConfidenceScore,
		proposal.Spec.DecisionMetadata.RiskScore,
		proposal.Spec.ExpectedImpact.EstimatedSavingsUSD.String(),
		proposal.Spec.DecisionMetadata.Rationale,
	)
	diff := fmt.Sprintf(`# Reconcilyx Optimization Patch
# Target: %s/%s
#
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  namespace: %s
  annotations:
    reconcilyx.io/proposal: %s
spec:
  template:
    spec:
      containers:
        # PATCH: set requests based on recommendation
        - name: <CONTAINER>
          resources:
            requests:
              cpu: %s
              memory: %s
`,
		proposal.Spec.TargetWorkload.Namespace, proposal.Spec.TargetWorkload.Name,
		proposal.Spec.TargetWorkload.Name,
		proposal.Spec.TargetWorkload.Namespace,
		proposal.Name,
		proposal.Spec.ProposedResources.Requests.Cpu().String(),
		proposal.Spec.ProposedResources.Requests.Memory().String(),
	)
	return provider.CreatePullRequest(ctx, &gitops.PullRequest{
		Title:       title,
		Description: body,
		HeadBranch:  fmt.Sprintf("reconcilyx/%s-%s", proposal.Spec.TargetWorkload.Namespace, proposal.Spec.TargetWorkload.Name),
		BaseBranch:  policy.Spec.GitOps.BaseBranch,
		Diff:        diff,
	})
}

func (r *OptimizationProposalReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("optimizationproposal-controller")
	r.Matcher = policy.NewMatcher()
	r.Evaluator = policy.NewEvaluator()
	r.ExecutionGate = NewExecutionGate(NewActuationControl(mgr.GetClient(), "reconcilyx-system"), NewWorkloadHealthGate(mgr.GetClient()))
	r.Actuator = NewActuator(mgr.GetClient())
	r.GitFactory = gitops.NewProviderFactory()

	promClient, err := telemetry.NewPrometheusClient("http://prometheus-operated.monitoring.svc.cluster.local:9090")
	if err != nil {
		return err
	}
	telemetryProvider := telemetry.NewTelemetryProvider(promClient)
	r.RollbackManager = NewRollbackManager(mgr.GetClient(), telemetryProvider)

	return ctrl.NewControllerManagedBy(mgr).
		For(&optimizationv1alpha1.OptimizationProposal{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Named("optimizationproposal").
		Complete(r)
}
