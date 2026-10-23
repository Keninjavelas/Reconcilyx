package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestParseFailOn(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		rules, err := parseFailOn("  ")
		if err != nil || len(rules) != 0 {
			t.Fatalf("expected no rules, got %v (err %v)", rules, err)
		}
	})

	t.Run("mixed conditions", func(t *testing.T) {
		rules, err := parseFailOn("cost-increase, carbon-increase, guardrail,not-allowed,risk>=70,confidence<60,savings-below:25.5")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []failOnRule{
			{kind: "cost-increase"},
			{kind: "carbon-increase"},
			{kind: "guardrail"},
			{kind: "not-allowed"},
			{kind: "risk", threshold: 70},
			{kind: "confidence", threshold: 60},
			{kind: "savings", threshold: 25.5},
		}
		if len(rules) != len(want) {
			t.Fatalf("expected %d rules, got %d: %v", len(want), len(rules), rules)
		}
		for i := range want {
			if rules[i] != want[i] {
				t.Fatalf("rule %d = %+v, want %+v", i, rules[i], want[i])
			}
		}
	})

	for _, raw := range []string{
		"cost-decrease",
		"risk>=high",
		"confidence<",
		"savings-below:",
		"risk>70",
	} {
		t.Run("invalid "+raw, func(t *testing.T) {
			if _, err := parseFailOn(raw); err == nil {
				t.Fatalf("expected %q to be rejected", raw)
			}
		})
	}
}

func TestEvaluateFailOn(t *testing.T) {
	allowed := true
	denied := false
	base := failOnMetrics{
		CostDeltaUSD:        10,
		CarbonDeltaGrams:    500,
		GuardrailViolations: 1,
		Allowed:             &denied,
		RiskScore:           75,
		ConfidenceScore:     50,
		SavingsUSD:          10,
	}

	cases := []struct {
		rule string
		want string
	}{
		{"cost-increase", "monthly cost increases by $10.00"},
		{"carbon-increase", "monthly carbon increases by 500 g CO2e"},
		{"guardrail", "1 guardrail violation(s) detected"},
		{"not-allowed", "what-if simulation is not allowed"},
		{"risk>=70", "risk score 75 is at or above 70"},
		{"confidence<60", "confidence score 50 is below 60"},
		{"savings-below:25", "monthly savings $10.00 are below $25.00"},
	}
	for _, tc := range cases {
		t.Run(tc.rule, func(t *testing.T) {
			rules, err := parseFailOn(tc.rule)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			got := evaluateFailOn(rules, base)
			if got == "" {
				t.Fatalf("expected %q to trigger", tc.rule)
			}
			if !strings.Contains(got, tc.want) {
				t.Fatalf("reason %q does not contain %q", got, tc.want)
			}
		})
	}

	t.Run("no trigger", func(t *testing.T) {
		rules, _ := parseFailOn("cost-increase,carbon-increase,guardrail,not-allowed,risk>=90,confidence<40,savings-below:1")
		metrics := failOnMetrics{Allowed: &allowed, CostDeltaUSD: -2, CarbonDeltaGrams: -10, RiskScore: 10, ConfidenceScore: 90, SavingsUSD: 5}
		if reason := evaluateFailOn(rules, metrics); reason != "" {
			t.Fatalf("expected no trigger, got %q", reason)
		}
	})

	t.Run("nil allowed pointer is ignored", func(t *testing.T) {
		rules, _ := parseFailOn("not-allowed")
		if reason := evaluateFailOn(rules, failOnMetrics{Allowed: nil}); reason != "" {
			t.Fatalf("expected nil Allowed to be ignored, got %q", reason)
		}
	})
}

func TestEnforceFailOnExitCode(t *testing.T) {
	rules, _ := parseFailOn("cost-increase")
	err := enforceFailOn(rules, failOnMetrics{CostDeltaUSD: 1})
	if err == nil {
		t.Fatal("expected an error")
	}
	var exitErr *exitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected exitCodeError, got %T: %v", err, err)
	}
	if exitErr.code != 2 {
		t.Fatalf("expected exit code 2, got %d", exitErr.code)
	}
	if err := enforceFailOn(nil, failOnMetrics{CostDeltaUSD: 1}); err != nil {
		t.Fatalf("empty rule set must not fail: %v", err)
	}
}

func TestRestrictFailOn(t *testing.T) {
	rules, _ := parseFailOn("cost-increase,guardrail")
	if _, err := restrictFailOn(rules, "cost-increase", "carbon-increase", "guardrail"); err != nil {
		t.Fatalf("expected supported rules: %v", err)
	}

	rules, _ = parseFailOn("risk>=70")
	_, err := restrictFailOn(rules, "cost-increase", "carbon-increase", "guardrail")
	if err == nil {
		t.Fatal("expected risk>= to be rejected for diff")
	}
	if !strings.Contains(err.Error(), "risk>=") {
		t.Fatalf("error should name the offending condition: %v", err)
	}
}

func TestResolveOutputFormat(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	addOutputFlag(cmd)

	for _, tc := range []struct {
		value string
		want  string
	}{
		{"", "table"},
		{"table", "table"},
		{"json", "json"},
		{" JSON ", "json"},
	} {
		if err := cmd.Flags().Set("output", tc.value); err != nil {
			t.Fatalf("set: %v", err)
		}
		got, err := resolveOutputFormat(cmd)
		if err != nil {
			t.Fatalf("resolve %q: %v", tc.value, err)
		}
		if got != tc.want {
			t.Fatalf("resolve %q = %q, want %q", tc.value, got, tc.want)
		}
	}

	if err := cmd.Flags().Set("output", "yaml"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, err := resolveOutputFormat(cmd); err == nil {
		t.Fatal("expected yaml to be rejected")
	}
}
