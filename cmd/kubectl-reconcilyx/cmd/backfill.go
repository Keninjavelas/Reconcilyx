package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	backfillFrom   string
	backfillTo     string
	backfillStep   string
	backfillOutput string
)

type backfillCLIResp struct {
	Namespace               string    `json:"namespace"`
	From                    time.Time `json:"from"`
	To                      time.Time `json:"to"`
	Step                    string    `json:"step"`
	WindowsEvaluated        int       `json:"windowsEvaluated"`
	BucketsIngested         int       `json:"bucketsIngested"`
	BucketsSkippedDuplicate int       `json:"bucketsSkippedDuplicate"`
	NewWatermark            time.Time `json:"newWatermark"`
}

var backfillCmd = &cobra.Command{
	Use:   "backfill",
	Short: "Reconstruct missed Prometheus telemetry windows idempotently without double-counting",
	RunE: func(cmd *cobra.Command, args []string) error {
		fromTime, err := time.Parse(time.RFC3339, strings.TrimSpace(backfillFrom))
		if err != nil {
			return fmt.Errorf("invalid --from timestamp (expected RFC3339): %w", err)
		}
		toTime, err := time.Parse(time.RFC3339, strings.TrimSpace(backfillTo))
		if err != nil {
			return fmt.Errorf("invalid --to timestamp (expected RFC3339): %w", err)
		}
		if !fromTime.Before(toTime) {
			return fmt.Errorf("--from (%s) must be earlier than --to (%s)", fromTime.Format(time.RFC3339), toTime.Format(time.RFC3339))
		}

		ns := strings.TrimSpace(namespace)
		if ns == "" {
			ns, _ = cmd.Flags().GetString("namespace")
		}
		payload := map[string]string{
			"namespace": ns,
			"from":      fromTime.UTC().Format(time.RFC3339),
			"to":        toTime.UTC().Format(time.RFC3339),
		}
		if s := strings.TrimSpace(backfillStep); s != "" {
			payload["step"] = s
		}
		raw, status, err := doAPI(http.MethodPost, "/api/v1/telemetry/backfill", payload)
		if err != nil {
			return fmt.Errorf("backfill request failed: %w", err)
		}
		if status < 200 || status >= 300 {
			return fmt.Errorf("backfill failed (HTTP %d): %s", status, extractAPIMessage(raw))
		}

		if strings.EqualFold(strings.TrimSpace(backfillOutput), "json") {
			fmt.Println(string(raw))
			return nil
		}

		var rep backfillCLIResp
		if err := json.Unmarshal(raw, &rep); err != nil {
			return fmt.Errorf("failed to parse backfill response: %w", err)
		}
		fmt.Printf("Reconcilyx Telemetry Backfill [%s]\n", rep.Namespace)
		fmt.Printf("  Range:              %s -> %s (step=%s)\n", rep.From.Format(time.RFC3339), rep.To.Format(time.RFC3339), rep.Step)
		fmt.Printf("  Windows evaluated:  %d\n", rep.WindowsEvaluated)
		fmt.Printf("  Buckets ingested:   %d\n", rep.BucketsIngested)
		fmt.Printf("  Skipped duplicates: %d\n", rep.BucketsSkippedDuplicate)
		fmt.Printf("  New watermark:      %s\n", rep.NewWatermark.Format(time.RFC3339))
		return nil
	},
}

func init() {
	backfillCmd.Flags().StringVar(&backfillFrom, "from", "", "Start timestamp in RFC3339 format (required)")
	backfillCmd.Flags().StringVar(&backfillTo, "to", "", "End timestamp in RFC3339 format (required)")
	backfillCmd.Flags().StringVar(&backfillStep, "step", "5m", "PromQL range evaluation step duration (e.g. 5m)")
	backfillCmd.Flags().StringVarP(&backfillOutput, "output", "o", "table", "Output format: table or json")
	_ = backfillCmd.MarkFlagRequired("from")
	_ = backfillCmd.MarkFlagRequired("to")
	RootCmd.AddCommand(backfillCmd)
}
