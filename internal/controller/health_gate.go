package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
)

// WorkloadHealthGate verifies target workload liveness.
type WorkloadHealthGate interface {
	CheckLiveness(ctx context.Context, target corev1.ObjectReference) error
}

type defaultHealthGate struct{}

func NewWorkloadHealthGate() WorkloadHealthGate {
	return &defaultHealthGate{}
}

func (h *defaultHealthGate) CheckLiveness(ctx context.Context, target corev1.ObjectReference) error {
	// TODO: Phase 2 - Implement health checking
	return nil
}
