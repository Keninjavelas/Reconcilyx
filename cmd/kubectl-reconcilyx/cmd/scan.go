package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

type scanItem struct {
	Proposal                   string    `json:"proposal"`
	Namespace                  string    `json:"namespace"`
	Cluster                    string    `json:"cluster,omitempty"`
	WorkloadKind               string    `json:"workloadKind"`
	WorkloadName               string    `json:"workloadName"`
	Recommendation             string    `json:"recommendation"`
	Phase                      string    `json:"phase"`
	ApprovalState              string    `json:"approvalState"`
	ExecutionState             string    `json:"executionState,omitempty"`
	RiskScore                  int32     `json:"riskScore"`
	ConfidenceScore            int32     `json:"confidenceScore"`
	EstimatedMonthlySavingsUSD float64   `json:"estimatedMonthlySavingsUSD"`
	CarbonAbatedGrams          float64   `json:"carbonAbatedGrams"`
	CarbonAbatedKg             float64   `json:"carbonAbatedKg"`
	CreatedAt                  time.Time `json:"createdAt"`
}

type scanSummary struct {
	Total                      int     `json:"total"`
	PendingApproval            int     `json:"pendingApproval"`
	InFlight                   int     `json:"inFlight"`
	Completed                  int     `json:"completed"`
	Failed                     int     `json:"failed"`
	Rejected                   int     `json:"rejected"`
	EstimatedMonthlySavingsUSD float64 `json:"estimatedMonthlySavingsUSD"`
	CarbonAbatedGrams          float64 `json:"carbonAbatedGrams"`
	CarbonAbatedKg             float64 `json:"carbonAbatedKg"`
}

type scanReport struct {
	GeneratedAt time.Time   `json:"generatedAt"`
	Server      string      `json:"server"`
	Namespace   string      `json:"namespace,omitempty"`
	Summary     scanSummary `json:"summary"`
	Items       []scanItem  `json:"items"`
}

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan Hub proposals for resource waste & carbon reduction opportunities",
	Long: `Scan queries the Reconcilyx Hub for optimization proposals and reports the
workloads, recommended changes, estimated monthly savings, and carbon impact.

The command never substitutes synthetic workloads: if the Hub is unreachable or
rejects the request, scan exits non-zero and reports the API error.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		format, err := resolveOutputFormat(cmd)
		if err != nil {
			return err
		}

		endpoint := "/api/proposals"
		if namespace != "" {
			endpoint += "?namespace=" + url.QueryEscape(namespace)
		}
		body, status, err := doAPI(http.MethodGet, endpoint, nil)
		if err != nil {
			return fmt.Errorf("scan failed: cannot reach Reconcilyx API at %s: %w", apiServerURL, err)
		}
		if status < 200 || status >= 300 {
			hint := ""
			if status == http.StatusUnauthorized || status == http.StatusForbidden {
				hint = " (check --token or RECONCILYX_API_KEY)"
			}
			return fmt.Errorf("scan failed (HTTP %d): %s%s", status, extractAPIMessage(body), hint)
		}

		var env proposalListResponse
		if err := json.Unmarshal(body, &env); err != nil {
			return fmt.Errorf("scan failed: cannot parse Hub response: %w", err)
		}

		report, err := buildScanReport(env.Items, apiServerURL, namespace)
		if err != nil {
			return err
		}

		if format == "json" {
			return writeJSONOutput(cmd.OutOrStdout(), report)
		}
		writeScanTable(cmd, report)
		return nil
	},
}

func buildScanReport(items []proposalListItem, server, ns string) (scanReport, error) {
	report := scanReport{
		GeneratedAt: time.Now().UTC(),
		Server:      server,
		Namespace:   ns,
		Items:       make([]scanItem, 0, len(items)),
	}

	for _, p := range items {
		var savings float64
		if raw := strings.TrimSpace(p.EstimatedSavings); raw != "" {
			v, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				return scanReport{}, fmt.Errorf("scan failed: proposal %s/%s has invalid estimatedSavings %q: %w", p.Namespace, p.Name, p.EstimatedSavings, err)
			}
			savings = v
		}
		item := scanItem{
			Proposal:                   p.Name,
			Namespace:                  p.Namespace,
			Cluster:                    p.Cluster,
			WorkloadKind:               p.WorkloadKind,
			WorkloadName:               p.WorkloadName,
			Recommendation:             p.Recommendation,
			Phase:                      p.Phase,
			ApprovalState:              p.ApprovalState,
			ExecutionState:             p.ExecutionState,
			RiskScore:                  p.RiskScore,
			ConfidenceScore:            p.ConfidenceScore,
			EstimatedMonthlySavingsUSD: savings,
			CarbonAbatedGrams:          float64(p.CarbonSavedGrams),
			CarbonAbatedKg:             float64(p.CarbonSavedGrams) / 1000,
			CreatedAt:                  p.CreatedAt,
		}
		report.Items = append(report.Items, item)

		report.Summary.Total++
		switch p.ApprovalState {
		case "Pending":
			report.Summary.PendingApproval++
		}
		switch p.Phase {
		case "Scheduled", "Executing", "Verifying", "Stabilizing", "RollbackRequired", "RollingBack", "RollbackVerification":
			report.Summary.InFlight++
		case "Completed":
			report.Summary.Completed++
		case "Failed", "Expired":
			report.Summary.Failed++
		case "Rejected", "RolledBack":
			report.Summary.Rejected++
		}
		report.Summary.EstimatedMonthlySavingsUSD += savings
		report.Summary.CarbonAbatedGrams += float64(p.CarbonSavedGrams)
	}
	report.Summary.CarbonAbatedKg = report.Summary.CarbonAbatedGrams / 1000

	sort.SliceStable(report.Items, func(i, j int) bool {
		if report.Items[i].EstimatedMonthlySavingsUSD != report.Items[j].EstimatedMonthlySavingsUSD {
			return report.Items[i].EstimatedMonthlySavingsUSD > report.Items[j].EstimatedMonthlySavingsUSD
		}
		return report.Items[i].CarbonAbatedGrams > report.Items[j].CarbonAbatedGrams
	})
	return report, nil
}

func writeScanTable(cmd *cobra.Command, report scanReport) {
	out := cmd.OutOrStdout()
	if len(report.Items) == 0 {
		fmt.Fprintln(out, "No optimization proposals found.")
		fmt.Fprintln(out, "Create an OptimizationPolicy targeting workloads so the Hub can generate proposals.")
		return
	}

	w := tabwriter.NewWriter(out, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAMESPACE\tWORKLOAD\tRECOMMENDATION\tSTATE\tSAVINGS ($/MO)\tCARBON (KG/MO)\tRISK/CONF\tPROPOSAL")
	fmt.Fprintln(w, "---------\t--------\t--------------\t-----\t--------------\t---------------\t---------\t--------")
	for _, item := range report.Items {
		state := strings.Trim(strings.Join([]string{item.ApprovalState, item.Phase}, "/"), "/")
		if state == "" {
			state = "-"
		}
		fmt.Fprintf(w, "%s\t%s/%s\t%s\t%s\t$%.2f\t%.2f\t%d/%d\t%s\n",
			item.Namespace,
			item.WorkloadKind,
			item.WorkloadName,
			item.Recommendation,
			state,
			item.EstimatedMonthlySavingsUSD,
			item.CarbonAbatedKg,
			item.RiskScore,
			item.ConfidenceScore,
			item.Proposal,
		)
	}
	_ = w.Flush()

	s := report.Summary
	fmt.Fprintf(out, "\n%d proposals: %d pending approval, %d in flight, %d completed, %d failed, %d rejected\n",
		s.Total, s.PendingApproval, s.InFlight, s.Completed, s.Failed, s.Rejected)
	fmt.Fprintf(out, "Net estimated opportunity: $%.2f/mo | %.2f kg CO2e/mo\n",
		s.EstimatedMonthlySavingsUSD, s.CarbonAbatedKg)
}

func init() {
	addOutputFlag(scanCmd)
	RootCmd.AddCommand(scanCmd)
}
