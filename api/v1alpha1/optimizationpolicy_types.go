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

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// OptimizationObjective defines the high-level goal of the policy.
type OptimizationObjective string

const (
	ObjectiveCost        OptimizationObjective = "Cost"
	ObjectivePerformance OptimizationObjective = "Performance"
	ObjectiveEfficiency  OptimizationObjective = "Efficiency"
)

// PolicyMode defines how aggressively the policy should actuate.
type PolicyMode string

const (
	ModeObserve   PolicyMode = "Observe"
	ModePropose   PolicyMode = "Propose"
	ModeAutoApply PolicyMode = "AutoApply"
	// ModeDryRun evaluates the proposal and records what the hypothetical
	// DryRunMode would decide without changing the proposal's lifecycle.
	ModeDryRun PolicyMode = "DryRun"
)

// TargetSelectors defines which workloads the policy applies to.
type TargetSelectors struct {
	// Selects namespaces based on labels. If empty, all namespaces are considered (unless overridden).
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`

	// Selects specific workloads within the target namespaces based on labels.
	// +optional
	WorkloadSelector *metav1.LabelSelector `json:"workloadSelector,omitempty"`
}

// ResourceConstraints defines boundaries for optimization.
type ResourceConstraints struct {
	// Minimum CPU requests/limits allowed.
	// +optional
	MinCPU *resource.Quantity `json:"minCPU,omitempty"`
	// Maximum CPU requests/limits allowed.
	// +optional
	MaxCPU *resource.Quantity `json:"maxCPU,omitempty"`
	// Minimum Memory requests/limits allowed.
	// +optional
	MinMemory *resource.Quantity `json:"minMemory,omitempty"`
	// Maximum Memory requests/limits allowed.
	// +optional
	MaxMemory *resource.Quantity `json:"maxMemory,omitempty"`
}

// SafetyConstraints defines availability and latency guardrails.
type SafetyConstraints struct {
	// Minimum number of replicas required to remain active.
	// +kubebuilder:validation:Minimum=1
	// +optional
	MinReplicas *int32 `json:"minReplicas,omitempty"`

	// Maximum allowable latency increase in percentage or milliseconds.
	// +kubebuilder:validation:Minimum=0
	// +optional
	MaxLatencyIncreaseMs *int32 `json:"maxLatencyIncreaseMs,omitempty"`

	// Cooldown duration between optimizations for a specific workload.
	// +kubebuilder:default="15m"
	// +optional
	CooldownPeriod *metav1.Duration `json:"cooldownPeriod,omitempty"`

	// RiskTolerance defines the maximum allowable risk score (0-100).
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +optional
	RiskTolerance *int32 `json:"riskTolerance,omitempty"`

	// ConfidenceRequirement defines the minimum required confidence score (0-100).
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +optional
	ConfidenceRequirement *int32 `json:"confidenceRequirement,omitempty"`

	// CarbonWeight weights multi-objective RL reward between cost (0.0) and carbon (1.0).
	// Cost-only (default) = 0.0, 50/50 = 0.5, carbon-only = 1.0.
	// +kubebuilder:validation:Minimum=0.0
	// +kubebuilder:validation:Maximum=1.0
	// +optional
	CarbonWeight *float64 `json:"carbonWeight,omitempty"`
}

// RecommendationGovernance defines allowed recommendation actions.
type RecommendationGovernance struct {
	// AllowedRecommendations explicitly whitelists which recommendations can be applied.
	// +optional
	AllowedRecommendations []RecommendationType `json:"allowedRecommendations,omitempty"`
}

// WorkloadProtection defines exclusions to protect specific targets.
type WorkloadProtection struct {
	// ExcludedContainers skips optimization if these containers are present (e.g., istio-proxy).
	// +optional
	ExcludedContainers []string `json:"excludedContainers,omitempty"`
}

// CanaryLabelKey marks a workload's pods as a canary scope for blast-radius policy.
const CanaryLabelKey = "reconcilyx.io/canary"

// ChangeBudget limits how many changes may be applied to a workload in a rolling window.
type ChangeBudget struct {
	// MaxChangesPerWindow caps executed changes per target workload within Window.
	// A value of 0 disables the budget.
	// +kubebuilder:validation:Minimum=0
	// +optional
	MaxChangesPerWindow int32 `json:"maxChangesPerWindow,omitempty"`

	// Window is the rolling period used for MaxChangesPerWindow. Defaults to 24h.
	// +kubebuilder:default="24h"
	// +optional
	Window *metav1.Duration `json:"window,omitempty"`
}

// BlastRadius limits the size of the target a single change may affect.
type BlastRadius struct {
	// MaxReplicasPerChange rejects changes to workloads with more replicas than this.
	// A value of 0 disables the check; DaemonSets cannot be capped and are rejected
	// when the check is enabled.
	// +kubebuilder:validation:Minimum=0
	// +optional
	MaxReplicasPerChange int32 `json:"maxReplicasPerChange,omitempty"`

	// RequireCanaryLabel requires the target workload or its pod template to carry
	// reconcilyx.io/canary=true before a change is allowed.
	// +optional
	RequireCanaryLabel bool `json:"requireCanaryLabel,omitempty"`
}

// RollbackConstraints defines conditions under which a rollback should occur.
type RollbackConstraints struct {
	// Automatically rollback if error rate spikes beyond this threshold.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +optional
	ErrorRateThresholdPercent *int32 `json:"errorRateThresholdPercent,omitempty"`
	// Automatically rollback if OOMKilled events are detected within this duration.
	// +optional
	OOMKilledWindow *metav1.Duration `json:"oomKilledWindow,omitempty"`
}

// GitProviderType defines the supported Git hosting providers.
type GitProviderType string

const (
	GitProviderMock   GitProviderType = "mock"
	GitProviderGitHub GitProviderType = "github"
	GitProviderGitLab GitProviderType = "gitlab"
)

// GitOpsConfig configures the GitOps workflow for a policy.
type GitOpsConfig struct {
	// Enabled toggles GitOps mode. When true, approved proposals generate
	// Pull Requests instead of live-patching the cluster.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Provider selects the Git hosting provider (mock, github, gitlab).
	// +kubebuilder:validation:Enum=mock;github;gitlab
	// +kubebuilder:default=mock
	// +optional
	Provider GitProviderType `json:"provider,omitempty"`

	// Owner is the repository owner (GitHub org/user, GitLab namespace).
	// +optional
	Owner string `json:"owner,omitempty"`

	// Repo is the repository name where manifests are stored.
	// +optional
	Repo string `json:"repo,omitempty"`

	// BaseBranch is the target branch for PRs (default: main).
	// +kubebuilder:default=main
	// +optional
	BaseBranch string `json:"baseBranch,omitempty"`

	// ManifestsPath is the directory within the repo containing manifests.
	// +optional
	ManifestsPath string `json:"manifestsPath,omitempty"`

	// AutoMerge controls whether PRs are merged automatically when CI passes.
	// +optional
	AutoMerge bool `json:"autoMerge,omitempty"`
}

// OptimizationPolicySpec defines the desired state of OptimizationPolicy
type OptimizationPolicySpec struct {
	// Objective defines the primary goal (e.g., Cost, Performance, Efficiency)
	// +kubebuilder:validation:Enum=Cost;Performance;Efficiency
	// +kubebuilder:default=Efficiency
	Objective OptimizationObjective `json:"objective"`

	// Mode determines if proposals should be observed, proposed, or auto-applied
	// +kubebuilder:validation:Enum=Observe;Propose;AutoApply;DryRun
	// +kubebuilder:default=Propose
	Mode PolicyMode `json:"mode"`

	// DryRunMode is the hypothetical mode reported when Mode is DryRun.
	// The proposal's lifecycle is not advanced; status.dryRun records what
	// this mode would have decided. Defaults to Propose.
	// +kubebuilder:validation:Enum=Observe;Propose;AutoApply
	// +kubebuilder:default=Propose
	// +optional
	DryRunMode PolicyMode `json:"dryRunMode,omitempty"`

	// TargetSelectors defines which workloads the policy applies to
	// +optional
	TargetSelectors TargetSelectors `json:"targetSelectors,omitempty"`

	// ResourceConstraints defines boundaries for optimization
	// +optional
	ResourceConstraints ResourceConstraints `json:"resourceConstraints,omitempty"`

	// SafetyConstraints defines availability and latency guardrails
	// +optional
	SafetyConstraints SafetyConstraints `json:"safetyConstraints,omitempty"`

	// RollbackConstraints defines conditions under which a rollback should occur
	// +optional
	RollbackConstraints RollbackConstraints `json:"rollbackConstraints,omitempty"`

	// RecommendationGovernance defines explicitly allowed recommendation types
	// +optional
	RecommendationGovernance RecommendationGovernance `json:"recommendationGovernance,omitempty"`

	// WorkloadProtection defines exclusions to protect specific targets
	// +optional
	WorkloadProtection WorkloadProtection `json:"workloadProtection,omitempty"`

	// ChangeBudget limits how many changes may be applied to a workload in a rolling window.
	// +optional
	ChangeBudget ChangeBudget `json:"changeBudget,omitempty"`

	// BlastRadius limits the size of the target a single change may affect.
	// +optional
	BlastRadius BlastRadius `json:"blastRadius,omitempty"`

	// GitOps configures Pull Request based actuation.
	// +optional
	GitOps GitOpsConfig `json:"gitOps,omitempty"`
}

// OptimizationPolicyStatus defines the observed state of OptimizationPolicy.
type OptimizationPolicyStatus struct {
	// Conditions represent the current state of the OptimizationPolicy resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Objective",type=string,JSONPath=`.spec.objective`
// +kubebuilder:printcolumn:name="Mode",type=string,JSONPath=`.spec.mode`

// OptimizationPolicy is the Schema for the optimizationpolicies API
type OptimizationPolicy struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of OptimizationPolicy
	// +required
	Spec OptimizationPolicySpec `json:"spec"`

	// status defines the observed state of OptimizationPolicy
	// +optional
	Status OptimizationPolicyStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// OptimizationPolicyList contains a list of OptimizationPolicy
type OptimizationPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []OptimizationPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &OptimizationPolicy{}, &OptimizationPolicyList{})
		return nil
	})
}
