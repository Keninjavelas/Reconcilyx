package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// exitCodeError lets a command request a specific process exit code without
// losing the underlying error message.
type exitCodeError struct {
	code int
	err  error
}

func (e *exitCodeError) Error() string { return e.err.Error() }
func (e *exitCodeError) Unwrap() error { return e.err }

func withExitCode(code int, err error) error {
	if err == nil {
		return nil
	}
	return &exitCodeError{code: code, err: err}
}

func addOutputFlag(cmd *cobra.Command) {
	cmd.Flags().StringP("output", "o", "table", "Output format: table or json")
}

func resolveOutputFormat(cmd *cobra.Command) (string, error) {
	raw, err := cmd.Flags().GetString("output")
	if err != nil {
		return "", err
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "table":
		return "table", nil
	case "json":
		return "json", nil
	default:
		return "", fmt.Errorf("unsupported output format %q (use table or json)", raw)
	}
}

func writeJSONOutput(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

// --- CI fail-on conditions --------------------------------------------------

type failOnRule struct {
	kind      string
	threshold float64
}

type failOnMetrics struct {
	CostDeltaUSD        float64
	CarbonDeltaGrams    float64
	GuardrailViolations int
	Allowed             *bool
	RiskScore           int32
	ConfidenceScore     int32
	SavingsUSD          float64
}

func addFailOnFlag(cmd *cobra.Command, help string) {
	cmd.Flags().String("fail-on", "", help)
}

func parseFailOn(raw string) ([]failOnRule, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var rules []failOnRule
	for _, part := range strings.Split(raw, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" {
			continue
		}
		switch {
		case part == "cost-increase", part == "carbon-increase", part == "guardrail", part == "not-allowed":
			rules = append(rules, failOnRule{kind: part})
		case strings.HasPrefix(part, "risk>="):
			v, err := parseThreshold(part, "risk>=")
			if err != nil {
				return nil, err
			}
			rules = append(rules, failOnRule{kind: "risk", threshold: v})
		case strings.HasPrefix(part, "confidence<"):
			v, err := parseThreshold(part, "confidence<")
			if err != nil {
				return nil, err
			}
			rules = append(rules, failOnRule{kind: "confidence", threshold: v})
		case strings.HasPrefix(part, "savings-below:"):
			v, err := parseThreshold(part, "savings-below:")
			if err != nil {
				return nil, err
			}
			rules = append(rules, failOnRule{kind: "savings", threshold: v})
		default:
			return nil, fmt.Errorf("unsupported --fail-on condition %q", part)
		}
	}
	return rules, nil
}

func parseThreshold(condition, prefix string) (float64, error) {
	raw := strings.TrimPrefix(condition, prefix)
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid --fail-on condition %q: expected a number after %q", condition, prefix)
	}
	return v, nil
}

// evaluateFailOn returns a human-readable trigger description, or "" when no
// rule matches. An empty rule set never fails.
func evaluateFailOn(rules []failOnRule, m failOnMetrics) string {
	for _, rule := range rules {
		switch rule.kind {
		case "cost-increase":
			if m.CostDeltaUSD > 0 {
				return fmt.Sprintf("monthly cost increases by $%.2f", m.CostDeltaUSD)
			}
		case "carbon-increase":
			if m.CarbonDeltaGrams > 0 {
				return fmt.Sprintf("monthly carbon increases by %.0f g CO2e", m.CarbonDeltaGrams)
			}
		case "guardrail":
			if m.GuardrailViolations > 0 {
				return fmt.Sprintf("%d guardrail violation(s) detected", m.GuardrailViolations)
			}
		case "not-allowed":
			if m.Allowed != nil && !*m.Allowed {
				return "what-if simulation is not allowed by its risk assessment"
			}
		case "risk":
			if float64(m.RiskScore) >= rule.threshold {
				return fmt.Sprintf("risk score %d is at or above %.0f", m.RiskScore, rule.threshold)
			}
		case "confidence":
			if float64(m.ConfidenceScore) < rule.threshold {
				return fmt.Sprintf("confidence score %d is below %.0f", m.ConfidenceScore, rule.threshold)
			}
		case "savings":
			if m.SavingsUSD < rule.threshold {
				return fmt.Sprintf("monthly savings $%.2f are below $%.2f", m.SavingsUSD, rule.threshold)
			}
		}
	}
	return ""
}

func enforceFailOn(rules []failOnRule, m failOnMetrics) error {
	if reason := evaluateFailOn(rules, m); reason != "" {
		return withExitCode(2, fmt.Errorf("fail-on triggered: %s", reason))
	}
	return nil
}
