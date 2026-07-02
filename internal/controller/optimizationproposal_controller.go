/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
	"github.com/reconcilyx/operator/internal/policy"
	"github.com/reconcilyx/operator/internal/telemetry"
)

// OptimizationProposalReconciler reconciles a OptimizationProposal object
type OptimizationProposalReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Recorder      record.EventRecorder
	Matcher       policy.Matcher
	Evaluator     policy.Evaluator
	ExecutionGate ExecutionGate
	Actuator      Actuator
	RollbackManager RollbackManager
}

// +kubebuilder:rbac:groups=optimization.reconcilyx.io,resources=optimizationproposals,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=optimization.reconcilyx.io,resources=optimizationproposals/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=optimization.reconcilyx.io,resources=optimizationproposals/finalizers,verbs=update
// +kubebuilder:rbac:groups=optimization.reconcilyx.io,resources=optimizationpolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch

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

	// Terminal states
	if proposal.Status.Phase == optimizationv1alpha1.PhaseRejected ||
		proposal.Status.Phase == optimizationv1alpha1.PhaseExpired ||
		proposal.Status.Phase == optimizationv1alpha1.PhaseCompleted ||
		proposal.Status.Phase == optimizationv1alpha1.PhaseFailed ||
		proposal.Status.Phase == optimizationv1alpha1.PhaseRolledBack {
		
		// Lock Garbage Collection
		if r.Actuator != nil {
			_ = r.Actuator.ReleaseLock(ctx, proposal)
		}
		return ctrl.Result{}, nil
	}

	// 1. Freshness Check (TTL)
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

	// Evaluate Policy if Detected
	if proposal.Status.Phase == optimizationv1alpha1.PhaseDetected || proposal.Status.Phase == "" {
		// Fetch Policies
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
			r.Recorder.Event(proposal, "Normal", "ProposalApproved", "Policy Engine approved proposal")
		} else {
			proposal.Status.Phase = optimizationv1alpha1.PhaseRejected
			r.Recorder.Event(proposal, "Warning", "ProposalRejected", reason)
		}

		if err := r.Status().Update(ctx, proposal); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Execution Flow (Local Actuation Only)
	if proposal.Status.Phase == optimizationv1alpha1.PhaseApproved {
		proposal.Status.Phase = optimizationv1alpha1.PhaseScheduled
		if err := r.Status().Update(ctx, proposal); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// If the proposal is targeted for a Spoke cluster, the Hub stops processing here.
	// The Spoke Agent's remote_reconciler will pick it up and drive the execution phases.
	if proposal.Spec.TargetClusterName != "" {
		logger.V(1).Info("Delegating execution to Spoke Agent", "cluster", proposal.Spec.TargetClusterName)
		return ctrl.Result{}, nil
	}

	if proposal.Status.Phase == optimizationv1alpha1.PhaseScheduled {
		// JIT Validation
		var pol optimizationv1alpha1.OptimizationPolicy // Get policy if needed
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
		// 1. Acquire Lock
		acquired, err := r.Actuator.AcquireLock(ctx, proposal)
		if err != nil {
			return ctrl.Result{}, err
		}
		if !acquired {
			logger.Info("Target workload locked by another proposal, yielding")
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}

		// 2. Snapshot Capture
		if proposal.Status.ExecutionSnapshot == nil {
			snapshot, err := r.Actuator.CaptureSnapshot(ctx, proposal)
			if err != nil {
				return ctrl.Result{}, err
			}
			proposal.Status.ExecutionSnapshot = snapshot
			if err := r.Status().Update(ctx, proposal); err != nil {
				return ctrl.Result{}, err
			}
			// Requeue to ensure status is persisted before we mutate
			return ctrl.Result{Requeue: true}, nil
		}

		r.Recorder.Event(proposal, "Normal", "ExecutionStarted", "Acquired lock and captured execution snapshot")

		// 3. Apply In-Place Pod Resize
		if err := r.Actuator.ApplyMutation(ctx, proposal); err != nil {
			logger.Error(err, "Actuation failed")
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
		// Wait for 5 minutes monitoring window.
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
			if err := r.Status().Update(ctx, proposal); err != nil {
				return ctrl.Result{}, err
			}
			r.Recorder.Event(proposal, "Warning", "ExecutionReverted", "3-Strike Rollback Triggered due to workload stress")
			_ = r.Actuator.ReleaseLock(ctx, proposal)
			return ctrl.Result{}, nil
		}

		proposal.Status.Phase = optimizationv1alpha1.PhaseCompleted
		proposal.Status.ExecutionState = optimizationv1alpha1.ExecutionVerified
		if err := r.Status().Update(ctx, proposal); err != nil {
			return ctrl.Result{}, err
		}
		
		r.Recorder.Event(proposal, "Normal", "ExecutionCompleted", "Workload stabilized")
		_ = r.Actuator.ReleaseLock(ctx, proposal)
		
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OptimizationProposalReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("optimizationproposal-controller")
	r.Matcher = policy.NewMatcher()
	r.Evaluator = policy.NewEvaluator()
	r.ExecutionGate = NewExecutionGate(NewActuationControl(mgr.GetClient(), "reconcilyx-system"), NewWorkloadHealthGate())
	r.Actuator = NewActuator(mgr.GetClient())
	
	// Create a default telemetry provider for the rollback manager
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
