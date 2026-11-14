package engine

import (
	"testing"
)

func TestNodeConsolidationAnalyzer(t *testing.T) {
	analyzer := NewNodeConsolidationAnalyzer(40.0)

	// Case 1: Under-utilized node (2 vCPU allocated out of 8 vCPU capacity = 25% utilization < 40%)
	prop := analyzer.EvaluateNode("ip-10-0-1-42.ec2.internal", 2.0, 8.0)
	if prop == nil {
		t.Fatalf("Expected NodeConsolidationProposal for underutilized node")
	}

	if prop.Action != "ConsolidateNode" {
		t.Errorf("Expected action ConsolidateNode, got %s", prop.Action)
	}

	if prop.CurrentUtilization != 25.0 {
		t.Errorf("Expected utilization 25.0%%, got %f", prop.CurrentUtilization)
	}

	// Case 2: Well-utilized node (7 vCPU allocated out of 8 vCPU capacity = 87.5% utilization > 40%)
	propHigh := analyzer.EvaluateNode("ip-10-0-1-43.ec2.internal", 7.0, 8.0)
	if propHigh != nil {
		t.Errorf("Expected no proposal for well-utilized node")
	}

	// Case 3: Evacuatable vs PDB-blocked node separates allocated vs realizable savings
	evacuatable := analyzer.EvaluateNodeWithConstraints(NodeConsolidationInput{
		NodeName:                  "ip-10-0-1-44.ec2.internal",
		CPUAllocatedCores:         1.0,
		CPUCapacityCores:          8.0,
		DaemonSetCPUOverheadCores: 0.5,
		MonthlyNodeCostUSD:        240.0,
	})
	if evacuatable == nil || !evacuatable.CanEvacuateWholeNode || evacuatable.RealizableNodeSavingsUSD != 240.0 {
		t.Fatalf("expected full $240 realizable savings when node can be evacuated, got %+v", evacuatable)
	}
	if evacuatable.RealizableCarbonKgCO2e <= 0 {
		t.Fatalf("expected positive realizable carbon reduction when whole node terminates")
	}

	pdbBlocked := analyzer.EvaluateNodeWithConstraints(NodeConsolidationInput{
		NodeName:          "ip-10-0-1-45.ec2.internal",
		CPUAllocatedCores: 1.0,
		CPUCapacityCores:  8.0,
		MonthlyNodeCostUSD: 240.0,
		PDBBlocksEviction: true,
		PDBBlockerReason:  "pdb default/critical-api allows 0 disruptions",
	})
	if pdbBlocked == nil || pdbBlocked.CanEvacuateWholeNode {
		t.Fatalf("expected PDB-blocked node not to be evacuatable")
	}
	if pdbBlocked.AllocatedSavingsUSD <= 0 {
		t.Fatalf("expected positive allocated headroom savings even when node evacuation is blocked")
	}
	if pdbBlocked.RealizableNodeSavingsUSD != 0 || pdbBlocked.RealizableCarbonKgCO2e != 0 {
		t.Fatalf("expected zero realizable savings when PDB blocks drain, got usd=%.2f carbon=%.2f", pdbBlocked.RealizableNodeSavingsUSD, pdbBlocked.RealizableCarbonKgCO2e)
	}
}
