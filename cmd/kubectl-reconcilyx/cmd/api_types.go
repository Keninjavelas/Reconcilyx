package cmd

import (
	"encoding/json"
	"time"
)

type proposalListItem struct {
	Name             string    `json:"name"`
	Namespace        string    `json:"namespace"`
	Cluster          string    `json:"cluster,omitempty"`
	WorkloadKind     string    `json:"workloadKind"`
	WorkloadName     string    `json:"workloadName"`
	Recommendation   string    `json:"recommendation"`
	ApprovalState    string    `json:"approvalState"`
	Phase            string    `json:"phase"`
	ExecutionState   string    `json:"executionState,omitempty"`
	RiskScore        int32     `json:"riskScore"`
	ConfidenceScore  int32     `json:"confidenceScore"`
	Rationale        string    `json:"rationale,omitempty"`
	EstimatedSavings string    `json:"estimatedSavings,omitempty"`
	CarbonSavedGrams int64     `json:"carbonSavedGrams,omitempty"`
	CarbonSavedHuman string    `json:"carbonSavedHuman,omitempty"`
	CPUBefore        string    `json:"cpuBefore,omitempty"`
	CPUAfter         string    `json:"cpuAfter,omitempty"`
	MemoryBefore     string    `json:"memoryBefore,omitempty"`
	MemoryAfter      string    `json:"memoryAfter,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	ExternalPRID     string    `json:"externalPRID,omitempty"`
}

type proposalListResponse struct {
	Items []proposalListItem `json:"items"`
	Total int                `json:"total"`
}

type apiEvent struct {
	Type           string     `json:"type,omitempty"`
	Reason         string     `json:"reason,omitempty"`
	Message        string     `json:"message,omitempty"`
	Count          int32      `json:"count,omitempty"`
	FirstTimestamp *time.Time `json:"firstTimestamp,omitempty"`
	LastTimestamp  *time.Time `json:"lastTimestamp,omitempty"`
	EventTime      *time.Time `json:"eventTime,omitempty"`
}

type proposalDetailResponse struct {
	Proposal json.RawMessage `json:"proposal"`
	Events   []apiEvent      `json:"events"`
}

func (e apiEvent) timestamp() *time.Time {
	if e.LastTimestamp != nil {
		return e.LastTimestamp
	}
	if e.EventTime != nil {
		return e.EventTime
	}
	return e.FirstTimestamp
}
