package policy

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/reconcilyx/operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourceConstraint evaluates Min/Max bounds for aggregate and per-container
// requests and limits.
type ResourceConstraint struct{}

func (c *ResourceConstraint) Name() string { return "ResourceBoundaries" }

func (c *ResourceConstraint) Evaluate(ctx context.Context, proposal *v1alpha1.OptimizationProposal, policy *v1alpha1.OptimizationPolicy, workloadContainers []string, lastOptimization *metav1.Time) (bool, string) {
	bounds := policy.Spec.ResourceConstraints
	var violations []string

	violations = append(violations, resourceViolations("", "request", proposal.Spec.ProposedResources.Requests, bounds)...)
	violations = append(violations, resourceViolations("", "limit", proposal.Spec.ProposedResources.Limits, bounds)...)

	containers := make([]string, 0, len(proposal.Spec.ProposedResourcesByContainer))
	for name := range proposal.Spec.ProposedResourcesByContainer {
		containers = append(containers, name)
	}
	sort.Strings(containers)
	for _, name := range containers {
		resources := proposal.Spec.ProposedResourcesByContainer[name]
		violations = append(violations, resourceViolations(name, "request", resources.Requests, bounds)...)
		violations = append(violations, resourceViolations(name, "limit", resources.Limits, bounds)...)
	}

	if len(violations) > 0 {
		return false, strings.Join(violations, "; ")
	}

	return true, ""
}

func resourceViolations(container, field string, resources corev1.ResourceList, bounds v1alpha1.ResourceConstraints) []string {
	if resources == nil {
		return nil
	}

	var violations []string
	scope := field
	if container != "" {
		scope = fmt.Sprintf("container %q %s", container, field)
	}

	if cpu, ok := resources[corev1.ResourceCPU]; ok {
		if bounds.MinCPU != nil && cpu.MilliValue() < bounds.MinCPU.MilliValue() {
			violations = append(violations, fmt.Sprintf("CPU %s (%s) < MinCPU (%s)", scope, cpu.String(), bounds.MinCPU.String()))
		}
		if bounds.MaxCPU != nil && cpu.MilliValue() > bounds.MaxCPU.MilliValue() {
			violations = append(violations, fmt.Sprintf("CPU %s (%s) > MaxCPU (%s)", scope, cpu.String(), bounds.MaxCPU.String()))
		}
	}

	if mem, ok := resources[corev1.ResourceMemory]; ok {
		if bounds.MinMemory != nil && mem.Value() < bounds.MinMemory.Value() {
			violations = append(violations, fmt.Sprintf("Memory %s (%s) < MinMemory (%s)", scope, mem.String(), bounds.MinMemory.String()))
		}
		if bounds.MaxMemory != nil && mem.Value() > bounds.MaxMemory.Value() {
			violations = append(violations, fmt.Sprintf("Memory %s (%s) > MaxMemory (%s)", scope, mem.String(), bounds.MaxMemory.String()))
		}
	}

	return violations
}

// SafetyConstraint evaluates engine confidence and risk.
type SafetyConstraint struct{}

func (c *SafetyConstraint) Name() string { return "SafetyScores" }

func (c *SafetyConstraint) Evaluate(ctx context.Context, proposal *v1alpha1.OptimizationProposal, policy *v1alpha1.OptimizationPolicy, workloadContainers []string, lastOptimization *metav1.Time) (bool, string) {
	meta := proposal.Spec.DecisionMetadata

	maxRisk := int32(50)
	if policy.Spec.SafetyConstraints.RiskTolerance != nil {
		maxRisk = *policy.Spec.SafetyConstraints.RiskTolerance
	}

	minConf := int32(80)
	if policy.Spec.SafetyConstraints.ConfidenceRequirement != nil {
		minConf = *policy.Spec.SafetyConstraints.ConfidenceRequirement
	}

	if meta.RiskScore > maxRisk {
		return false, fmt.Sprintf("Risk Score (%d) exceeds configured safety threshold (%d)", meta.RiskScore, maxRisk)
	}
	if meta.ConfidenceScore < minConf {
		return false, fmt.Sprintf("Confidence Score (%d) is below configured requirement (%d)", meta.ConfidenceScore, minConf)
	}

	return true, ""
}

// RecommendationConstraint evaluates allowed recommendation types.
type RecommendationConstraint struct{}

func (c *RecommendationConstraint) Name() string { return "RecommendationGovernance" }

func (c *RecommendationConstraint) Evaluate(ctx context.Context, proposal *v1alpha1.OptimizationProposal, policy *v1alpha1.OptimizationPolicy, workloadContainers []string, lastOptimization *metav1.Time) (bool, string) {
	allowed := policy.Spec.RecommendationGovernance.AllowedRecommendations

	if len(allowed) == 0 {
		return false, "Policy does not whitelist any recommendations (AllowedRecommendations is empty)"
	}

	for _, req := range allowed {
		if req == proposal.Spec.Recommendation {
			return true, ""
		}
	}

	return false, fmt.Sprintf("Recommendation '%s' is not in the policy allowed list", proposal.Spec.Recommendation)
}

// CooldownConstraint prevents rapid oscillatory optimizations.
type CooldownConstraint struct{}

func (c *CooldownConstraint) Name() string { return "CooldownEnforcement" }

func (c *CooldownConstraint) Evaluate(ctx context.Context, proposal *v1alpha1.OptimizationProposal, policy *v1alpha1.OptimizationPolicy, workloadContainers []string, lastOptimization *metav1.Time) (bool, string) {
	cooldown := 15 * time.Minute // default MVP cooldown
	if policy.Spec.SafetyConstraints.CooldownPeriod != nil {
		cooldown = policy.Spec.SafetyConstraints.CooldownPeriod.Duration
	}

	if lastOptimization != nil && !lastOptimization.IsZero() {
		elapsed := time.Since(lastOptimization.Time)
		if elapsed < cooldown {
			return false, fmt.Sprintf("Cooldown active: Only %v elapsed since last optimization, requires %v", elapsed.Round(time.Second), cooldown)
		}
	}

	return true, ""
}

// WorkloadProtectionConstraint skips optimization for critical sidecars.
type WorkloadProtectionConstraint struct{}

func (c *WorkloadProtectionConstraint) Name() string { return "WorkloadProtection" }

func (c *WorkloadProtectionConstraint) Evaluate(ctx context.Context, proposal *v1alpha1.OptimizationProposal, policy *v1alpha1.OptimizationPolicy, workloadContainers []string, lastOptimization *metav1.Time) (bool, string) {
	excluded := policy.Spec.WorkloadProtection.ExcludedContainers

	if len(excluded) == 0 || len(workloadContainers) == 0 {
		return true, ""
	}

	// Check if any workload container is in the excluded list
	for _, actual := range workloadContainers {
		for _, ex := range excluded {
			if actual == ex {
				return false, fmt.Sprintf("Workload contains protected sidecar/container: '%s'", ex)
			}
		}
	}

	return true, ""
}
