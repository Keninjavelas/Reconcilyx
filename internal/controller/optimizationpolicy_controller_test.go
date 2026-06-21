package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	optimizationv1alpha1 "github.com/reconcilyx/operator/api/v1alpha1"
)

var _ = Describe("OptimizationPolicy Controller", func() {
	Context("When reconciling an OptimizationPolicy", func() {
		const (
			resourceName      = "test-policy-validation"
			resourceNamespace = "default"
		)

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: resourceNamespace,
		}

		BeforeEach(func() {
			policy := &optimizationv1alpha1.OptimizationPolicy{}
			err := k8sClient.Get(ctx, typeNamespacedName, policy)
			if err != nil && errors.IsNotFound(err) {
				resource := &optimizationv1alpha1.OptimizationPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: resourceNamespace,
					},
					Spec: optimizationv1alpha1.OptimizationPolicySpec{
						Mode: optimizationv1alpha1.ModeObserve,
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &optimizationv1alpha1.OptimizationPolicy{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should successfully validate and update the policy status", func() {
			controllerReconciler := &OptimizationPolicyReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(100),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			var updated optimizationv1alpha1.OptimizationPolicy
			Expect(k8sClient.Get(ctx, typeNamespacedName, &updated)).To(Succeed())
			Expect(updated.Status.Conditions).To(HaveLen(1))
			Expect(updated.Status.Conditions[0].Type).To(Equal("Validated"))
			Expect(updated.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
		})
	})
})
