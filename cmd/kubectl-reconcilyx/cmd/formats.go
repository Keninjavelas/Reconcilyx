package cmd

import (
	"fmt"
	"io"
	"strings"
)

// --- SARIF v2.1.0 serializer ------------------------------------------------

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules,omitempty"`
}

type sarifRule struct {
	ID               string       `json:"id"`
	ShortDescription sarifMessage `json:"shortDescription"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	Level     string          `json:"level"`
	Message   sarifMessage    `json:"message"`
	Locations []sarifLocation `json:"locations,omitempty"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

func newSARIFLog(rules []sarifRule, results []sarifResult) sarifLog {
	if results == nil {
		results = []sarifResult{}
	}
	return sarifLog{
		Schema:  "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json",
		Version: "2.1.0",
		Runs: []sarifRun{{
			Tool: sarifTool{
				Driver: sarifDriver{
					Name:           "reconcilyx",
					Version:        "v4.1.0-rc1",
					InformationURI: "https://github.com/reconcilyx/reconcilyx",
					Rules:          rules,
				},
			},
			Results: results,
		}},
	}
}

func writeDiffSARIF(w io.Writer, report manifestDiffReport) error {
	rulesMap := map[string]sarifRule{}
	results := make([]sarifResult, 0, len(report.Violations)+len(report.Workloads))

	for _, v := range report.Violations {
		ruleID := "reconcilyx/" + v.Rule
		if _, ok := rulesMap[ruleID]; !ok {
			rulesMap[ruleID] = sarifRule{
				ID:               ruleID,
				ShortDescription: sarifMessage{Text: "Reconcilyx manifest policy guardrail: " + v.Rule},
			}
		}
		target := v.Workload
		if v.Container != "" {
			target += " [" + v.Container + "]"
		}
		uri := report.ManifestsPath
		if uri == "" {
			uri = "."
		}
		results = append(results, sarifResult{
			RuleID:  ruleID,
			Level:   "error",
			Message: sarifMessage{Text: fmt.Sprintf("%s: %s", target, v.Message)},
			Locations: []sarifLocation{{
				PhysicalLocation: sarifPhysicalLocation{ArtifactLocation: sarifArtifactLocation{URI: uri}},
			}},
		})
	}

	if report.Summary.MonthlyCostDeltaUSD > 0 || report.Summary.MonthlyCarbonDeltaGrams > 0 {
		ruleID := "reconcilyx/CostCarbonIncrease"
		rulesMap[ruleID] = sarifRule{
			ID:               ruleID,
			ShortDescription: sarifMessage{Text: "Workload manifest change increases estimated monthly cloud cost or carbon"},
		}
		uri := report.ManifestsPath
		if uri == "" {
			uri = "."
		}
		results = append(results, sarifResult{
			RuleID: ruleID,
			Level:  "warning",
			Message: sarifMessage{Text: fmt.Sprintf("Projected monthly delta: %s (%s CO2e)",
				formatSignedUSD(report.Summary.MonthlyCostDeltaUSD),
				formatSignedCarbon(report.Summary.MonthlyCarbonDeltaGrams))},
			Locations: []sarifLocation{{
				PhysicalLocation: sarifPhysicalLocation{ArtifactLocation: sarifArtifactLocation{URI: uri}},
			}},
		})
	}

	rules := make([]sarifRule, 0, len(rulesMap))
	for _, r := range rulesMap {
		rules = append(rules, r)
	}
	return writeJSONOutput(w, newSARIFLog(rules, results))
}

func writeScanSARIF(w io.Writer, report scanReport) error {
	rules := []sarifRule{{
		ID:               "reconcilyx/RightSizingOpportunity",
		ShortDescription: sarifMessage{Text: "Kubernetes workload right-sizing and carbon reduction proposal"},
	}}
	results := make([]sarifResult, 0, len(report.Items))
	for _, item := range report.Items {
		level := "note"
		if item.RiskScore >= 70 {
			level = "warning"
		}
		results = append(results, sarifResult{
			RuleID: "reconcilyx/RightSizingOpportunity",
			Level:  level,
			Message: sarifMessage{Text: fmt.Sprintf("%s/%s/%s (%s): est. $%.2f/mo savings, %.2f kg CO2e/mo abatement, risk=%d confidence=%d",
				item.Namespace, item.WorkloadKind, item.WorkloadName, item.Recommendation,
				item.EstimatedMonthlySavingsUSD, item.CarbonAbatedKg, item.RiskScore, item.ConfidenceScore)},
		})
	}
	return writeJSONOutput(w, newSARIFLog(rules, results))
}

func writeWhatIfSARIF(w io.Writer, req whatIfRequest, resp whatIfResponse) error {
	rules := []sarifRule{{
		ID:               "reconcilyx/WhatIfAssessment",
		ShortDescription: sarifMessage{Text: "What-if FinOps, carbon, and risk assessment"},
	}}
	level := "note"
	if !resp.Allowed || resp.RiskScore >= 70 || resp.CostDeltaUSDPerMonth > 0 {
		level = "warning"
	}
	if !resp.Allowed {
		level = "error"
	}
	results := []sarifResult{{
		RuleID: "reconcilyx/WhatIfAssessment",
		Level:  level,
		Message: sarifMessage{Text: fmt.Sprintf("%s/%s: allowed=%v risk=%d confidence=%d costDelta=$%.2f/mo (%s)",
			req.Namespace, req.WorkloadName, resp.Allowed, resp.RiskScore, resp.ConfidenceScore, resp.CostDeltaUSDPerMonth, resp.Rationale)},
	}}
	return writeJSONOutput(w, newSARIFLog(rules, results))
}

func writeExplainSARIF(w io.Writer, report explainReport) error {
	rules := []sarifRule{{
		ID:               "reconcilyx/AuditReceipt",
		ShortDescription: sarifMessage{Text: "Proposal audit receipt comparing predicted vs realized savings and carbon"},
	}}
	results := []sarifResult{{
		RuleID: "reconcilyx/AuditReceipt",
		Level:  "note",
		Message: sarifMessage{Text: fmt.Sprintf("Proposal %s (%s): phase=%s predicted=%s realized=%s",
			report.Name, report.Target.Name, report.Phase, impactUSD(report.Predicted.MonthlySavingsUSD), realizedCost(report.Realized))},
	}}
	return writeJSONOutput(w, newSARIFLog(rules, results))
}

// --- PR Markdown renderers --------------------------------------------------

func writeDiffMarkdown(w io.Writer, report manifestDiffReport, showAll bool) {
	s := report.Summary
	region := report.Region
	if region == "" {
		region = "global average"
	}
	modeTag := "live-git"
	if report.Offline {
		modeTag = "offline-catalog"
	}
	fmt.Fprintf(w, "### Reconcilyx FinOps & Carbon Manifest Diff (`%s`)\n\n", modeTag)
	fmt.Fprintf(w, "| Metric | Value |\n|---|---|\n")
	fmt.Fprintf(w, "| **Manifests / Base** | `%s` vs `%s` |\n", report.ManifestsPath, report.BaseRef)
	fmt.Fprintf(w, "| **Monthly Cost Delta** | **%s** (`%s` -> `%s`) |\n",
		formatSignedUSD(s.MonthlyCostDeltaUSD), formatUSD(s.MonthlyCostBeforeUSD), formatUSD(s.MonthlyCostAfterUSD))
	fmt.Fprintf(w, "| **Monthly Carbon Delta** | **%s** (`%s` -> `%s`, region `%s` @ %.0f gCO2e/kWh) |\n",
		formatSignedCarbon(s.MonthlyCarbonDeltaGrams), formatCarbon(s.MonthlyCarbonBeforeGrams), formatCarbon(s.MonthlyCarbonAfterGrams),
		region, report.CarbonIntensityGCO2PerKWh)
	fmt.Fprintf(w, "| **Guardrail Violations** | %d |\n", s.GuardrailViolations)
	fmt.Fprintf(w, "| **Provenance** | Pricing: `%s` · Carbon: `%s` |\n\n", report.PricingSource, report.CarbonSource)

	fmt.Fprintln(w, "<details>")
	fmt.Fprintf(w, "<summary>Workload Breakdown (%d changed, %d added, %d removed)</summary>\n\n", s.Changed, s.Added, s.Removed)
	fmt.Fprintln(w, "| Status | Workload | Replicas | CPU (Pod) | Memory (Pod) | Cost Delta | Carbon Delta |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|---|")
	rows := 0
	for _, wl := range report.Workloads {
		if !showAll && wl.Status == "unchanged" {
			continue
		}
		rows++
		fmt.Fprintf(w, "| `%s` | `%s %s/%s` | %s -> %s | %s -> %s | %s -> %s | %s | %s |\n",
			wl.Status, wl.Kind, wl.Namespace, wl.Name,
			sideReplicas(wl.Before), sideReplicas(wl.After),
			sideCPU(wl.Before), sideCPU(wl.After),
			sideMemory(wl.Before), sideMemory(wl.After),
			formatSignedUSD(wl.MonthlyCostDeltaUSD), formatSignedCarbon(wl.CarbonDeltaGrams))
	}
	if rows == 0 {
		fmt.Fprintln(w, "| `unchanged` | No workload resource changes detected | - | - | - | $0.00 | 0 g CO2e |")
	}
	fmt.Fprintln(w, "\n</details>")

	if len(report.Violations) > 0 {
		fmt.Fprintln(w, "\n#### Guardrail Violations")
		for _, v := range report.Violations {
			c := ""
			if v.Container != "" {
				c = " `" + v.Container + "`"
			}
			fmt.Fprintf(w, "- **%s** `%s`%s: %s\n", v.Rule, v.Workload, c, v.Message)
		}
	}
}

func writeScanMarkdown(w io.Writer, report scanReport) {
	s := report.Summary
	fmt.Fprintf(w, "### Reconcilyx Cluster Efficiency Scan (%d Proposals)\n\n", s.Total)
	fmt.Fprintf(w, "- **Net Opportunity:** `$%.2f/mo` · `%.2f kg CO2e/mo`\n", s.EstimatedMonthlySavingsUSD, s.CarbonAbatedKg)
	fmt.Fprintf(w, "- **Pending Approval:** %d · **In Flight:** %d · **Completed:** %d · **Failed:** %d\n\n",
		s.PendingApproval, s.InFlight, s.Completed, s.Failed)
	fmt.Fprintln(w, "<details open>")
	fmt.Fprint(w, "<summary>Proposal Table</summary>\n\n")
	fmt.Fprintln(w, "| Namespace | Workload | Action | State | Est. Savings | Carbon | Risk / Conf | Proposal |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|---|---|")
	for _, item := range report.Items {
		state := strings.Trim(strings.Join([]string{item.ApprovalState, item.Phase}, "/"), "/")
		fmt.Fprintf(w, "| `%s` | `%s/%s` | `%s` | `%s` | $%.2f/mo | %.2f kg/mo | %d / %d | `%s` |\n",
			item.Namespace, item.WorkloadKind, item.WorkloadName, item.Recommendation,
			dash(state), item.EstimatedMonthlySavingsUSD, item.CarbonAbatedKg,
			item.RiskScore, item.ConfidenceScore, item.Proposal)
	}
	fmt.Fprintln(w, "\n</details>")
}

func writeWhatIfMarkdown(w io.Writer, req whatIfRequest, resp whatIfResponse) {
	verdict := "ALLOWED"
	if !resp.Allowed {
		verdict = "BLOCKED"
	}
	fmt.Fprintf(w, "### Reconcilyx What-If Simulation (`%s`)\n\n", verdict)
	fmt.Fprintf(w, "| Field | Value |\n|---|---|\n")
	fmt.Fprintf(w, "| **Workload** | `%s/%s/%s` |\n", dash(req.WorkloadKind), dash(req.Namespace), dash(req.WorkloadName))
	fmt.Fprintf(w, "| **CPU** | `%s` -> `%s` (delta `%s`) |\n", req.CurrentCPU, req.ProposedCPU, resp.CPUDelta)
	fmt.Fprintf(w, "| **Memory** | `%s` -> `%s` (delta `%s`) |\n", req.CurrentMemory, req.ProposedMemory, resp.MemoryDelta)
	fmt.Fprintf(w, "| **Monthly Cost Delta** | %s (savings `$%s/mo`) |\n", formatSignedUSD(resp.CostDeltaUSDPerMonth), resp.SavingsUSDPerMonth)
	fmt.Fprintf(w, "| **Risk / Confidence** | %d / %d |\n", resp.RiskScore, resp.ConfidenceScore)
	if resp.Rationale != "" {
		fmt.Fprintf(w, "\n> %s\n", resp.Rationale)
	}
}

func writeExplainMarkdown(w io.Writer, report explainReport) {
	fmt.Fprintf(w, "### Reconcilyx ESG & FinOps Receipt: `%s`\n\n", report.Name)
	fmt.Fprintf(w, "| Metric | Predicted | Realized (%s) |\n|---|---|---|\n", dash(report.Realized.State))
	fmt.Fprintf(w, "| **Monthly Savings** | %s | %s |\n", impactUSD(report.Predicted.MonthlySavingsUSD), realizedCost(report.Realized))
	fmt.Fprintf(w, "| **Carbon Reduction** | %s | %s |\n", impactCarbon(report.Predicted.CarbonGrams), realizedCarbon(report.Realized))
	fmt.Fprintf(w, "| **Policy / Phase** | `%s` | `%s` |\n", dash(report.PolicyApplied), dash(report.Phase))
}
