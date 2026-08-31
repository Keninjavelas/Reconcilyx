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
}
