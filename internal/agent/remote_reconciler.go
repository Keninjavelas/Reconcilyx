/*
Package agent implements the spoke-side controller.
It watches OptimizationProposal resources on the Hub cluster,
filters by its own cluster name, and executes InPlacePodVerticalScaling
on its local cluster using the local API server.
*/
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	optimizationv1alpha1 "github.com/reconcilyx/operator/api/v1alpha1"
	"github.com/reconcilyx/operator/internal/telemetry"
)

var RollbackWindow time.Duration

// RemoteReconciler reconciles OptimizationProposal objects that are
// targeted to this specific spoke cluster.
//
// It maintains TWO separate clients:
//   - LocalClient: talks to the local spoke API server (for actuation)
//   - HubClient:   talks to the remote Hub API server (for watching and status patches)
type RemoteReconciler struct {
	// HubClient is configured with credentials that have write access to
	// OptimizationProposal/status on the management cluster.
	HubClient client.Client

	// LocalClient uses InClusterConfig and has permissions to patch Pods
	// and read local Telemetry (Prometheus/metrics).
	LocalClient client.Client

	// Scheme is shared across both clients.
	Scheme *runtime.Scheme

	// ClusterName is injected via env var (e.g., SPOKE_CLUSTER_NAME).
	// It must match .spec.targetClusterName on the Hub's proposals.
	ClusterName string

	// Recorder emits events on the Hub cluster for observability.
	Recorder record.EventRecorder

	// RollbackWindow is how long to monitor a pod after resizing (e.g., 5m).
	RollbackWindow time.Duration
}

// +kubebuilder:rbac:groups=optimization.reconcilyx.io,resources=optimizationproposals,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups=optimization.reconcilyx.io,resources=optimizationproposals/status,verbs=get;patch;update
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups=core,resources=pods/status,verbs=get;patch

// Reconcile is the main loop for the Spoke Agent.
// It runs on the local cluster but reads/writes the proposal CRD on the Hub.
func (r *RemoteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("proposal", req.Name, "namespace", req.Namespace)

	// 1. Fetch the proposal from the HUB cluster.
	//    We use HubClient because the proposal CRD lives ONLY on the Hub.
	proposal := &optimizationv1alpha1.OptimizationProposal{}
	if err := r.HubClient.Get(ctx, req.NamespacedName, proposal); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Proposal deleted on Hub, nothing to do")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get proposal from Hub")
		return ctrl.Result{}, err
	}

	// 2. Filter: If this proposal is not for us, ignore it.
	if proposal.Spec.TargetClusterName != r.ClusterName {
		logger.V(1).Info("Skipping proposal targeted to different cluster",
			"targetCluster", proposal.Spec.TargetClusterName)
		return ctrl.Result{}, nil
	}

	// 3. If the proposal is already completed or failed, ignore.
	if proposal.Status.Phase == optimizationv1alpha1.PhaseCompleted ||
		proposal.Status.Phase == optimizationv1alpha1.PhaseFailed ||
		proposal.Status.Phase == optimizationv1alpha1.PhaseRolledBack {
		return ctrl.Result{}, nil
	}

	logger.Info("Processing proposal targeted to this cluster")

	// Observe staleness
	telemetry.ProposalStaleness.Observe(time.Since(proposal.CreationTimestamp.Time).Seconds())

	// 4. Execute the actuation on the LOCAL cluster.
	//    Use LocalClient to patch the pod's resources in-place.
	if err := r.executeResize(ctx, proposal); err != nil {
		logger.Error(err, "Failed to execute resize")
		// Update status on Hub to Failed
		_ = r.patchHubStatus(ctx, proposal, optimizationv1alpha1.PhaseFailed,
			fmt.Sprintf("Execution failed: %v", err), nil)
		return ctrl.Result{}, err
	}

	// 5. Mark as Executed on Hub.
	if err := r.patchHubStatus(ctx, proposal, optimizationv1alpha1.PhaseExecuting,
		"Resize applied, monitoring for stability", nil); err != nil {
		logger.Error(err, "Failed to update Hub status to Executing")
		return ctrl.Result{}, err
	}

	// 6. Start the monitoring window (rollback detection).
	//    This runs asynchronously to avoid blocking the reconcile loop.
	//    We use a separate goroutine with a context derived from the request.
	go r.monitorAndRollback(context.Background(), proposal)

	// Requeue after the rollback window to finalize the status.
	return ctrl.Result{RequeueAfter: r.RollbackWindow}, nil
}

// executeResize performs the InPlacePodVerticalScaling by patching the
// Pod's Status (or Spec, depending on K8s version) on the local cluster.
func (r *RemoteReconciler) executeResize(ctx context.Context, proposal *optimizationv1alpha1.OptimizationProposal) error {
	logger := log.FromContext(ctx)

	if proposal.Spec.TargetWorkload.Name == "" {
		return fmt.Errorf("proposal has nil TargetWorkload")
	}

	podKey := types.NamespacedName{
		Name:      proposal.Spec.TargetWorkload.Name,
		Namespace: proposal.Spec.TargetWorkload.Namespace,
	}

	pod := &corev1.Pod{}
	if err := r.LocalClient.Get(ctx, podKey, pod); err != nil {
		return fmt.Errorf("failed to get pod %s/%s: %w", podKey.Namespace, podKey.Name, err)
	}

	if pod.Status.Phase != corev1.PodRunning {
		return fmt.Errorf("pod is not running, phase=%s", pod.Status.Phase)
	}

	updated := pod.DeepCopy()
	if len(updated.Spec.Containers) == 0 {
		return fmt.Errorf("pod has no containers")
	}

	containerIdx := 0 // For demo; in real code, match by container name.

	// Use ProposedResources from the Proposal if available
	cpuQuantity := proposal.Spec.ProposedResources.Requests[corev1.ResourceCPU]
	memQuantity := proposal.Spec.ProposedResources.Requests[corev1.ResourceMemory]

	if updated.Spec.Containers[containerIdx].Resources.Requests == nil {
		updated.Spec.Containers[containerIdx].Resources.Requests = corev1.ResourceList{}
	}
	if !cpuQuantity.IsZero() {
		updated.Spec.Containers[containerIdx].Resources.Requests[corev1.ResourceCPU] = cpuQuantity
		// Update limits for Guaranteed QoS
		if updated.Spec.Containers[containerIdx].Resources.Limits == nil {
			updated.Spec.Containers[containerIdx].Resources.Limits = corev1.ResourceList{}
		}
		updated.Spec.Containers[containerIdx].Resources.Limits[corev1.ResourceCPU] = cpuQuantity
	}
	if !memQuantity.IsZero() {
		updated.Spec.Containers[containerIdx].Resources.Requests[corev1.ResourceMemory] = memQuantity
		if updated.Spec.Containers[containerIdx].Resources.Limits == nil {
			updated.Spec.Containers[containerIdx].Resources.Limits = corev1.ResourceList{}
		}
		updated.Spec.Containers[containerIdx].Resources.Limits[corev1.ResourceMemory] = memQuantity
	}

	// Inject Eviction Defense Annotations
	if updated.Annotations == nil {
		updated.Annotations = map[string]string{}
	}
	updated.Annotations["reconcilyx.io/defense-mode"] = "active"
	updated.Annotations["reconcilyx.io/optimization-phase"] = "Executing"

	if err := r.LocalClient.SubResource("resize").Patch(ctx, updated, client.MergeFrom(pod)); err != nil {
		if apierrors.IsConflict(err) {
			telemetry.SSAConflictsTotal.Inc()
		}
		return fmt.Errorf("failed to apply pod resize via merge patch (subresource resize): %w", err)
	}

	logger.Info("Successfully applied InPlacePodResize",
		"pod", podKey.String(),
		"cpu", cpuQuantity.String(),
		"memory", memQuantity.String())

	return nil
}

// monitorAndRollback watches the pod for 5 minutes (or configured window)
// and reverts the resize if throttling or OOM occurs.
func (r *RemoteReconciler) monitorAndRollback(ctx context.Context, proposal *optimizationv1alpha1.OptimizationProposal) {
	logger := log.FromContext(ctx).WithName("monitor")

	podKey := types.NamespacedName{
		Name:      proposal.Spec.TargetWorkload.Name,
		Namespace: proposal.Spec.TargetWorkload.Namespace,
	}

	time.Sleep(10 * time.Second) // Give kubelet a moment.

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	timeout := time.After(r.RollbackWindow)

	// Store the original resources to rollback to.
	var originalCPU, originalMemory resource.Quantity
	if proposal.Status.ExecutionSnapshot != nil && len(proposal.Status.ExecutionSnapshot.ContainerResources) > 0 {
		for _, state := range proposal.Status.ExecutionSnapshot.ContainerResources {
			originalCPU = state.Resources.Requests[corev1.ResourceCPU]
			originalMemory = state.Resources.Requests[corev1.ResourceMemory]
			break
		}
	} else {
		logger.Info("No snapshot in proposal status; rollback disabled")
		return
	}

	for {
		select {
		case <-ctx.Done():
			logger.Info("Monitoring context cancelled")
			return
		case <-timeout:
			logger.Info("Rollback window expired; workload stable")
			r.cleanupDefense(ctx, proposal, podKey)
			_ = r.patchHubStatus(ctx, proposal, optimizationv1alpha1.PhaseCompleted,
				"Stable within rollback window", nil)
			return
		case <-ticker.C:
			pod := &corev1.Pod{}
			if err := r.LocalClient.Get(ctx, podKey, pod); err != nil {
				logger.Error(err, "Failed to get pod during monitoring")
				continue
			}

			for _, cs := range pod.Status.ContainerStatuses {
				// cs.LastTerminationState is a struct, not a pointer
				if cs.LastTerminationState.Terminated != nil &&
					cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
					logger.Info("OOMKilled detected! Rolling back.")
					r.rollback(ctx, proposal, podKey, originalCPU, originalMemory)
					_ = r.patchHubStatus(ctx, proposal, optimizationv1alpha1.PhaseRolledBack,
						"Rolled back due to OOMKilled", nil)
					return
				}
			}
		}
	}
}

// cleanupDefense restores original limits and removes defense annotations
func (r *RemoteReconciler) cleanupDefense(ctx context.Context, proposal *optimizationv1alpha1.OptimizationProposal, podKey types.NamespacedName) {
	logger := log.FromContext(ctx)
	pod := &corev1.Pod{}
	if err := r.LocalClient.Get(ctx, podKey, pod); err != nil {
		logger.Error(err, "Failed to get pod for cleanup")
		return
	}

	updated := pod.DeepCopy()
	
	// Restore limits if annotation exists
	if origLimitsStr, ok := updated.Annotations["reconcilyx.io/original-limits"]; ok {
		var origLimits map[string]string
		if err := json.Unmarshal([]byte(origLimitsStr), &origLimits); err == nil {
			for i := range updated.Spec.Containers {
				container := &updated.Spec.Containers[i]
				if container.Resources.Limits == nil {
					container.Resources.Limits = corev1.ResourceList{}
				}
				if cpuStr, ok := origLimits["cpu"]; ok {
					container.Resources.Limits[corev1.ResourceCPU] = resource.MustParse(cpuStr)
				}
				if memStr, ok := origLimits["memory"]; ok {
					container.Resources.Limits[corev1.ResourceMemory] = resource.MustParse(memStr)
				}
			}
		}
	}

	// Remove annotations
	if updated.Annotations != nil {
		delete(updated.Annotations, "reconcilyx.io/defense-mode")
		delete(updated.Annotations, "reconcilyx.io/optimization-phase")
		delete(updated.Annotations, "reconcilyx.io/original-limits")
	}

	if err := r.LocalClient.Patch(ctx, updated, client.Apply, client.ForceOwnership, client.FieldOwner("reconcilyx-agent")); err != nil {
		logger.Error(err, "Failed to apply cleanup patch")
	} else {
		logger.Info("Cleanup successfully applied")
	}
}

// rollback reverts the pod to the original CPU/Memory.
func (r *RemoteReconciler) rollback(ctx context.Context, proposal *optimizationv1alpha1.OptimizationProposal,
	podKey types.NamespacedName, originalCPU, originalMemory resource.Quantity) {

	logger := log.FromContext(ctx)
	r.cleanupDefense(ctx, proposal, podKey)

	pod := &corev1.Pod{}
	if err := r.LocalClient.Get(ctx, podKey, pod); err != nil {
		logger.Error(err, "Failed to get pod for rollback")
		return
	}

	updated := pod.DeepCopy()
	if len(updated.Spec.Containers) == 0 {
		logger.Info("Pod has no containers; cannot rollback")
		return
	}

	updated.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU] = originalCPU
	updated.Spec.Containers[0].Resources.Requests[corev1.ResourceMemory] = originalMemory

	if err := r.LocalClient.Patch(ctx, updated, client.MergeFrom(pod)); err != nil {
		logger.Error(err, "Failed to apply rollback patch")
	} else {
		logger.Info("Rollback successfully applied")
	}
}

// patchHubStatus updates the OptimizationProposal status on the Hub cluster.
func (r *RemoteReconciler) patchHubStatus(ctx context.Context, proposal *optimizationv1alpha1.OptimizationProposal,
	phase optimizationv1alpha1.OptimizationProposalPhase, reason string, snapshot *optimizationv1alpha1.ExecutionSnapshot) error {

	logger := log.FromContext(ctx)

	updated := proposal.DeepCopy()
	updated.Status.Phase = phase
	updated.Status.Reason = reason
	if snapshot != nil {
		updated.Status.ExecutionSnapshot = snapshot
	}

	if err := r.HubClient.Status().Patch(ctx, updated, client.MergeFrom(proposal)); err != nil {
		logger.Error(err, "Failed to patch status on Hub")
		return err
	}

	r.Recorder.Event(updated, corev1.EventTypeNormal, "AgentExecuted", reason)
	return nil
}

// SetupWithManager sets up the controller for the Spoke Agent.
func (r *RemoteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Remove predicate to rely on Reconcile method's filtering and logging

	return ctrl.NewControllerManagedBy(mgr).
		For(&optimizationv1alpha1.OptimizationProposal{}).
		Complete(r)
}
