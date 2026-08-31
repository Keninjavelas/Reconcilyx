package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	optimizationv1alpha1 "github.com/reconcilyx/operator/api/v1alpha1"
	"github.com/reconcilyx/operator/internal/policy"
)

var _ = Describe("OptimizationProposal Controller", func() {
	Context("When reconciling an OptimizationProposal", func() {
		const (
			resourceName      = "test-proposal"
			resourceNamespace = "default"
		)

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: resourceNamespace,
		}

		BeforeEach(func() {
			// Clear any previous resources
			proposal := &optimizationv1alpha1.OptimizationProposal{}
			_ = k8sClient.Get(ctx, typeNamespacedName, proposal)
			_ = k8sClient.Delete(ctx, proposal)

			pol := &optimizationv1alpha1.OptimizationPolicy{}
			_ = k8sClient.Get(ctx, types.NamespacedName{Name: "test-policy", Namespace: resourceNamespace}, pol)
			_ = k8sClient.Delete(ctx, pol)

			// Create a generic policy that will match globally
			policyResource := &optimizationv1alpha1.OptimizationPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-policy",
					Namespace: resourceNamespace,
				},
				Spec: optimizationv1alpha1.OptimizationPolicySpec{
					Objective: optimizationv1alpha1.ObjectiveEfficiency,
					Mode:      optimizationv1alpha1.ModeAutoApply,
					RecommendationGovernance: optimizationv1alpha1.RecommendationGovernance{
						AllowedRecommendations: []optimizationv1alpha1.RecommendationType{
							optimizationv1alpha1.RecommendationRightSize,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, policyResource)).To(Succeed())
		})

		It("should successfully evaluate a safe proposal to Approved", func() {
			proposal := &optimizationv1alpha1.OptimizationProposal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: optimizationv1alpha1.OptimizationProposalSpec{
					TargetWorkload: corev1.ObjectReference{
						Kind:       "Deployment",
						Name:       "test-deployment",
						APIVersion: "apps/v1",
					},
					Recommendation: optimizationv1alpha1.RecommendationRightSize,
					DecisionMetadata: optimizationv1alpha1.DecisionMetadata{
						RiskScore:       10,
						ConfidenceScore: 90,
					},
				},
			}
			Expect(k8sClient.Create(ctx, proposal)).To(Succeed())

			reconciler := &OptimizationProposalReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				Recorder:  record.NewFakeRecorder(100),
				Matcher:   policy.NewMatcher(),
				Evaluator: policy.NewEvaluator(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			var updated optimizationv1alpha1.OptimizationProposal
			Expect(k8sClient.Get(ctx, typeNamespacedName, &updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(optimizationv1alpha1.PhaseApproved))
			Expect(updated.Status.ApprovalState).To(Equal(optimizationv1alpha1.ApprovalApproved))
			Expect(updated.Status.PolicyApplied).To(Equal("test-policy"))
		})

		It("should reject proposal if policy forbids the recommendation type", func() {
			proposal := &optimizationv1alpha1.OptimizationProposal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: optimizationv1alpha1.OptimizationProposalSpec{
					TargetWorkload:   corev1.ObjectReference{Kind: "Deployment", Name: "test-deployment", APIVersion: "apps/v1"},
					Recommendation:   optimizationv1alpha1.RecommendationScaleZero, // Not allowed by test-policy
					DecisionMetadata: optimizationv1alpha1.DecisionMetadata{RiskScore: 10, ConfidenceScore: 90},
				},
			}
			Expect(k8sClient.Create(ctx, proposal)).To(Succeed())

			reconciler := &OptimizationProposalReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				Recorder:  record.NewFakeRecorder(100),
				Matcher:   policy.NewMatcher(),
				Evaluator: policy.NewEvaluator(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			var updated optimizationv1alpha1.OptimizationProposal
			Expect(k8sClient.Get(ctx, typeNamespacedName, &updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(optimizationv1alpha1.PhaseRejected))
			Expect(updated.Status.ApprovalState).To(Equal(optimizationv1alpha1.ApprovalRejected))
			Expect(updated.Status.Reason).To(ContainSubstring("not in the policy allowed list"))
		})

		It("should transition proposal to Expired if TTL is exceeded", func() {
			proposal := &optimizationv1alpha1.OptimizationProposal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: optimizationv1alpha1.OptimizationProposalSpec{
					TargetWorkload: corev1.ObjectReference{Kind: "Deployment", Name: "test-deployment", APIVersion: "apps/v1"},
					Recommendation: optimizationv1alpha1.RecommendationRightSize,
				},
			}
			Expect(k8sClient.Create(ctx, proposal)).To(Succeed())

			// Artificially expire the proposal
			proposal.Status.ExpirationTime = &metav1.Time{Time: time.Now().Add(-1 * time.Hour)}
			proposal.Status.ApprovalState = optimizationv1alpha1.ApprovalPending
			Expect(k8sClient.Status().Update(ctx, proposal)).To(Succeed())

			reconciler := &OptimizationProposalReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				Recorder:  record.NewFakeRecorder(100),
				Matcher:   policy.NewMatcher(),
				Evaluator: policy.NewEvaluator(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			var updated optimizationv1alpha1.OptimizationProposal
			Expect(k8sClient.Get(ctx, typeNamespacedName, &updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(optimizationv1alpha1.PhaseExpired))
			Expect(updated.Status.ApprovalState).To(Equal(optimizationv1alpha1.ApprovalRejected))
			Expect(updated.Status.Reason).To(ContainSubstring("TTL"))
		})

		It("should ignore terminal proposals gracefully", func() {
			proposal := &optimizationv1alpha1.OptimizationProposal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: optimizationv1alpha1.OptimizationProposalSpec{
					TargetWorkload: corev1.ObjectReference{Kind: "Deployment", Name: "test-deployment", APIVersion: "apps/v1"},
					Recommendation: optimizationv1alpha1.RecommendationRightSize,
				},
			}
			Expect(k8sClient.Create(ctx, proposal)).To(Succeed())

			proposal.Status.Phase = optimizationv1alpha1.PhaseRejected
			proposal.Status.ApprovalState = optimizationv1alpha1.ApprovalRejected
			Expect(k8sClient.Status().Update(ctx, proposal)).To(Succeed())

			reconciler := &OptimizationProposalReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				Recorder:  record.NewFakeRecorder(100),
				Matcher:   policy.NewMatcher(),
				Evaluator: policy.NewEvaluator(),
			}

			// Calling reconcile should do nothing since it's already terminal
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			var updated optimizationv1alpha1.OptimizationProposal
			Expect(k8sClient.Get(ctx, typeNamespacedName, &updated)).To(Succeed())
			// Should remain unmodified
			Expect(updated.Status.Phase).To(Equal(optimizationv1alpha1.PhaseRejected))
			Expect(updated.Status.PolicyApplied).To(BeEmpty()) // it didn't run eval
		})
	})
})
