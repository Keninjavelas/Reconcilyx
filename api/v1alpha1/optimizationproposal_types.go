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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RecommendationType identifies the class of recommendation.
type RecommendationType string

const (
	RecommendationRightSize           RecommendationType = "RightSize"
	RecommendationScaleZero           RecommendationType = "ScaleToZero"
	RecommendationReduceCPURequest    RecommendationType = "ReduceCPURequest"
	RecommendationReduceMemoryRequest RecommendationType = "ReduceMemoryRequest"
	RecommendationMigrateCloud        RecommendationType = "MigrateCloud"
)

// MigrationPlan specifies target cloud provider, region, instance type, and migration script.
type MigrationPlan struct {
	SourceCluster    string  `json:"sourceCluster,omitempty"`
	TargetCloud      string  `json:"targetCloud"`
	TargetRegion     string  `json:"targetRegion"`
	TargetInstance   string  `json:"targetInstance"`
	CostDeltaUSD     float64 `json:"costDeltaUSD"`
	CarbonSavedGrams float64 `json:"carbonSavedGrams"`
	MigrationScript  string  `json:"migrationScript,omitempty"`
}

// ApprovalState represents the state of approval for the proposal.
type ApprovalState string

const (
	ApprovalPending  ApprovalState = "Pending"
	ApprovalApproved ApprovalState = "Approved"
	ApprovalRejected ApprovalState = "Rejected"
)

// ExecutionState represents the operational state of the proposal.
type ExecutionState string

const (
	ExecutionPending     ExecutionState = "Pending"
	ExecutionApplying    ExecutionState = "Applying"
	ExecutionStabilizing ExecutionState = "Stabilizing"
	ExecutionVerified    ExecutionState = "Verified"
	ExecutionReverted    ExecutionState = "Reverted"
	ExecutionFailed      ExecutionState = "Failed"
)

// ResourceObservation captures the current baseline state.
type ResourceObservation struct {
	// Currently configured requests and limits.
	// +optional
	CurrentResources corev1.ResourceRequirements `json:"currentResources,omitempty"`
	// Observed P95 usage for CPU/Memory over a window.
	// +optional
	P95Usage corev1.ResourceList `json:"p95Usage,omitempty"`
}

// ExpectedImpact estimates the effect of the recommendation.
type ExpectedImpact struct {
	// Estimated reduction/increase in compute/memory limits.
	// +optional
	ResourceDelta corev1.ResourceList `json:"resourceDelta,omitempty"`
	// Estimated monthly savings in USD.
	// +optional
	EstimatedSavingsUSD *resource.Quantity `json:"estimatedSavingsUSD,omitempty"`
	// Estimated monthly CO2 reduction in grams.
	// +optional
	CarbonSavedGramsCO2 *resource.Quantity `json:"carbonSavedGramsCO2,omitempty"`
}

// DecisionMetadata tracks the reasoning behind the proposal.
type DecisionMetadata struct {
	// Confidence score (0-100) based on data volume and stability.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	ConfidenceScore int32 `json:"confidenceScore"`

	// Risk score (0-100) based on historical volatility.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	RiskScore int32 `json:"riskScore"`

	// Human-readable rationale for the proposal.
	// +optional
	Rationale string `json:"rationale,omitempty"`
}

// OptimizationProposalSpec defines the desired state of OptimizationProposal
type OptimizationProposalSpec struct {
	// TargetClusterName defines which spoke cluster should execute this proposal.
	TargetClusterName string `json:"targetClusterName,omitempty"`

	// TargetWorkload references the resource to be optimized
	TargetWorkload corev1.ObjectReference `json:"targetWorkload"`

	// Recommendation defines the optimization strategy
	// +kubebuilder:validation:Enum=RightSize;ScaleToZero
	Recommendation RecommendationType `json:"recommendation"`

	// ObservedResources contains the baseline usage and current limits
	// +optional
	ObservedResources ResourceObservation `json:"observedResources,omitempty"`

	// ProposedResources defines the new desired limits and requests
	// +optional
	ProposedResources corev1.ResourceRequirements `json:"proposedResources,omitempty"`

	// ExpectedImpact tracks estimated savings and deltas
	// +optional
	ExpectedImpact ExpectedImpact `json:"expectedImpact,omitempty"`

	// DecisionMetadata provides context and confidence for the decision
	// +optional
	DecisionMetadata DecisionMetadata `json:"decisionMetadata,omitempty"`

	// MigrationPlan specifies target cloud details and migration script for cross-cloud arbitrage
	// +optional
	MigrationPlan *MigrationPlan `json:"migrationPlan,omitempty"`
}

// OptimizationProposalPhase represents the current phase of the proposal lifecycle.
type OptimizationProposalPhase string

const (
	PhaseDetected                 OptimizationProposalPhase = "Detected"
	PhaseEvaluating               OptimizationProposalPhase = "Evaluating"
	PhaseApproved                 OptimizationProposalPhase = "Approved"
	PhaseAwaitingExternalApproval OptimizationProposalPhase = "AwaitingExternalApproval"
	PhaseRejected                 OptimizationProposalPhase = "Rejected"
	PhaseExpired                  OptimizationProposalPhase = "Expired"
	PhaseScheduled                OptimizationProposalPhase = "Scheduled"
	PhaseExecuting                OptimizationProposalPhase = "Executing"
	PhaseVerifying                OptimizationProposalPhase = "Verifying"
	PhaseStabilizing              OptimizationProposalPhase = "Stabilizing"
	PhaseCompleted                OptimizationProposalPhase = "Completed"
	PhaseFailed                   OptimizationProposalPhase = "Failed"
	PhaseRollbackRequired         OptimizationProposalPhase = "RollbackRequired"
	PhaseRollingBack              OptimizationProposalPhase = "RollingBack"
	PhaseRollbackVerification     OptimizationProposalPhase = "RollbackVerification"
	PhaseRolledBack               OptimizationProposalPhase = "RolledBack"
)

// ContainerResourceState holds the exact resource state for a single container.
type ContainerResourceState struct {
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// ExecutionSnapshot captures the exact state of the target workload 1 millisecond before actuation.
type ExecutionSnapshot struct {
	// ResourceVersion at the time of snapshot
	ResourceVersion string `json:"resourceVersion"`
	// Generation at the time of snapshot
	Generation int64 `json:"generation"`
	// Exact resources per container (keyed by container name)
	ContainerResources map[string]ContainerResourceState `json:"containerResources"`
	// Expected Replica count
	ReplicaCount int32 `json:"replicaCount"`
	// Annotations present on the workload
	Annotations map[string]string `json:"annotations,omitempty"`
	// Timestamp of the snapshot
	Timestamp metav1.Time `json:"timestamp"`
}

// ExecutionMetadata tracks the context of an actuation attempt.
type ExecutionMetadata struct {
	// Unique ID for this execution run
	ExecutionID string `json:"executionID"`
	// Field manager used in Server-Side Apply
	FieldManager string `json:"fieldManager"`
	// ResourceVersion of the Policy at the time of evaluation
	PolicyVersion string `json:"policyVersion"`
	// ResourceVersion of the Proposal at the time of execution
	ProposalVersion string `json:"proposalVersion"`
	// Timestamp of the JIT telemetry refresh
	TelemetryTimestamp metav1.Time `json:"telemetryTimestamp"`
}

// OptimizationProposalStatus defines the observed state of OptimizationProposal.
type OptimizationProposalStatus struct {
	// Phase represents the current stage of the proposal lifecycle
	// +optional
	Phase OptimizationProposalPhase `json:"phase,omitempty"`

	// ApprovalState represents the human/policy approval status
	// +optional
	ApprovalState ApprovalState `json:"approvalState,omitempty"`

	// ExecutionState represents the controller execution status
	// +optional
	ExecutionState ExecutionState `json:"executionState,omitempty"`

	// PolicyApplied tracks which policy governed the decision
	// +optional
	PolicyApplied string `json:"policyApplied,omitempty"`

	// LastEvaluationTime is the timestamp of the last policy evaluation
	// +optional
	LastEvaluationTime *metav1.Time `json:"lastEvaluationTime,omitempty"`

	// ExpirationTime defines when this proposal naturally expires
	// +optional
	ExpirationTime *metav1.Time `json:"expirationTime,omitempty"`

	// Reason explains the rationale behind the approval or rejection
	// +optional
	Reason string `json:"reason,omitempty"`

	// ExecutionSnapshot captures the target state before mutation for guaranteed safe rollback
	// +optional
	ExecutionSnapshot *ExecutionSnapshot `json:"executionSnapshot,omitempty"`

	// ExecutionMetadata records the audit trail of the execution context
	// +optional
	ExecutionMetadata *ExecutionMetadata `json:"executionMetadata,omitempty"`

	// Conditions represent the current state of the OptimizationProposal resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// YieldCount tracks how many times the execution was reverted by external controllers.
	// +optional
	YieldCount int32 `json:"yieldCount,omitempty"`

	// ExternalPRID is the ID of an external GitOps pull/merge request created for this proposal.
	// +optional
	ExternalPRID string `json:"externalPRID,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Target-Kind",type=string,JSONPath=`.spec.targetWorkload.kind`
// +kubebuilder:printcolumn:name="Target-Name",type=string,JSONPath=`.spec.targetWorkload.name`
// +kubebuilder:printcolumn:name="Recommendation",type=string,JSONPath=`.spec.recommendation`
// +kubebuilder:printcolumn:name="Approval",type=string,JSONPath=`.status.approvalState`
// +kubebuilder:printcolumn:name="Execution",type=string,JSONPath=`.status.executionState`

// OptimizationProposal is the Schema for the optimizationproposals API
type OptimizationProposal struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of OptimizationProposal
	// +required
	Spec OptimizationProposalSpec `json:"spec"`

	// status defines the observed state of OptimizationProposal
	// +optional
	Status OptimizationProposalStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// OptimizationProposalList contains a list of OptimizationProposal
type OptimizationProposalList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []OptimizationProposal `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &OptimizationProposal{}, &OptimizationProposalList{})
		return nil
	})
}
