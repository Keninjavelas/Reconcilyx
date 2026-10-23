package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func setWhatIfFlags(t *testing.T, values map[string]string) {
	t.Helper()
	for name, value := range values {
		if err := whatIfCmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}
	t.Cleanup(func() {
		for name := range values {
			_ = whatIfCmd.Flags().Set(name, "")
		}
		_ = whatIfCmd.Flags().Set("kind", "Deployment")
		whatIfCmd.SetOut(nil)
	})
}

func whatIfServer(t *testing.T, response whatIfResponse, capture *whatIfRequest) {
	t.Helper()
	withAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/what-if" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if capture != nil {
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, capture); err != nil {
				t.Errorf("decode request: %v", err)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
}

func TestWhatIfCommandJSONAndFailOn(t *testing.T) {
	var captured whatIfRequest
	whatIfServer(t, whatIfResponse{
		Allowed:              true,
		ConfidenceScore:      78,
		RiskScore:            15,
		CPUDelta:             "-1",
		MemoryDelta:          "-2Gi",
		SavingsUSDPerMonth:   "0.00",
		CostDeltaUSDPerMonth: 51.5,
		Rationale:            "Proposal within default safety constraints. Note: proposed configuration increases spend.",
	}, &captured)

	setWhatIfFlags(t, map[string]string{
		"current-cpu":     "2",
		"current-memory":  "4Gi",
		"proposed-cpu":    "4",
		"proposed-memory": "8Gi",
		"workload":        "web",
		"kind":            "Deployment",
		"output":          "json",
		"fail-on":         "cost-increase",
	})
	namespace = "prod"
	t.Cleanup(func() { namespace = "" })

	var out bytes.Buffer
	whatIfCmd.SetOut(&out)

	err := whatIfCmd.RunE(whatIfCmd, nil)
	if err == nil {
		t.Fatal("expected cost-increase to fail the command")
	}
	var exitErr *exitCodeError
	if !errors.As(err, &exitErr) || exitErr.code != 2 {
		t.Fatalf("expected exit code 2, got %v", err)
	}
	if !strings.Contains(err.Error(), "monthly cost increases by $51.50") {
		t.Fatalf("unexpected failure reason: %v", err)
	}

	var got whatIfResponse
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode JSON output: %v\n%s", err, out.String())
	}
	if got.CostDeltaUSDPerMonth != 51.5 {
		t.Fatalf("cost delta not preserved: %+v", got)
	}
	if captured.CurrentCPU != "2" || captured.ProposedMemory != "8Gi" || captured.WorkloadName != "web" || captured.Namespace != "prod" {
		t.Fatalf("request payload not forwarded: %+v", captured)
	}
}

func TestWhatIfCommandAllowedAndSavings(t *testing.T) {
	whatIfServer(t, whatIfResponse{
		Allowed:              true,
		ConfidenceScore:      80,
		RiskScore:            20,
		CPUDelta:             "500m",
		MemoryDelta:          "2Gi",
		SavingsUSDPerMonth:   "25.75",
		CostDeltaUSDPerMonth: -25.75,
		Rationale:            "Proposal within default safety constraints.",
	}, nil)

	setWhatIfFlags(t, map[string]string{
		"current-cpu":     "2",
		"current-memory":  "4Gi",
		"proposed-cpu":    "1",
		"proposed-memory": "2Gi",
		"output":          "table",
		"fail-on":         "cost-increase,not-allowed,savings-below:20",
	})

	var out bytes.Buffer
	whatIfCmd.SetOut(&out)

	if err := whatIfCmd.RunE(whatIfCmd, nil); err != nil {
		t.Fatalf("unexpected failure: %v", err)
	}
	text := out.String()
	for _, want := range []string{"-$25.75/mo (savings)", "Reported savings: $25.75/mo", "allowed: true"} {
		if !strings.Contains(text, want) {
			t.Fatalf("table missing %q:\n%s", want, text)
		}
	}
}

func TestWhatIfCommandRejectsHubError(t *testing.T) {
	withAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Invalid currentCPU","details":"quantities must match the regular expression"}`))
	})

	setWhatIfFlags(t, map[string]string{
		"current-cpu":     "bogus",
		"current-memory":  "4Gi",
		"proposed-cpu":    "1",
		"proposed-memory": "2Gi",
		"output":          "table",
	})

	err := whatIfCmd.RunE(whatIfCmd, nil)
	if err == nil {
		t.Fatal("expected API error")
	}
	if !strings.Contains(err.Error(), "HTTP 400") || !strings.Contains(err.Error(), "Invalid currentCPU") {
		t.Fatalf("unexpected error: %v", err)
	}
}
