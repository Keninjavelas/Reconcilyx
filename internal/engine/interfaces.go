package engine

import (
	"context"

	"github.com/reconcilyx/operator/api/v1alpha1"
	"github.com/reconcilyx/operator/internal/telemetry"
	corev1 "k8s.io/api/core/v1"
)

// Analyzer coordinates the telemetry collection and rule evaluation pipeline.
type Analyzer interface {
	Analyze(ctx context.Context, ref corev1.ObjectReference, currentResources map[string]corev1.ResourceRequirements) (*v1alpha1.OptimizationProposal, error)
}

// Rule defines a single evaluation heuristic.
type Rule interface {
	Name() string
	Description() string
	// Evaluate analyzes a single container's telemetry and returns a Finding if a rule violation is detected.
	// Returns nil if no violation is found.
	Evaluate(containerName string, metrics *telemetry.ContainerMetrics, currentResources corev1.ResourceRequirements) (*Finding, error)
}

// RuleRegistry manages the set of active rules.
type RuleRegistry interface {
	Register(rule Rule)
	All() []Rule
}

// Scorer calculates risk and confidence based on telemetry health.
type Scorer interface {
	Score(metrics *telemetry.WorkloadMetrics, isPartial bool) (confidence int32, risk int32)
}
