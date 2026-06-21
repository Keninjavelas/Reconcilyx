package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("KillSwitch Control", func() {
	Context("When evaluating actuation paused state", func() {
		It("should return false if configmap does not exist", func() {
			client := fake.NewClientBuilder().Build()
			ctrl := NewActuationControl(client, "reconcilyx-system")

			paused, err := ctrl.IsExecutionPaused(context.TODO())
			Expect(err).NotTo(HaveOccurred())
			Expect(paused).To(BeFalse())
		})

		It("should return false if actuation_paused is not true", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "reconcilyx-config",
					Namespace: "reconcilyx-system",
				},
				Data: map[string]string{
					"actuation_paused": "false",
				},
			}
			client := fake.NewClientBuilder().WithObjects(cm).Build()
			ctrl := NewActuationControl(client, "reconcilyx-system")

			paused, err := ctrl.IsExecutionPaused(context.TODO())
			Expect(err).NotTo(HaveOccurred())
			Expect(paused).To(BeFalse())
		})

		It("should return true if actuation_paused is true", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "reconcilyx-config",
					Namespace: "reconcilyx-system",
				},
				Data: map[string]string{
					"actuation_paused": "true",
				},
			}
			client := fake.NewClientBuilder().WithObjects(cm).Build()
			ctrl := NewActuationControl(client, "reconcilyx-system")

			paused, err := ctrl.IsExecutionPaused(context.TODO())
			Expect(err).NotTo(HaveOccurred())
			Expect(paused).To(BeTrue())
		})
	})
})
