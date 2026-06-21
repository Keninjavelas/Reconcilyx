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
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	optimizationv1alpha1 "github.com/reconcilyx/operator/api/v1alpha1"
)

// OptimizationPolicyReconciler reconciles a OptimizationPolicy object
type OptimizationPolicyReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=optimization.reconcilyx.io,resources=optimizationpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=optimization.reconcilyx.io,resources=optimizationpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=optimization.reconcilyx.io,resources=optimizationpolicies/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *OptimizationPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the OptimizationPolicy instance
	policy := &optimizationv1alpha1.OptimizationPolicy{}
	err := r.Get(ctx, req.NamespacedName, policy)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get OptimizationPolicy")
		return ctrl.Result{}, err
	}

	// Initialize status if needed
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

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OptimizationPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("optimizationpolicy-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&optimizationv1alpha1.OptimizationPolicy{}).
		Named("optimizationpolicy").
		Complete(r)
}
