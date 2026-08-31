package engine

import (
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"time"
)

// NOTE (v5.0 planned): The NodeConsolidationAnalyzer is currently a STUB.
// Real node-level optimization (Karpenter consolidation actuation, eviction
// safety gates, Cluster-Autoscaler interaction) is scheduled for
// Reconcilyx v5.0. For v4.0-beta, Reconcilyx only right-sizes workload
// containers via In-Place Pod Resizing. Callers that want to opt in to
// the real consolidation path once it lands can set:
// `RECONCILYX_V5_NODE_CONSOLIDATION_ENABLED=1`.
var ncStubWarned atomic.Int32

func ncWarnOnce(node string) {
	if ncStubWarned.Add(1) == 1 {
		l := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
		l.Warn(fmt.Sprintf("[node-consolidation v5.0 stub] EvaluateNode(%s) returning fixed heuristic; real Karpenter/eviction actuation and live node utilization are scheduled for v5.0. Set RECONCILYX_V5_NODE_CONSOLIDATION_ENABLED=1 to opt in once available.", node))
	}
}

type NodeConsolidationProposal struct {
	ProposalID          string    `json:"proposalId"`
	NodeName            string    `json:"nodeName"`
	CurrentUtilization  float64   `json:"currentUtilization"` // %
	TargetKarpenterClaim string    `json:"targetKarpenterClaim"`
	Action              string    `json:"action"` // "ConsolidateNode" or "DeleteNode"
	MonthlySavingsUSD   float64   `json:"monthlySavingsUSD"`
	Timestamp           time.Time `json:"timestamp"`
}

type NodeConsolidationAnalyzer struct {
	utilizationThreshold float64
}

func NewNodeConsolidationAnalyzer(threshold float64) *NodeConsolidationAnalyzer {
	if threshold <= 0 {
		threshold = 40.0 // Default 40% node utilization threshold
	}
	return &NodeConsolidationAnalyzer{
		utilizationThreshold: threshold,
	}
}

func (n *NodeConsolidationAnalyzer) EvaluateNode(nodeName string, cpuAllocated, cpuCapacity float64) *NodeConsolidationProposal {
	ncWarnOnce(nodeName)
	if cpuCapacity <= 0 {
		return nil
	}

	utilPct := (cpuAllocated / cpuCapacity) * 100.0

	if utilPct < n.utilizationThreshold {
		return &NodeConsolidationProposal{
			ProposalID:           fmt.Sprintf("node-prop-%s-%d", nodeName, time.Now().Unix()),
			NodeName:             nodeName,
			CurrentUtilization:   utilPct,
			TargetKarpenterClaim: fmt.Sprintf("karpenter-claim-%s", nodeName),
			Action:               "ConsolidateNode",
			MonthlySavingsUSD:    185.00, // Fixed heuristic (v5.0 will use live node pricing)
			Timestamp:            time.Now().UTC(),
		}
	}

	return nil
}
