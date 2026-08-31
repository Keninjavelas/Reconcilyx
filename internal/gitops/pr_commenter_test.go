package gitops

import (
	"strings"
	"testing"
)

func TestShiftLeftPRCommenter(t *testing.T) {
	commenter := NewShiftLeftPRCommenter()

	diff := PRManifestDiff{
		WorkloadName:       "payment-service",
		Namespace:          "production",
		CurrentCPU:         "1000m",
		ProposedCPU:        "450m",
		CurrentMemory:      "2048Mi",
		ProposedMemory:     "1280Mi",
		EstimatedCostDelta: -142.50,
		EstimatedCarbonDelta: -18.4,
	}

	comment := commenter.EvaluatePRDiff(diff)

	if !strings.Contains(comment, "Reconcilyx Shift-Left FinOps") {
		t.Errorf("Comment header missing")
	}

	if !strings.Contains(comment, "-$142.50/mo") {
		t.Errorf("Cost savings formatting missing from comment:\n%s", comment)
	}

	if !strings.Contains(comment, "-18.4 kg CO₂e/mo") {
		t.Errorf("Carbon reduction formatting missing from comment:\n%s", comment)
	}
}
