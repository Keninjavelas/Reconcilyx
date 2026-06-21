package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	optimizationv1alpha1 "github.com/reconcilyx/operator/api/v1alpha1"
)

var _ = Describe("Rollback Validator", func() {
	Context("When validating rollback readiness", func() {
		It("should fail if no execution snapshot exists", func() {
			validator := NewRollbackValidator()
			proposal := &optimizationv1alpha1.OptimizationProposal{}

			err := validator.CanRollback(context.TODO(), proposal, corev1.ObjectReference{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no execution snapshot available"))
		})

		It("should succeed if execution snapshot exists", func() {
			validator := NewRollbackValidator()
			proposal := &optimizationv1alpha1.OptimizationProposal{
				Status: optimizationv1alpha1.OptimizationProposalStatus{
					ExecutionSnapshot: &optimizationv1alpha1.ExecutionSnapshot{},
				},
			}

			err := validator.CanRollback(context.TODO(), proposal, corev1.ObjectReference{})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
