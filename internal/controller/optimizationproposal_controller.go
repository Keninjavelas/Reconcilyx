package controller

import (
	"context"
	"fmt"
	"os"
	"strings"
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

	optimizationv1alpha1 "github.com/reconcilyx/operator/api/v1alpha1"
	"github.com/reconcilyx/operator/internal/carbon"
	"github.com/reconcilyx/operator/internal/engine/rl"
	"github.com/reconcilyx/operator/internal/gitops"
	"github.com/reconcilyx/operator/internal/policy"
	"github.com/reconcilyx/operator/internal/telemetry"
)

type GitProviderFactory interface {
	ForPolicy(policy *optimizationv1alpha1.OptimizationPolicy) (gitops.GitProvider, error)
}

type OptimizationProposalReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	Recorder        record.EventRecorder
	Auditor         Auditor
	Matcher         policy.Matcher
	Evaluator       policy.Evaluator
	ExecutionGate   ExecutionGate
	Actuator        Actuator
	RollbackManager RollbackManager
	GitFactory      GitProviderFactory
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
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch;get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods/resize,verbs=update;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch
// +kubebuilder:rbac:groups=autoscaling.k8s.io,resources=verticalpodautoscalers,verbs=get;list;watch

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

	if proposal.Status.Phase == optimizationv1alpha1.PhaseAwaitingExternalApproval {
		return r.reconcileExternalApproval(ctx, proposal)
	}

	if proposal.Status.Phase == optimizationv1alpha1.PhaseRollingBack {
		return r.reconcileManualRollback(ctx, proposal)
	}

	if proposal.Status.Phase == optimizationv1alpha1.PhaseDetected || proposal.Status.Phase == "" {
		var policyList optimizationv1alpha1.OptimizationPolicyList
		if err := r.List(ctx, &policyList); err != nil {
			return ctrl.Result{}, err
		}

		namespaceLabels, workloadLabels, err := workloadMatchLabels(ctx, r.Client, proposal)
		if err != nil {
			return ctrl.Result{}, err
		}
		matchedPolicy, err := r.Matcher.Match(ctx, proposal.Spec.TargetWorkload, namespaceLabels, workloadLabels, policyList.Items)
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
		decision, reason, err := r.evaluatePolicy(ctx, proposal, matchedPolicy)
		if err != nil {
			logger.Error(err, "Could not evaluate policy")
			return ctrl.Result{}, err
		}

		proposal.Status.ApprovalState = decision
		proposal.Status.Reason = reason
		proposal.Status.PolicyApplied = matchedPolicy.Name
		proposal.Status.LastEvaluationTime = &metav1.Time{Time: time.Now()}
		if r.Auditor != nil {
			if auditErr := r.Auditor.RecordEvaluation(ctx, proposal, matchedPolicy, fmt.Sprintf("%s: %s", decision, reason)); auditErr != nil {
				logger.Error(auditErr, "Could not record policy evaluation audit", "proposal", proposal.Name)
			}
		}

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
					return ctrl.Result{RequeueAfter: time.Minute}, nil
				}
			}
		} else if decision == optimizationv1alpha1.ApprovalRejected {
			proposal.Status.Phase = optimizationv1alpha1.PhaseRejected
			r.Recorder.Event(proposal, "Warning", "ProposalRejected", reason)
		} else {
			// A Pending decision awaits human review. Keep the phase non-terminal
			// so the approval API can promote Evaluating to Approved.
			proposal.Status.Phase = optimizationv1alpha1.PhaseEvaluating
			r.Recorder.Event(proposal, "Normal", "ProposalPendingReview", reason)
		}

		if err := r.Status().Update(ctx, proposal); err != nil {
			return ctrl.Result{}, err
		}
		if decision != optimizationv1alpha1.ApprovalApproved {
			return ctrl.Result{}, nil
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
		if proposal.Status.PolicyApplied == "" {
			return r.failScheduledProposal(ctx, proposal, "JIT Validation Rejected: proposal has no applied policy recorded; refusing to execute without policy guardrails"), nil
		}
		appliedPolicy, policyErr := r.lookupAppliedPolicy(ctx, proposal)
		if policyErr != nil {
			if apierrors.IsNotFound(policyErr) {
				return r.failScheduledProposal(ctx, proposal, fmt.Sprintf("JIT Validation Rejected: applied policy %q no longer exists", proposal.Status.PolicyApplied)), nil
			}
			return ctrl.Result{}, policyErr
		}

		decision, gateErr := r.ExecutionGate.ValidateJIT(ctx, proposal, appliedPolicy)
		if gateErr != nil {
			logger.Error(gateErr, "JIT validation encountered an error")
		}

		switch decision {
		case ExecutionReject:
			reason := "JIT Validation Rejected"
			if gateErr != nil {
				reason += ": " + gateErr.Error()
			}
			r.recordExecutionAttempt(ctx, proposal, "rejected: "+reason)
			return r.failScheduledProposal(ctx, proposal, reason), nil
		case ExecutionExpire:
			proposal.Status.Phase = optimizationv1alpha1.PhaseExpired
			_ = r.Status().Update(ctx, proposal)
			return ctrl.Result{}, nil
		case ExecutionRetryLater:
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		case ExecutionExecute:
			r.recordExecutionAttempt(ctx, proposal, "allowed: JIT validation passed")
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
			r.recordExecutionAttempt(ctx, proposal, "failed: "+err.Error())
			_ = r.Actuator.ReleaseLock(ctx, proposal)
			return ctrl.Result{}, nil
		}

		r.recordExecutionAttempt(ctx, proposal, "applied: in-place pod resize")
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
			r.recordExecutionAttempt(ctx, proposal, "rolled-back: workload stress exceeded rollback threshold")
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

		r.recordExecutionAttempt(ctx, proposal, "completed: workload stabilized")
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

// evaluatePolicy supplies the policy engine with the live context its
// cooldown and workload-protection constraints depend on.
func (r *OptimizationProposalReconciler) evaluatePolicy(ctx context.Context, proposal *optimizationv1alpha1.OptimizationProposal, matchedPolicy *optimizationv1alpha1.OptimizationPolicy) (optimizationv1alpha1.ApprovalState, string, error) {
	lastOptimization, err := lastExecutionForWorkload(ctx, r.Client, proposal)
	if err != nil {
		return "", "", err
	}
	containers, err := workloadContainerNames(ctx, r.Client, proposal)
	if err != nil {
		return "", "", err
	}
	decision, reason := r.Evaluator.Evaluate(ctx, proposal, matchedPolicy, containers, lastOptimization)
	return decision, reason, nil
}

func (r *OptimizationProposalReconciler) recordExecutionAttempt(ctx context.Context, proposal *optimizationv1alpha1.OptimizationProposal, decision string) {
	if r.Auditor == nil {
		return
	}
	if err := r.Auditor.RecordExecutionAttempt(ctx, proposal, decision); err != nil {
		log.FromContext(ctx).Error(err, "Could not record execution audit", "proposal", proposal.Name)
	}
}

func (r *OptimizationProposalReconciler) createGitOpsPR(ctx context.Context, proposal *optimizationv1alpha1.OptimizationProposal, policy *optimizationv1alpha1.OptimizationPolicy) (string, error) {
	if r.GitFactory == nil {
		return "", fmt.Errorf("git provider factory not configured")
	}
	provider, err := r.GitFactory.ForPolicy(policy)
	if err != nil {
		return "", err
	}

	dep := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      proposal.Spec.TargetWorkload.Name,
		Namespace: proposal.Spec.TargetWorkload.Namespace,
	}, dep); err != nil {
		return "", fmt.Errorf("failed to get target deployment: %w", err)
	}

	result, err := gitops.NewPRManager(provider).CreateProposalPR(
		ctx,
		proposal,
		extractCurrentResources(dep),
		gitops.PROptions{
			ManifestsPath: policy.Spec.GitOps.ManifestsPath,
			BaseBranch:    policy.Spec.GitOps.BaseBranch,
			AutoMerge:     policy.Spec.GitOps.AutoMerge,
		},
	)
	if err != nil {
		return "", err
	}
	return result.ID, nil
}

// reconcileExternalApproval polls the PR that gates execution. A merged PR
// completes the proposal because the GitOps controller owns the rollout; a
// closed PR is rejected; an open PR is retried until it merges or the TTL
// expires.
func (r *OptimizationProposalReconciler) reconcileExternalApproval(ctx context.Context, proposal *optimizationv1alpha1.OptimizationProposal) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if proposal.Status.ExternalPRID == "" {
		proposal.Status.Phase = optimizationv1alpha1.PhaseFailed
		proposal.Status.Reason = "AwaitingExternalApproval without a pull request ID"
		if err := r.Status().Update(ctx, proposal); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if r.GitFactory == nil {
		return ctrl.Result{RequeueAfter: time.Minute}, fmt.Errorf("git provider factory not configured")
	}

	policy, err := r.lookupAppliedPolicy(ctx, proposal)
	if err != nil {
		if apierrors.IsNotFound(err) {
			proposal.Status.Phase = optimizationv1alpha1.PhaseRejected
			proposal.Status.ApprovalState = optimizationv1alpha1.ApprovalRejected
			proposal.Status.Reason = fmt.Sprintf("Policy %q no longer exists; cannot resolve GitOps approval", proposal.Status.PolicyApplied)
			if updateErr := r.Status().Update(ctx, proposal); updateErr != nil {
				return ctrl.Result{}, updateErr
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	provider, err := r.GitFactory.ForPolicy(policy)
	if err != nil {
		logger.Error(err, "Cannot resolve Git provider for external approval", "proposal", proposal.Name)
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	status, err := provider.GetPullRequestStatus(ctx, proposal.Status.ExternalPRID)
	if err != nil {
		logger.Error(err, "Could not read GitOps PR status", "proposal", proposal.Name, "pr", proposal.Status.ExternalPRID)
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	switch {
	case status.Merged:
		proposal.Status.Phase = optimizationv1alpha1.PhaseCompleted
		proposal.Status.Reason = "GitOps change merged; rollout is owned by the GitOps controller"
		if err := r.Status().Update(ctx, proposal); err != nil {
			return ctrl.Result{}, err
		}
		r.Recorder.Event(proposal, "Normal", "GitOpsMerged", "Pull request merged; proposal completed")
		logger.Info("GitOps pull request merged", "proposal", proposal.Name, "pr", proposal.Status.ExternalPRID)
		return ctrl.Result{}, nil
	case status.State == "closed":
		proposal.Status.Phase = optimizationv1alpha1.PhaseRejected
		proposal.Status.ApprovalState = optimizationv1alpha1.ApprovalRejected
		proposal.Status.Reason = "GitOps pull request was closed without merging"
		if err := r.Status().Update(ctx, proposal); err != nil {
			return ctrl.Result{}, err
		}
		r.Recorder.Event(proposal, "Warning", "GitOpsClosed", "Pull request closed without merging")
		return ctrl.Result{}, nil
	default:
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}
}

// lookupAppliedPolicy resolves the policy recorded on the proposal. The
// recorded reference is a bare name, so the proposal namespace is tried first
// and a cluster-wide list is used as a fallback for cross-namespace policies.
func (r *OptimizationProposalReconciler) lookupAppliedPolicy(ctx context.Context, proposal *optimizationv1alpha1.OptimizationProposal) (*optimizationv1alpha1.OptimizationPolicy, error) {
	policy := &optimizationv1alpha1.OptimizationPolicy{}
	err := r.Get(ctx, types.NamespacedName{Name: proposal.Status.PolicyApplied, Namespace: proposal.Namespace}, policy)
	if err == nil {
		return policy, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, err
	}

	var list optimizationv1alpha1.OptimizationPolicyList
	if listErr := r.List(ctx, &list); listErr != nil {
		return nil, listErr
	}
	for i := range list.Items {
		if list.Items[i].Name == proposal.Status.PolicyApplied {
			return &list.Items[i], nil
		}
	}
	return nil, err
}

// reconcileManualRollback restores the captured snapshot when a reviewer
// requests a rollback through the API or CLI.
func (r *OptimizationProposalReconciler) reconcileManualRollback(ctx context.Context, proposal *optimizationv1alpha1.OptimizationProposal) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if r.RollbackManager == nil {
		return r.failScheduledProposal(ctx, proposal, "Manual rollback failed: rollback manager is not configured"), nil
	}
	if err := r.RollbackManager.ExecuteRollback(ctx, proposal); err != nil {
		logger.Error(err, "Manual rollback failed")
		r.recordExecutionAttempt(ctx, proposal, "rollback-failed: "+err.Error())
		return r.failScheduledProposal(ctx, proposal, "Manual rollback failed: "+err.Error()), nil
	}

	proposal.Status.Phase = optimizationv1alpha1.PhaseRolledBack
	proposal.Status.ExecutionState = optimizationv1alpha1.ExecutionReverted
	learningErr := r.applyRLLearning(ctx, proposal)
	if err := r.Status().Update(ctx, proposal); err != nil {
		return ctrl.Result{}, err
	}

	r.recordExecutionAttempt(ctx, proposal, "rolled-back: manual rollback restored the captured snapshot")
	if r.Recorder != nil {
		r.Recorder.Event(proposal, "Warning", "ExecutionReverted", "Manual rollback restored the captured snapshot")
	}
	if r.Actuator != nil {
		_ = r.Actuator.ReleaseLock(ctx, proposal)
	}

	if learningErr != nil {
		logger.Error(learningErr, "RL reward pending after rollback, will retry")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}
	return ctrl.Result{}, nil
}

func (r *OptimizationProposalReconciler) failScheduledProposal(ctx context.Context, proposal *optimizationv1alpha1.OptimizationProposal, reason string) ctrl.Result {
	proposal.Status.Phase = optimizationv1alpha1.PhaseFailed
	proposal.Status.ExecutionState = optimizationv1alpha1.ExecutionFailed
	proposal.Status.Reason = reason
	_ = r.Status().Update(ctx, proposal)
	if r.Recorder != nil {
		r.Recorder.Event(proposal, "Warning", "ExecutionFailed", reason)
	}
	return ctrl.Result{}
}

// DefaultPrometheusURL resolves the telemetry endpoint from the environment,
// preferring the chart-provided RECONCILYX_PROMETHEUS_URL.
func DefaultPrometheusURL() string {
	for _, key := range []string{"RECONCILYX_PROMETHEUS_URL", "PROMETHEUS_URL"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return "http://prometheus-operated.monitoring.svc.cluster.local:9090"
}

func (r *OptimizationProposalReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("optimizationproposal-controller")
	r.Auditor = NewAuditor(mgr.GetClient())
	r.Matcher = policy.NewMatcher()
	r.Evaluator = policy.NewEvaluator()
	r.ExecutionGate = NewExecutionGate(
		NewActuationControl(mgr.GetClient(), "reconcilyx-system"),
		NewWorkloadHealthGate(mgr.GetClient()),
		NewAutoscalerConflictGate(mgr.GetClient()),
		NewPolicyGuard(mgr.GetClient()),
	)
	r.Actuator = NewActuator(mgr.GetClient(), mgr.GetAPIReader())
	r.GitFactory = gitops.NewProviderFactory()

	telemetryProvider := r.TelemetryProvider
	if telemetryProvider == nil {
		promClient, err := telemetry.NewPrometheusClient(DefaultPrometheusURL())
		if err != nil {
			return err
		}
		telemetryProvider = telemetry.NewTelemetryProvider(telemetry.NewRetriableMetricsClient(promClient))
	}
	r.RollbackManager = NewRollbackManager(mgr.GetClient(), telemetryProvider)

	return ctrl.NewControllerManagedBy(mgr).
		For(&optimizationv1alpha1.OptimizationProposal{}).
		Named("optimizationproposal").
		Complete(r)
}
