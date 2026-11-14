package engine

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// NodeConsolidationInput describes a node's capacity, DaemonSet reservation,
// scheduled workload demand, and eviction constraints for bin-packing analysis.
type NodeConsolidationInput struct {
	NodeName                     string  `json:"nodeName"`
	CPUAllocatedCores            float64 `json:"cpuAllocatedCores"`
	CPUCapacityCores             float64 `json:"cpuCapacityCores"`
	DaemonSetCPUOverheadCores    float64 `json:"daemonSetCpuOverheadCores,omitempty"`
	MemoryAllocatedGiB           float64 `json:"memoryAllocatedGiB,omitempty"`
	MemoryCapacityGiB            float64 `json:"memoryCapacityGiB,omitempty"`
	DaemonSetMemoryOverheadGiB   float64 `json:"daemonSetMemoryOverheadGiB,omitempty"`
	MonthlyNodeCostUSD           float64 `json:"monthlyNodeCostUSD,omitempty"`
	NodeCarbonGramsCO2PerMonth   float64 `json:"nodeCarbonGramsCO2PerMonth,omitempty"`
	PDBBlocksEviction            bool    `json:"pdbBlocksEviction,omitempty"`
	PDBBlockerReason             string  `json:"pdbBlockerReason,omitempty"`
	TopologySpreadBlocksEviction bool    `json:"topologySpreadBlocksEviction,omitempty"`
	TopologyBlockerReason        string  `json:"topologyBlockerReason,omitempty"`
}

// NodeConsolidationProposal separates container-level allocated request drops
// from whole-node realizable VM/carbon termination savings.
type NodeConsolidationProposal struct {
	ProposalID               string    `json:"proposalId"`
	NodeName                 string    `json:"nodeName"`
	CurrentUtilization       float64   `json:"currentUtilization"` // %
	AllocatableUtilization   float64   `json:"allocatableUtilization"`
	TargetKarpenterClaim     string    `json:"targetKarpenterClaim"`
	Action                   string    `json:"action"` // "ConsolidateNode" or "HoldNode"
	MonthlySavingsUSD        float64   `json:"monthlySavingsUSD"`
	AllocatedSavingsUSD      float64   `json:"allocatedSavingsUSD"`
	RealizableNodeSavingsUSD float64   `json:"realizableNodeSavingsUSD"`
	RealizableCarbonKgCO2e   float64   `json:"realizableCarbonKgCO2e"`
	CanEvacuateWholeNode     bool      `json:"canEvacuateWholeNode"`
	EvictionBlockers         []string  `json:"evictionBlockers,omitempty"`
	Timestamp                time.Time `json:"timestamp"`
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

// EvaluateNode preserves backward compatibility for simple CPU-only checks
// while delegating to the two-tier bin-packing evaluator.
func (n *NodeConsolidationAnalyzer) EvaluateNode(nodeName string, cpuAllocated, cpuCapacity float64) *NodeConsolidationProposal {
	return n.EvaluateNodeWithConstraints(NodeConsolidationInput{
		NodeName:          nodeName,
		CPUAllocatedCores: cpuAllocated,
		CPUCapacityCores:  cpuCapacity,
	})
}

// EvaluateNodeWithConstraints computes two-tier consolidation economics:
// 1. AllocatedSavingsUSD: virtual CPU/memory request headroom freed on the node.
// 2. RealizableNodeSavingsUSD + RealizableCarbonKgCO2e: savings only realized
//    when the full node can be evacuated (respecting DaemonSet overhead, PDBs,
//    and topologySpreadConstraints).
func (n *NodeConsolidationAnalyzer) EvaluateNodeWithConstraints(in NodeConsolidationInput) *NodeConsolidationProposal {
	if in.CPUCapacityCores <= 0 {
		return nil
	}

	rawUtilPct := (in.CPUAllocatedCores / in.CPUCapacityCores) * 100.0
	allocatableCPU := in.CPUCapacityCores - math.Max(0, in.DaemonSetCPUOverheadCores)
	if allocatableCPU <= 0 {
		allocatableCPU = in.CPUCapacityCores
	}
	allocatableUtilPct := (in.CPUAllocatedCores / allocatableCPU) * 100.0

	if rawUtilPct >= n.utilizationThreshold {
		return nil
	}

	// Allocated virtual savings: unutilized non-DaemonSet CPU converted to USD/mo.
	unusedCores := math.Max(0, allocatableCPU-in.CPUAllocatedCores)
	allocatedSavings := unusedCores * 1000.0 * usdPerMilliCPUMonth

	nodeMonthlyCost := in.MonthlyNodeCostUSD
	if nodeMonthlyCost <= 0 {
		nodeMonthlyCost = in.CPUCapacityCores * 1000.0 * usdPerMilliCPUMonth
	}
	nodeCarbonKg := in.NodeCarbonGramsCO2PerMonth / 1000.0
	if nodeCarbonKg <= 0 {
		// Standard default grid intensity baseline when node-specific telemetry isn't injected.
		nodeCarbonKg = (in.CPUCapacityCores * 1000.0 * kWhPerMilliCPUMonth * 367.0) / 1000.0
	}

	var blockers []string
	if in.PDBBlocksEviction {
		msg := strings.TrimSpace(in.PDBBlockerReason)
		if msg == "" {
			msg = "PodDisruptionBudget prevents draining remaining pods"
		}
		blockers = append(blockers, msg)
	}
	if in.TopologySpreadBlocksEviction {
		msg := strings.TrimSpace(in.TopologyBlockerReason)
		if msg == "" {
			msg = "topologySpreadConstraints block rescheduling onto peer nodes"
		}
		blockers = append(blockers, msg)
	}
	if in.MemoryCapacityGiB > 0 && in.DaemonSetMemoryOverheadGiB >= in.MemoryCapacityGiB*0.8 {
		blockers = append(blockers, "DaemonSet memory overhead exceeds 80% of node capacity")
	}

	canEvacuate := len(blockers) == 0
	realizableUSD := 0.0
	realizableCarbonKg := 0.0
	action := "HoldNode"
	if canEvacuate {
		realizableUSD = nodeMonthlyCost
		realizableCarbonKg = nodeCarbonKg
		action = "ConsolidateNode"
	}

	return &NodeConsolidationProposal{
		ProposalID:               fmt.Sprintf("node-prop-%s-%d", in.NodeName, time.Now().Unix()),
		NodeName:                 in.NodeName,
		CurrentUtilization:       rawUtilPct,
		AllocatableUtilization:   allocatableUtilPct,
		TargetKarpenterClaim:     fmt.Sprintf("karpenter-claim-%s", in.NodeName),
		Action:                   action,
		MonthlySavingsUSD:        realizableUSD,
		AllocatedSavingsUSD:      allocatedSavings,
		RealizableNodeSavingsUSD: realizableUSD,
		RealizableCarbonKgCO2e:   realizableCarbonKg,
		CanEvacuateWholeNode:     canEvacuate,
		EvictionBlockers:         blockers,
		Timestamp:                time.Now().UTC(),
	}
}
