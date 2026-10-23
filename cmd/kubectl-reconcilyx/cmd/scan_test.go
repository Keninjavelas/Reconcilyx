package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func withAPIServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(handler)
	oldURL := apiServerURL
	oldToken := apiToken
	apiServerURL = ts.URL
	apiToken = ""
	t.Cleanup(func() {
		apiServerURL = oldURL
		apiToken = oldToken
		ts.Close()
	})
	return ts
}

func scanFixtureItems() []proposalListItem {
	created := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	return []proposalListItem{
		{
			Name:             "web-rightsize",
			Namespace:        "prod",
			Cluster:          "east",
			WorkloadKind:     "Deployment",
			WorkloadName:     "web",
			Recommendation:   "RightSize",
			ApprovalState:    "Pending",
			Phase:            "Scheduled",
			RiskScore:        20,
			ConfidenceScore:  85,
			EstimatedSavings: "12.5",
			CarbonSavedGrams: 3000,
			CreatedAt:        created,
		},
		{
			Name:             "api-rightsize",
			Namespace:        "prod",
			WorkloadKind:     "Deployment",
			WorkloadName:     "api",
			Recommendation:   "ReduceCPURequest",
			ApprovalState:    "Approved",
			Phase:            "Completed",
			RiskScore:        10,
			ConfidenceScore:  92,
			EstimatedSavings: "5",
			CarbonSavedGrams: 1000,
			CreatedAt:        created.Add(time.Hour),
		},
		{
			Name:             "batch-rightsize",
			Namespace:        "staging",
			WorkloadKind:     "StatefulSet",
			WorkloadName:     "batch",
			Recommendation:   "ReduceMemoryRequest",
			ApprovalState:    "Rejected",
			Phase:            "Rejected",
			RiskScore:        55,
			ConfidenceScore:  40,
			EstimatedSavings: "40",
			CarbonSavedGrams: 500,
			CreatedAt:        created.Add(2 * time.Hour),
		},
	}
}

func TestBuildScanReportMapsHubData(t *testing.T) {
	report, err := buildScanReport(scanFixtureItems(), "http://hub", "prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Summary.Total == 0 || len(report.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(report.Items))
	}
	if report.Summary.Total != 3 ||
		report.Summary.PendingApproval != 1 ||
		report.Summary.InFlight != 1 ||
		report.Summary.Completed != 1 ||
		report.Summary.Failed != 0 ||
		report.Summary.Rejected != 1 {
		t.Fatalf("unexpected summary counts: %+v", report.Summary)
	}
	if report.Summary.EstimatedMonthlySavingsUSD != 57.5 {
		t.Fatalf("expected $57.50 savings, got %v", report.Summary.EstimatedMonthlySavingsUSD)
	}
	if report.Summary.CarbonAbatedGrams != 4500 || report.Summary.CarbonAbatedKg != 4.5 {
		t.Fatalf("unexpected carbon summary: %+v", report.Summary)
	}
	if report.Server != "http://hub" || report.Namespace != "prod" {
		t.Fatalf("server/namespace not recorded: %+v", report)
	}
	if report.GeneratedAt.IsZero() {
		t.Fatal("generatedAt must be set")
	}

	if report.Items[0].Proposal != "batch-rightsize" || report.Items[0].EstimatedMonthlySavingsUSD != 40 {
		t.Fatalf("expected highest-savings proposal first, got %+v", report.Items[0])
	}
	if report.Items[1].Proposal != "web-rightsize" || report.Items[2].Proposal != "api-rightsize" {
		t.Fatalf("expected savings-descending order, got %v, %v", report.Items[1].Proposal, report.Items[2].Proposal)
	}
	web := report.Items[1]
	if web.WorkloadKind != "Deployment" || web.WorkloadName != "web" || web.CarbonAbatedKg != 3 {
		t.Fatalf("hub fields were not preserved: %+v", web)
	}
}

func TestBuildScanReportRejectsMalformedSavings(t *testing.T) {
	items := []proposalListItem{{
		Name:             "bad",
		Namespace:        "prod",
		EstimatedSavings: "twelve dollars",
	}}
	_, err := buildScanReport(items, "http://hub", "")
	if err == nil {
		t.Fatal("expected malformed savings to fail")
	}
	if !strings.Contains(err.Error(), "invalid estimatedSavings") || !strings.Contains(err.Error(), "bad") {
		t.Fatalf("error should identify the proposal and field: %v", err)
	}
}

func TestScanCommandUsesHubResponse(t *testing.T) {
	items := scanFixtureItems()
	withAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/proposals" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "total": len(items)})
	})

	var out bytes.Buffer
	scanCmd.SetOut(&out)
	t.Cleanup(func() {
		scanCmd.SetOut(nil)
		_ = scanCmd.Flags().Set("output", "table")
		namespace = ""
	})
	if err := scanCmd.Flags().Set("output", "json"); err != nil {
		t.Fatalf("set output: %v", err)
	}
	namespace = "prod"

	if err := scanCmd.RunE(scanCmd, nil); err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	var report scanReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("decode scan JSON: %v\n%s", err, out.String())
	}
	if len(report.Items) != 3 {
		t.Fatalf("expected 3 hub items, got %d: %s", len(report.Items), out.String())
	}
	if report.Items[0].Proposal != "batch-rightsize" {
		t.Fatalf("unexpected first item: %+v", report.Items[0])
	}
	if strings.Contains(out.String(), "No optimization proposals found") {
		t.Fatal("scan must not print the empty-state fallback when the hub returned data")
	}
}

func TestScanCommandPropagatesHubFailure(t *testing.T) {
	withAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"Service Unavailable","details":"hub offline"}`))
	})

	var out bytes.Buffer
	scanCmd.SetOut(&out)
	t.Cleanup(func() {
		scanCmd.SetOut(nil)
		_ = scanCmd.Flags().Set("output", "table")
	})
	_ = scanCmd.Flags().Set("output", "json")

	err := scanCmd.RunE(scanCmd, nil)
	if err == nil {
		t.Fatal("expected scan to fail when the hub is unavailable")
	}
	if !strings.Contains(err.Error(), "HTTP 503") || !strings.Contains(err.Error(), "hub offline") {
		t.Fatalf("error should carry the API failure: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("scan must not print a report on failure, got %q", out.String())
	}
}

func TestScanCommandEmptyHubIsHonest(t *testing.T) {
	withAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[],"total":0}`))
	})

	var out bytes.Buffer
	scanCmd.SetOut(&out)
	t.Cleanup(func() { scanCmd.SetOut(nil) })

	if err := scanCmd.RunE(scanCmd, nil); err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if !strings.Contains(out.String(), "No optimization proposals found") {
		t.Fatalf("expected empty-state message, got %q", out.String())
	}
	if strings.Contains(out.String(), "demo") || strings.Contains(out.String(), "checkout") {
		t.Fatalf("empty hub response must not fabricate workloads: %q", out.String())
	}
}
