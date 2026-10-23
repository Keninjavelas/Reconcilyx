package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type apiErrorResp struct {
	Error   string `json:"error"`
	Code    int    `json:"code"`
	Details string `json:"details,omitempty"`
}

type actionResp struct {
	Status        string `json:"status"`
	ApprovalState string `json:"approvalState,omitempty"`
	Phase         string `json:"phase,omitempty"`
	Reason        string `json:"reason,omitempty"`
	Message       string `json:"message,omitempty"`
}

var (
	reasonFlag string
)

var proposalsCmd = &cobra.Command{
	Use:   "proposals",
	Short: "Inspect pending optimization proposals",
	RunE: func(cmd *cobra.Command, args []string) error {
		endpoint := "/api/proposals"
		ns, _ := cmd.Flags().GetString("namespace")
		if ns != "" {
			endpoint = fmt.Sprintf("%s?namespace=%s", endpoint, url.QueryEscape(ns))
		}
		body, status, err := doAPI(http.MethodGet, endpoint, nil)
		if err != nil {
			return fmt.Errorf("failed to list proposals: %w", err)
		}
		if status < 200 || status >= 300 {
			return fmt.Errorf("list proposals failed (HTTP %d): %s", status, extractAPIMessage(body))
		}

		var env proposalListResponse
		if err := json.Unmarshal(body, &env); err != nil {
			return fmt.Errorf("failed to parse proposals response: %w", err)
		}
		if len(env.Items) == 0 {
			fmt.Println("📋 No optimization proposals found.")
			fmt.Println("Create an OptimizationPolicy targeting workloads to generate proposals.")
			return nil
		}
		fmt.Printf("📋 Active Reconcilyx Optimization Proposals (%d total):\n", env.Total)
		fmt.Println("---------------------------------------------------------------------------------")
		for i, p := range env.Items {
			loc := fmt.Sprintf("%s/%s", p.Namespace, p.WorkloadName)
			if p.Cluster != "" {
				loc = fmt.Sprintf("[%s] %s", p.Cluster, loc)
			}
			fmt.Printf("%d. %-30s  [%s / %s]\n", i+1, p.Name, p.ApprovalState, p.Phase)
			fmt.Printf("   %s  •  %s / %s\n", loc, p.WorkloadKind, p.Recommendation)
			impact := []string{}
			if p.EstimatedSavings != "" {
				impact = append(impact, fmt.Sprintf("Save: $%s/mo", p.EstimatedSavings))
			}
			if p.CarbonSavedHuman != "" {
				impact = append(impact, p.CarbonSavedHuman)
			}
			impact = append(impact, fmt.Sprintf("risk=%d conf=%d", p.RiskScore, p.ConfidenceScore))
			fmt.Printf("   %s\n", strings.Join(impact, " | "))
		}
		fmt.Println()
		fmt.Println("Run 'kubectl reconcilyx approve <proposal-name>' to approve and execute.")
		fmt.Println("Run 'kubectl reconcilyx reject  <proposal-name> --reason <str>' to reject.")
		return nil
	},
}

var approveCmd = &cobra.Command{
	Use:   "approve [proposal-name]",
	Short: "Approve a resource optimization proposal",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		proposalName := args[0]
		ns, _ := cmd.Flags().GetString("namespace")
		if reasonFlag == "" {
			reasonFlag = "Approved via kubectl reconcilyx"
		}
		payload := map[string]string{"reason": reasonFlag}
		q := ""
		if ns != "" {
			q = "?namespace=" + url.QueryEscape(ns)
		}
		path := fmt.Sprintf("/api/proposals/%s/approve%s", url.PathEscape(proposalName), q)
		fmt.Printf("🚀 Approving proposal '%s'...\n", proposalName)
		body, status, err := doAPI(http.MethodPost, path, payload)
		if err != nil {
			return fmt.Errorf("approval request failed: %w", err)
		}
		if status < 200 || status >= 300 {
			return fmt.Errorf("failed to approve proposal: %s", extractAPIMessage(body))
		}
		var resp actionResp
		_ = json.Unmarshal(body, &resp)
		if resp.Status == "no-op" {
			fmt.Printf("ℹ️  Proposal '%s' is already approved. %s\n", proposalName, resp.Message)
			return nil
		}
		fmt.Printf("✅ Proposal '%s' approved successfully (state=%s, phase=%s)\n", proposalName, resp.ApprovalState, resp.Phase)
		return nil
	},
}

var rejectCmd = &cobra.Command{
	Use:   "reject [proposal-name]",
	Short: "Reject a resource optimization proposal (requires --reason)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		proposalName := args[0]
		ns, _ := cmd.Flags().GetString("namespace")
		if strings.TrimSpace(reasonFlag) == "" {
			return fmt.Errorf("--reason is required when rejecting a proposal (e.g. --reason 'Aggressive cut, too risky')")
		}
		payload := map[string]string{"reason": reasonFlag}
		q := ""
		if ns != "" {
			q = "?namespace=" + url.QueryEscape(ns)
		}
		path := fmt.Sprintf("/api/proposals/%s/reject%s", url.PathEscape(proposalName), q)
		fmt.Printf("🛑 Rejecting proposal '%s'...\n", proposalName)
		body, status, err := doAPI(http.MethodPost, path, payload)
		if err != nil {
			return fmt.Errorf("reject request failed: %w", err)
		}
		if status < 200 || status >= 300 {
			return fmt.Errorf("failed to reject proposal: %s", extractAPIMessage(body))
		}
		var resp actionResp
		_ = json.Unmarshal(body, &resp)
		fmt.Printf("❌ Proposal '%s' rejected successfully (state=%s, phase=%s)\n", proposalName, resp.ApprovalState, resp.Phase)
		return nil
	},
}

var rollbackCmd = &cobra.Command{
	Use:   "rollback [proposal-name]",
	Short: "Request rollback of an applied/stabilizing optimization proposal",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		proposalName := args[0]
		ns, _ := cmd.Flags().GetString("namespace")
		if reasonFlag == "" {
			reasonFlag = "Rollback requested via kubectl reconcilyx"
		}
		payload := map[string]string{"reason": reasonFlag}
		q := ""
		if ns != "" {
			q = "?namespace=" + url.QueryEscape(ns)
		}
		path := fmt.Sprintf("/api/proposals/%s/rollback%s", url.PathEscape(proposalName), q)
		fmt.Printf("↩️  Requesting rollback for proposal '%s'...\n", proposalName)
		body, status, err := doAPI(http.MethodPost, path, payload)
		if err != nil {
			return fmt.Errorf("rollback request failed: %w", err)
		}
		if status < 200 || status >= 300 {
			return fmt.Errorf("failed to rollback proposal: %s", extractAPIMessage(body))
		}
		var resp actionResp
		_ = json.Unmarshal(body, &resp)
		fmt.Printf("🔄 Rollback scheduled for proposal '%s' (phase=%s)\n", proposalName, resp.Phase)
		fmt.Println("   Controller will revert resources to the execution snapshot.")
		return nil
	},
}

func doAPI(method, path string, payload any) ([]byte, int, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal payload: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}
	full := strings.TrimRight(apiServerURL, "/") + path
	req, err := http.NewRequest(method, full, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if apiToken != "" {
		req.Header.Set("X-API-Key", apiToken)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response: %w", err)
	}
	return b, resp.StatusCode, nil
}

func extractAPIMessage(raw []byte) string {
	if len(raw) == 0 {
		return "empty response body"
	}
	var ae apiErrorResp
	if err := json.Unmarshal(raw, &ae); err == nil {
		msg := strings.TrimSpace(ae.Error + " " + ae.Details)
		if strings.TrimSpace(msg) != "" {
			return msg
		}
	}
	s := string(bytes.TrimSpace(raw))
	if len(s) > 240 {
		s = s[:240] + "..."
	}
	return s
}

func init() {
	approveCmd.Flags().StringVarP(&reasonFlag, "reason", "r", "", "Human-readable reason for this action (optional for approve)")
	rejectCmd.Flags().StringVarP(&reasonFlag, "reason", "r", "", "Human-readable reason for this rejection (required)")
	rollbackCmd.Flags().StringVarP(&reasonFlag, "reason", "r", "", "Human-readable reason for rollback (optional)")
	RootCmd.AddCommand(proposalsCmd)
	RootCmd.AddCommand(approveCmd)
	RootCmd.AddCommand(rejectCmd)
	RootCmd.AddCommand(rollbackCmd)
}
