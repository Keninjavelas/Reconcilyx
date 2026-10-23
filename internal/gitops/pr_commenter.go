package gitops

import (
	"fmt"
	"strings"
)

type PRManifestDiff struct {
	WorkloadName         string
	Namespace            string
	CurrentCPU           string
	ProposedCPU          string
	CurrentMemory        string
	ProposedMemory       string
	EstimatedCostDelta   float64 // $/month (negative means savings)
	EstimatedCarbonDelta float64 // kg CO2e/month
}

type ShiftLeftPRCommenter struct{}

func NewShiftLeftPRCommenter() *ShiftLeftPRCommenter {
	return &ShiftLeftPRCommenter{}
}

func (s *ShiftLeftPRCommenter) EvaluatePRDiff(diff PRManifestDiff) string {
	var sb strings.Builder

	sb.WriteString("### 🤖 Reconcilyx Shift-Left FinOps & Carbon Audit\n\n")
	sb.WriteString(fmt.Sprintf("Audited manifest diff for workload **`%s`** in namespace **`%s`**:\n\n", diff.WorkloadName, diff.Namespace))

	sb.WriteString("| Resource Axis | Current Manifest | Proposed Manifest | Projected Impact |\n")
	sb.WriteString("|---------------|------------------|-------------------|------------------|\n")
	sb.WriteString(fmt.Sprintf("| **CPU** | `%s` | `%s` | |\n", diff.CurrentCPU, diff.ProposedCPU))
	sb.WriteString(fmt.Sprintf("| **Memory** | `%s` | `%s` | |\n", diff.CurrentMemory, diff.ProposedMemory))

	if diff.EstimatedCostDelta < 0 {
		sb.WriteString(fmt.Sprintf("| **Cost ($/mo)** | | | 🟢 **-$%.2f/mo** (Savings) |\n", -diff.EstimatedCostDelta))
	} else if diff.EstimatedCostDelta > 0 {
		sb.WriteString(fmt.Sprintf("| **Cost ($/mo)** | | | ⚠️ **+$%.2f/mo** (Cost Increase) |\n", diff.EstimatedCostDelta))
	} else {
		sb.WriteString("| **Cost ($/mo)** | | | ⚪ **$0.00/mo** (Neutral) |\n")
	}

	if diff.EstimatedCarbonDelta < 0 {
		sb.WriteString(fmt.Sprintf("| **Carbon Footprint** | | | 🍃 **-%.1f kg CO₂e/mo** (Reduction) |\n\n", -diff.EstimatedCarbonDelta))
	} else if diff.EstimatedCarbonDelta > 0 {
		sb.WriteString(fmt.Sprintf("| **Carbon Footprint** | | | ⚡ **+%.1f kg CO₂e/mo** (Increase) |\n\n", diff.EstimatedCarbonDelta))
	}

	if diff.EstimatedCostDelta > 20.0 {
		sb.WriteString("> ⚠️ **Recommendation**: This change increases cloud spend significantly. Consider reducing CPU/Memory requests prior to merging.")
	} else {
		sb.WriteString("> ✅ **Passed Policy Check**: Manifest updates meet cluster efficiency standards.")
	}

	return sb.String()
}
