package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

const explainFixtureBody = `{
  "proposal": {
    "metadata": {"name": "p1", "namespace": "prod", "creationTimestamp": "2026-09-30T08:00:00Z"},
    "spec": {
      "targetClusterName": "east",
      "targetWorkload": {"apiVersion": "apps/v1", "kind": "Deployment", "namespace": "prod", "name": "web"},
      "recommendation": "RightSize",
      "observedResources": {"currentResources": {"requests": {"cpu": "2", "memory": "2Gi"}}},
      "proposedResources": {"requests": {"cpu": "1", "memory": "1Gi"}},
      "expectedImpact": {"estimatedSavingsUSD": "10", "carbonSavedGramsCO2": "2500"},
      "decisionMetadata": {"confidenceScore": 88, "riskScore": 12, "rationale": "P95 usage is well below requests"}
    },
    "status": {
      "phase": "Completed",
      "approvalState": "Approved",
      "executionState": "Completed",
      "policyApplied": "prod-policy",
      "executionSnapshot": {"replicaCount": 3, "timestamp": "2026-10-01T09:00:00Z"},
      "learning": {
        "finalized": true,
        "outcome": "Completed",
        "reward": 0.82,
        "completedAt": "2026-10-01T10:00:00Z",
        "results": {
          "web": {"cpuAdjustmentPercent": -50, "memoryAdjustmentPercent": -50, "costSavedUSD": "10", "carbonSavedGramsCO2": "2500", "reward": 0.82},
          "sidecar": {"cpuAdjustmentPercent": -25, "memoryAdjustmentPercent": 0, "costSavedUSD": "2.5", "carbonSavedGramsCO2": "500", "reward": 0.6}
        }
      }
    }
  },
  "events": [
    {"type": "Normal", "reason": "Validated", "message": "Execution gate passed", "lastTimestamp": "2026-10-01T09:05:00Z"},
    {"type": "Normal", "reason": "Created", "message": "Proposal created", "lastTimestamp": "2026-09-30T08:01:00Z"}
  ]
}`

func TestBuildExplainReportReconcilesReceipts(t *testing.T) {
	report, err := buildExplainReport([]byte(explainFixtureBody))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Name != "p1" || report.Namespace != "prod" || report.Target.Name != "web" || report.Target.Kind != "Deployment" {
		t.Fatalf("proposal identity not mapped: %+v", report)
	}
	if report.PolicyApplied != "prod-policy" || report.Phase != "Completed" || report.ConfidenceScore != 88 || report.RiskScore != 12 {
		t.Fatalf("lifecycle fields not mapped: %+v", report)
	}
	if report.Resources.CPUBefore != "2" || report.Resources.CPUAfter != "1" || report.Resources.CPUDelta != "1" {
		t.Fatalf("cpu resources wrong: %+v", report.Resources)
	}
	if report.Resources.MemoryDelta != "1Gi" {
		t.Fatalf("memory delta = %q, want 1Gi", report.Resources.MemoryDelta)
	}
	if report.Predicted.MonthlySavingsUSD == nil || *report.Predicted.MonthlySavingsUSD != 10 {
		t.Fatalf("predicted savings wrong: %+v", report.Predicted)
	}
	if report.Predicted.CarbonKg == nil || *report.Predicted.CarbonKg != 2.5 {
		t.Fatalf("predicted carbon wrong: %+v", report.Predicted)
	}

	realized := report.Realized
	if realized.State != "finalized" || realized.Outcome != "Completed" {
		t.Fatalf("receipt not finalized: %+v", realized)
	}
	if realized.MonthlySavingsUSD == nil || *realized.MonthlySavingsUSD != 12.5 {
		t.Fatalf("realized savings should sum containers ($12.50): %+v", realized)
	}
	if realized.CarbonGrams == nil || *realized.CarbonGrams != 3000 || realized.CarbonKg == nil || *realized.CarbonKg != 3 {
		t.Fatalf("realized carbon should sum containers (3000 g): %+v", realized)
	}
	if realized.Reward == nil || *realized.Reward != 0.82 {
		t.Fatalf("reward not mapped: %+v", realized)
	}
	if len(realized.Containers) != 2 || realized.Containers[0].Name != "sidecar" || realized.Containers[1].Name != "web" {
		t.Fatalf("container receipts should be name-sorted: %+v", realized.Containers)
	}

	if report.Variance.MonthlySavingsUSD == nil || *report.Variance.MonthlySavingsUSD != 2.5 {
		t.Fatalf("savings variance wrong: %+v", report.Variance)
	}
	if report.Variance.MonthlySavingsChange == nil || *report.Variance.MonthlySavingsChange != 25 {
		t.Fatalf("savings variance percent wrong: %+v", report.Variance)
	}
	if report.Variance.CarbonGrams == nil || *report.Variance.CarbonGrams != 500 {
		t.Fatalf("carbon variance wrong: %+v", report.Variance)
	}
	if report.Snapshot == nil || report.Snapshot.Replicas != 3 {
		t.Fatalf("snapshot not mapped: %+v", report.Snapshot)
	}

	if len(report.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(report.Events))
	}
	if report.Events[0].Reason != "Created" || report.Events[1].Reason != "Validated" {
		t.Fatalf("events should be chronological, got %+v", report.Events)
	}
}

func TestBuildExplainReportPendingReceipt(t *testing.T) {
	body := strings.Replace(explainFixtureBody, `"finalized": true`, `"finalized": false`, 1)
	report, err := buildExplainReport([]byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Realized.State != "pending" {
		t.Fatalf("expected pending receipt, got %+v", report.Realized)
	}
	if report.Realized.MonthlySavingsUSD != nil {
		t.Fatalf("pending receipt must not claim realized savings: %+v", report.Realized)
	}
	if report.Variance.MonthlySavingsUSD != nil {
		t.Fatalf("pending receipt must not compute variance: %+v", report.Variance)
	}
}

func TestBuildExplainReportRejectsMalformedBody(t *testing.T) {
	for name, body := range map[string]string{
		"not json":       `{`,
		"missing object": `{"events":[]}`,
		"bad proposal":   `{"proposal": {"spec": "not-an-object"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := buildExplainReport([]byte(body)); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestExplainCommandJSON(t *testing.T) {
	withAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/proposals/p1" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(explainFixtureBody))
	})

	namespace = "prod"
	explainCmd.SetOut(nil)
	t.Cleanup(func() {
		namespace = ""
		_ = explainCmd.Flags().Set("output", "table")
		explainCmd.SetOut(nil)
	})
	if err := explainCmd.Flags().Set("output", "json"); err != nil {
		t.Fatalf("set output: %v", err)
	}

	var out bytes.Buffer
	explainCmd.SetOut(&out)

	if err := explainCmd.RunE(explainCmd, []string{"p1"}); err != nil {
		t.Fatalf("explain failed: %v", err)
	}
	var report explainReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("decode explain JSON: %v\n%s", err, out.String())
	}
	if report.Name != "p1" || report.Realized.MonthlySavingsUSD == nil || *report.Realized.MonthlySavingsUSD != 12.5 {
		t.Fatalf("unexpected explain report: %+v", report)
	}
}

func TestResolveProposalNamespace(t *testing.T) {
	t.Run("ambiguous", func(t *testing.T) {
		withAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"name": "p1", "namespace": "prod"},
					{"name": "p1", "namespace": "staging"},
				},
				"total": 2,
			})
		})
		namespace = ""
		t.Cleanup(func() { namespace = "" })

		_, err := resolveProposalNamespace("p1")
		if err == nil || !strings.Contains(err.Error(), "multiple namespaces") {
			t.Fatalf("expected ambiguity error, got %v", err)
		}
	})

	t.Run("not found", func(t *testing.T) {
		withAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}, "total": 0})
		})
		namespace = ""
		t.Cleanup(func() { namespace = "" })

		_, err := resolveProposalNamespace("missing")
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("expected not-found error, got %v", err)
		}
	})

	t.Run("resolves unique namespace", func(t *testing.T) {
		withAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{"name": "p1", "namespace": "prod"}},
				"total": 1,
			})
		})
		namespace = ""
		t.Cleanup(func() { namespace = "" })

		ns, err := resolveProposalNamespace("p1")
		if err != nil || ns != "prod" {
			t.Fatalf("got namespace %q, err %v", ns, err)
		}
	})
}

func TestExplainFormattingHelpers(t *testing.T) {
	when := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	if got := dash(""); got != "-" {
		t.Fatalf("dash empty = %q", got)
	}
	if got := quoteOrNone(""); got != "(none recorded)" {
		t.Fatalf("quoteOrNone empty = %q", got)
	}
	if got := formatCarbon(2500); got != "2.50 kg CO2e" {
		t.Fatalf("formatCarbon = %q", got)
	}
	if got := formatCarbon(2_500_000); got != "2.50 t CO2e" {
		t.Fatalf("formatCarbon tonnes = %q", got)
	}
	snapshot := explainSnapshot{Replicas: 2, Timestamp: &when}
	report := explainReport{Name: "p1", Namespace: "prod", Snapshot: &snapshot}
	var out bytes.Buffer
	cmd := explainCmd
	cmd.SetOut(&out)
	t.Cleanup(func() { cmd.SetOut(nil) })
	writeExplainReport(cmd, report)
	if !strings.Contains(out.String(), "Snapshot: 2 replicas at 2026-10-01T12:00:00Z") {
		t.Fatalf("unexpected table output:\n%s", out.String())
	}
}
