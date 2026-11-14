package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiffFormatsAndOfflineMode(t *testing.T) {
	tmp := t.TempDir()
	baseDir := filepath.Join(tmp, "base")
	headDir := filepath.Join(tmp, "head")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		t.Fatalf("mkdir base: %v", err)
	}
	if err := os.MkdirAll(headDir, 0o755); err != nil {
		t.Fatalf("mkdir head: %v", err)
	}

	baseYAML := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: default
spec:
  replicas: 2
  template:
    spec:
      containers:
        - name: app
          image: nginx:1.25
          resources:
            requests:
              cpu: 250m
              memory: 256Mi
            limits:
              memory: 512Mi
`
	headYAML := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: default
spec:
  replicas: 2
  template:
    spec:
      containers:
        - name: app
          image: nginx:1.25
          resources:
            requests:
              cpu: 2000m
              memory: 4Gi
            limits:
              memory: 2Gi
`
	if err := os.WriteFile(filepath.Join(baseDir, "deploy.yaml"), []byte(baseYAML), 0o600); err != nil {
		t.Fatalf("write base: %v", err)
	}
	if err := os.WriteFile(filepath.Join(headDir, "deploy.yaml"), []byte(headYAML), 0o600); err != nil {
		t.Fatalf("write head: %v", err)
	}

	report, err := buildManifestDiffReportWithOptions(manifestDiffOptions{
		ManifestsPath:     headDir,
		BaseManifestsPath: baseDir,
		Region:            "aws-us-east-1",
		Offline:           true,
	})
	if err != nil {
		t.Fatalf("buildManifestDiffReportWithOptions: %v", err)
	}
	if !report.Offline {
		t.Fatalf("expected report.Offline=true")
	}
	if report.PricingSource != "bundled-catalog-fallback" {
		t.Fatalf("expected bundled-catalog-fallback pricing provenance, got %q", report.PricingSource)
	}
	if report.CarbonSource != "static-regional-fallback" {
		t.Fatalf("expected static-regional-fallback carbon provenance, got %q", report.CarbonSource)
	}
	if report.Summary.MonthlyCostDeltaUSD <= 0 {
		t.Fatalf("expected positive cost delta, got %v", report.Summary.MonthlyCostDeltaUSD)
	}
	if report.Summary.GuardrailViolations == 0 {
		t.Fatalf("expected memory-request-exceeds-limit guardrail violation")
	}

	var md bytes.Buffer
	writeDiffMarkdown(&md, report, false)
	mdStr := md.String()
	for _, want := range []string{
		"### Reconcilyx FinOps & Carbon Manifest Diff (`offline-catalog`)",
		"<details>",
		"<summary>Workload Breakdown",
		"**memory-request-exceeds-limit**",
		"bundled-catalog-fallback",
	} {
		if !strings.Contains(mdStr, want) {
			t.Fatalf("markdown output missing %q:\n%s", want, mdStr)
		}
	}

	var sarifBuf bytes.Buffer
	if err := writeDiffSARIF(&sarifBuf, report); err != nil {
		t.Fatalf("writeDiffSARIF: %v", err)
	}
	var parsed sarifLog
	if err := json.Unmarshal(sarifBuf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid SARIF JSON: %v", err)
	}
	if parsed.Version != "2.1.0" || len(parsed.Runs) != 1 {
		t.Fatalf("unexpected SARIF envelope: %+v", parsed)
	}
	if len(parsed.Runs[0].Results) < 2 {
		t.Fatalf("expected guardrail + cost/carbon SARIF results, got %d", len(parsed.Runs[0].Results))
	}

	rules, err := parseFailOn("cost-increase,guardrail")
	if err != nil {
		t.Fatalf("parseFailOn: %v", err)
	}
	err = enforceFailOn(rules, failOnMetrics{
		CostDeltaUSD:        report.Summary.MonthlyCostDeltaUSD,
		CarbonDeltaGrams:    report.Summary.MonthlyCarbonDeltaGrams,
		GuardrailViolations: report.Summary.GuardrailViolations,
	})
	var exitErr *exitCodeError
	if !errors.As(err, &exitErr) || exitErr.code != 2 {
		t.Fatalf("expected exit code 2 on policy gate failure, got %v", err)
	}
}

func TestScanWhatIfExplainFormats(t *testing.T) {
	scanRep := scanReport{
		Summary: scanSummary{Total: 1, PendingApproval: 1, EstimatedMonthlySavingsUSD: 18.25, CarbonAbatedKg: 11.10},
		Items: []scanItem{{
			Proposal:                   "opt-api",
			Namespace:                  "default",
			WorkloadKind:               "Deployment",
			WorkloadName:               "api",
			Recommendation:             "DownsizeCPU",
			ApprovalState:              "Pending",
			Phase:                      "Proposed",
			EstimatedMonthlySavingsUSD: 18.25,
			CarbonAbatedKg:             11.10,
			RiskScore:                  20,
			ConfidenceScore:            92,
		}},
	}
	var md, sarif bytes.Buffer
	writeScanMarkdown(&md, scanRep)
	if !strings.Contains(md.String(), "opt-api") {
		t.Fatalf("scan markdown missing proposal: %s", md.String())
	}
	if err := writeScanSARIF(&sarif, scanRep); err != nil {
		t.Fatalf("writeScanSARIF: %v", err)
	}
	if !strings.Contains(sarif.String(), `"version": "2.1.0"`) {
		t.Fatalf("scan SARIF missing 2.1.0 header")
	}

	md.Reset()
	sarif.Reset()
	req := whatIfRequest{Namespace: "default", WorkloadKind: "Deployment", WorkloadName: "api", CurrentCPU: "1000m", ProposedCPU: "400m"}
	resp := whatIfResponse{Allowed: false, RiskScore: 78, ConfidenceScore: 85, CPUDelta: "-600m", CostDeltaUSDPerMonth: -18.25, SavingsUSDPerMonth: "18.25", Rationale: "hpa conflict"}
	writeWhatIfMarkdown(&md, req, resp)
	if !strings.Contains(md.String(), "`BLOCKED`") {
		t.Fatalf("what-if markdown missing BLOCKED verdict: %s", md.String())
	}
	if err := writeWhatIfSARIF(&sarif, req, resp); err != nil {
		t.Fatalf("writeWhatIfSARIF: %v", err)
	}
	if !strings.Contains(sarif.String(), `"level": "error"`) {
		t.Fatalf("what-if SARIF expected error level for blocked response")
	}

	md.Reset()
	sarif.Reset()
	exp := explainReport{
		Name:          "opt-api",
		PolicyApplied: "prod-policy",
		Phase:         "Completed",
		Target:        explainTarget{Name: "api"},
		Predicted:     explainImpact{MonthlySavingsUSD: ptrFloat(18.25), CarbonGrams: ptrFloat(11097)},
		Realized:      explainRealized{State: "Verified", MonthlySavingsUSD: ptrFloat(18.25), CarbonGrams: ptrFloat(11097)},
	}
	writeExplainMarkdown(&md, exp)
	if !strings.Contains(md.String(), "Reconcilyx ESG & FinOps Receipt") {
		t.Fatalf("explain markdown missing receipt header: %s", md.String())
	}
	if err := writeExplainSARIF(&sarif, exp); err != nil {
		t.Fatalf("writeExplainSARIF: %v", err)
	}
	if !strings.Contains(sarif.String(), "reconcilyx/AuditReceipt") {
		t.Fatalf("explain SARIF missing AuditReceipt rule")
	}
}

func ptrFloat(v float64) *float64 { return &v }
