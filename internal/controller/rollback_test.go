package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	optimizationv1alpha1 "github.com/reconcilyx/operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Rollback Manager", func() {
	Context("When validating rollback readiness", func() {
		It("should fail if no execution snapshot exists", func() {
			fakeClient := fake.NewClientBuilder().Build()
			// using nil telemetry provider since we expect it to fail before telemetry is called
			manager := NewRollbackManager(fakeClient, nil)
			proposal := &optimizationv1alpha1.OptimizationProposal{}

			_, err := manager.CheckMonitoringWindow(context.TODO(), proposal)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no execution snapshot available for rollback check"))
		})
	})
})
