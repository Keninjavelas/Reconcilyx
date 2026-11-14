package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type whatIfRequest struct {
	Namespace      string `json:"namespace"`
	WorkloadName   string `json:"workloadName"`
	WorkloadKind   string `json:"workloadKind"`
	CurrentCPU     string `json:"currentCPU"`
	CurrentMemory  string `json:"currentMemory"`
	ProposedCPU    string `json:"proposedCPU"`
	ProposedMemory string `json:"proposedMemory"`
	P95CPU         string `json:"p95CPU,omitempty"`
	P95Memory      string `json:"p95Memory,omitempty"`
}

type whatIfResponse struct {
	Allowed              bool    `json:"allowed"`
	ConfidenceScore      int32   `json:"confidenceScore"`
	RiskScore            int32   `json:"riskScore"`
	CPUDelta             string  `json:"cpuDelta"`
	MemoryDelta          string  `json:"memoryDelta"`
	SavingsUSDPerMonth   string  `json:"savingsUSDPerMonth"`
	CostDeltaUSDPerMonth float64 `json:"costDeltaUSDPerMonth"`
	Rationale            string  `json:"rationale"`
}

var whatIfCmd = &cobra.Command{
	Use:   "what-if",
	Short: "Simulate a resource change and report cost, carbon, and risk before applying it",
	Long: `Simulate changes to a workload's CPU and memory requests against the Hub
what-if engine. The command reports the risk score, confidence, allowed state,
monthly savings, and a signed cost delta (positive means increased spend).

Use --fail-on in CI to fail the step when the simulation is unsafe or increases
cost. A triggered condition exits with code 2; transport and validation errors
exit with code 1.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		format, err := resolveOutputFormat(cmd)
		if err != nil {
			return err
		}
		rules, err := parseFailOn(flagString(cmd, "fail-on"))
		if err != nil {
			return err
		}

		payload := whatIfRequest{
			Namespace:      namespace,
			WorkloadName:   flagString(cmd, "workload"),
			WorkloadKind:   flagString(cmd, "kind"),
			CurrentCPU:     flagString(cmd, "current-cpu"),
			CurrentMemory:  flagString(cmd, "current-memory"),
			ProposedCPU:    flagString(cmd, "proposed-cpu"),
			ProposedMemory: flagString(cmd, "proposed-memory"),
			P95CPU:         flagString(cmd, "p95-cpu"),
			P95Memory:      flagString(cmd, "p95-memory"),
		}
		body, status, err := doAPI(http.MethodPost, "/api/what-if", payload)
		if err != nil {
			return fmt.Errorf("what-if failed: cannot reach Reconcilyx API at %s: %w", apiServerURL, err)
		}
		if status < 200 || status >= 300 {
			hint := ""
			if status == http.StatusUnauthorized || status == http.StatusForbidden {
				hint = " (check --token or RECONCILYX_API_KEY)"
			}
			return fmt.Errorf("what-if failed (HTTP %d): %s%s", status, extractAPIMessage(body), hint)
		}

		var resp whatIfResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return fmt.Errorf("what-if failed: cannot parse Hub response: %w", err)
		}
		savings, err := strconv.ParseFloat(resp.SavingsUSDPerMonth, 64)
		if err != nil {
			return fmt.Errorf("what-if failed: Hub returned invalid savings %q: %w", resp.SavingsUSDPerMonth, err)
		}

		switch format {
		case "json":
			if err := writeJSONOutput(cmd.OutOrStdout(), resp); err != nil {
				return err
			}
		case "markdown":
			writeWhatIfMarkdown(cmd.OutOrStdout(), payload, resp)
		case "sarif":
			if err := writeWhatIfSARIF(cmd.OutOrStdout(), payload, resp); err != nil {
				return err
			}
		default:
			writeWhatIfTable(cmd, payload, resp)
		}

		allowed := resp.Allowed
		return enforceFailOn(rules, failOnMetrics{
			CostDeltaUSD:    resp.CostDeltaUSDPerMonth,
			Allowed:         &allowed,
			RiskScore:       resp.RiskScore,
			ConfidenceScore: resp.ConfidenceScore,
			SavingsUSD:      savings,
		})
	},
}

func writeWhatIfTable(cmd *cobra.Command, req whatIfRequest, resp whatIfResponse) {
	out := cmd.OutOrStdout()
	workload := strings.Trim(strings.Join([]string{req.WorkloadKind, req.Namespace, req.WorkloadName}, "/"), "/")
	if workload == "" {
		workload = "(unnamed workload)"
	}
	fmt.Fprintf(out, "What-if: %s\n", workload)
	fmt.Fprintf(out, "  Resources: CPU %s -> %s | Memory %s -> %s\n",
		req.CurrentCPU, req.ProposedCPU, req.CurrentMemory, req.ProposedMemory)

	costDelta := "no change"
	switch {
	case resp.CostDeltaUSDPerMonth > 0:
		costDelta = fmt.Sprintf("+$%.2f/mo (increase)", resp.CostDeltaUSDPerMonth)
	case resp.CostDeltaUSDPerMonth < 0:
		costDelta = fmt.Sprintf("-$%.2f/mo (savings)", -resp.CostDeltaUSDPerMonth)
	}
	fmt.Fprintf(out, "  Monthly cost delta: %s\n", costDelta)
	fmt.Fprintf(out, "  Reported savings: $%s/mo\n", resp.SavingsUSDPerMonth)
	fmt.Fprintf(out, "  Risk %d / confidence %d | allowed: %t\n", resp.RiskScore, resp.ConfidenceScore, resp.Allowed)
	fmt.Fprintf(out, "  CPU delta: %s | Memory delta: %s\n", resp.CPUDelta, resp.MemoryDelta)
	fmt.Fprintf(out, "  %s\n", resp.Rationale)
}

func flagString(cmd *cobra.Command, name string) string {
	v, err := cmd.Flags().GetString(name)
	if err != nil {
		return ""
	}
	return v
}

func init() {
	whatIfCmd.Flags().String("current-cpu", "", "Current CPU request (for example 500m)")
	whatIfCmd.Flags().String("current-memory", "", "Current memory request (for example 256Mi)")
	whatIfCmd.Flags().String("proposed-cpu", "", "Proposed CPU request")
	whatIfCmd.Flags().String("proposed-memory", "", "Proposed memory request")
	whatIfCmd.Flags().String("workload", "", "Optional workload name for the report")
	whatIfCmd.Flags().String("kind", "Deployment", "Optional workload kind for the report")
	whatIfCmd.Flags().String("p95-cpu", "", "Optional P95 CPU usage")
	whatIfCmd.Flags().String("p95-memory", "", "Optional P95 memory usage")
	addOutputFlag(whatIfCmd)
	addFailOnFlag(whatIfCmd, "Comma-separated CI conditions: cost-increase, not-allowed, risk>=N, confidence<N, savings-below:N")
	for _, name := range []string{"current-cpu", "current-memory", "proposed-cpu", "proposed-memory"} {
		_ = whatIfCmd.MarkFlagRequired(name)
	}
	RootCmd.AddCommand(whatIfCmd)
}
