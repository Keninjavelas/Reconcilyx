package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ActuationControl determines if global workload mutation is enabled.
type ActuationControl interface {
	IsExecutionPaused(ctx context.Context) (bool, error)
}

type defaultActuationControl struct {
	client client.Client
	ns     string
}

// NewActuationControl creates an actuation control bound to reconcilyx-system namespace config.
func NewActuationControl(client client.Client, namespace string) ActuationControl {
	return &defaultActuationControl{
		client: client,
		ns:     namespace,
	}
}

func (c *defaultActuationControl) IsExecutionPaused(ctx context.Context) (bool, error) {
	cm := &corev1.ConfigMap{}
	err := c.client.Get(ctx, types.NamespacedName{Name: "reconcilyx-config", Namespace: c.ns}, cm)
	if err != nil {
		// If configmap is missing, default to secure/enabled, or log it
		// For safety, missing ConfigMap means Actuation is NOT paused.
		return false, nil
	}

	val, exists := cm.Data["actuation_paused"]
	if exists && val == "true" {
		return true, nil
	}

	return false, nil
}
