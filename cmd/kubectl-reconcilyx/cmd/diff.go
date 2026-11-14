package cmd

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Diff manifest cost, carbon, and guardrails against a git base ref",
	Long: `Diff reads Kubernetes Deployment, StatefulSet, and DaemonSet manifests from
a file or directory and compares them with the same files at a git base ref.

The report estimates monthly cost and carbon from pod resource requests, and
flags guardrail problems in the proposed manifests (missing requests, requests
above limits, replica bounds, and optional request-increase caps). YAML is read
from disk for the working tree and from "git show <base>:<path>" for the base.

Use --fail-on cost-increase, --fail-on carbon-increase, or --fail-on guardrail
in CI. A triggered condition exits with code 2; errors exit with code 1.`,
	Example: `  kubectl reconcilyx diff --manifests deploy/ --base origin/main
  kubectl reconcilyx diff --manifests deploy/app.yaml --base HEAD~1 --region aws-eu-west-1 --output json
  kubectl reconcilyx diff --manifests deploy/ --base origin/main --fail-on cost-increase,guardrail`,
	RunE: func(cmd *cobra.Command, args []string) error {
		format, err := resolveOutputFormat(cmd)
		if err != nil {
			return err
		}
		rules, err := parseFailOn(flagString(cmd, "fail-on"))
		if err != nil {
			return err
		}
		rules, err = restrictFailOn(rules, "cost-increase", "carbon-increase", "guardrail")
		if err != nil {
			return err
		}

		minReplicas, _ := cmd.Flags().GetInt32("min-replicas")
		maxReplicas, _ := cmd.Flags().GetInt32("max-replicas")
		if minReplicas < 0 || maxReplicas < 0 {
			return fmt.Errorf("replica bounds cannot be negative")
		}
		if maxReplicas > 0 && minReplicas > maxReplicas {
			return fmt.Errorf("--min-replicas (%d) cannot exceed --max-replicas (%d)", minReplicas, maxReplicas)
		}
		maxIncrease, _ := cmd.Flags().GetFloat64("max-increase-percent")
		if maxIncrease < 0 {
			return fmt.Errorf("--max-increase-percent cannot be negative")
		}

		offline, _ := cmd.Flags().GetBool("offline")
		report, err := buildManifestDiffReportWithOptions(manifestDiffOptions{
			ManifestsPath:      flagString(cmd, "manifests"),
			BaseRef:            flagString(cmd, "base"),
			BaseManifestsPath:  flagString(cmd, "base-manifests"),
			Region:             flagString(cmd, "region"),
			MinReplicas:        minReplicas,
			MaxIncreasePercent: maxIncrease,
			MaxReplicas:        maxReplicas,
			Offline:            offline,
		})
		if err != nil {
			return err
		}

		all, _ := cmd.Flags().GetBool("all")
		switch format {
		case "json":
			if err := writeJSONOutput(cmd.OutOrStdout(), report); err != nil {
				return err
			}
		case "markdown":
			writeDiffMarkdown(cmd.OutOrStdout(), report, all)
		case "sarif":
			if err := writeDiffSARIF(cmd.OutOrStdout(), report); err != nil {
				return err
			}
		default:
			writeDiffReport(cmd, report, all)
		}

		return enforceFailOn(rules, failOnMetrics{
			CostDeltaUSD:        report.Summary.MonthlyCostDeltaUSD,
			CarbonDeltaGrams:    report.Summary.MonthlyCarbonDeltaGrams,
			GuardrailViolations: report.Summary.GuardrailViolations,
		})
	},
}

func restrictFailOn(rules []failOnRule, allowed ...string) ([]failOnRule, error) {
	allowedSet := map[string]bool{}
	for _, name := range allowed {
		allowedSet[name] = true
	}
	for _, rule := range rules {
		kind := rule.kind
		switch kind {
		case "risk":
			kind = "risk>="
		case "confidence":
			kind = "confidence<"
		case "savings":
			kind = "savings-below:"
		}
		if !allowedSet[rule.kind] {
			return nil, fmt.Errorf("--fail-on condition %q is not supported by this command", kind)
		}
	}
	return rules, nil
}

func writeDiffReport(cmd *cobra.Command, report manifestDiffReport, showAll bool) {
	out := cmd.OutOrStdout()
	tw := tabwriter.NewWriter(out, 0, 0, 3, ' ', 0)
	defer func() { _ = tw.Flush() }()
	region := report.Region
	if region == "" {
		region = "global average"
	}
	fmt.Fprintf(tw, "Manifest diff: %s vs %s\n", report.ManifestsPath, report.BaseRef)
	fmt.Fprintf(tw, "Carbon intensity: %s (%.0f g CO2e/kWh)\n", region, report.CarbonIntensityGCO2PerKWh)

	s := report.Summary
	fmt.Fprintf(tw, "Workloads: %d -> %d (%d changed, %d added, %d removed)\n",
		s.WorkloadsBefore, s.WorkloadsAfter, s.Changed, s.Added, s.Removed)
	fmt.Fprintf(tw, "Monthly cost: %s -> %s (%s)\n",
		formatUSD(s.MonthlyCostBeforeUSD), formatUSD(s.MonthlyCostAfterUSD), formatSignedUSD(s.MonthlyCostDeltaUSD))
	fmt.Fprintf(tw, "Monthly carbon: %s -> %s (%s)\n",
		formatCarbon(s.MonthlyCarbonBeforeGrams), formatCarbon(s.MonthlyCarbonAfterGrams), formatSignedCarbon(s.MonthlyCarbonDeltaGrams))
	fmt.Fprintf(tw, "Guardrail violations: %d\n", s.GuardrailViolations)

	rows := 0
	for _, w := range report.Workloads {
		if !showAll && w.Status == "unchanged" {
			continue
		}
		rows++
		if rows == 1 {
			fmt.Fprintln(tw)
			fmt.Fprintln(tw, "STATUS\tWORKLOAD\tREPLICAS\tCPU (POD)\tMEMORY (POD)\tCOST DELTA\tCARBON DELTA")
		}
		beforeCPU, afterCPU := sideCPU(w.Before), sideCPU(w.After)
		beforeMem, afterMem := sideMemory(w.Before), sideMemory(w.After)
		fmt.Fprintf(tw, "%s\t%s %s/%s\t%s -> %s\t%s -> %s\t%s -> %s\t%s\t%s\n",
			w.Status,
			w.Kind, w.Namespace, w.Name,
			sideReplicas(w.Before), sideReplicas(w.After),
			beforeCPU, afterCPU,
			beforeMem, afterMem,
			formatSignedUSD(w.MonthlyCostDeltaUSD),
			formatSignedCarbon(w.CarbonDeltaGrams),
		)
		for _, c := range w.Containers {
			if c.Status == "unchanged" {
				continue
			}
			name := c.Name
			if c.Init {
				name = "init:" + name
			}
			fmt.Fprintf(tw, "  %s\t%s\t\t%s -> %s\t%s -> %s\t\t\n",
				c.Status, name, dash(c.CPUBefore), dash(c.CPUAfter), dash(c.MemoryBefore), dash(c.MemoryAfter))
		}
	}
	if rows == 0 {
		fmt.Fprintln(tw, "\nNo workload changes detected.")
	}

	if len(report.Violations) > 0 {
		fmt.Fprintln(tw, "\nGuardrail violations:")
		for _, v := range report.Violations {
			container := ""
			if v.Container != "" {
				container = " [" + v.Container + "]"
			}
			fmt.Fprintf(tw, "  - %s%s %s: %s\n", v.Workload, container, v.Rule, v.Message)
		}
	}
	if len(report.Warnings) > 0 {
		fmt.Fprintln(tw, "\nWarnings:")
		for _, warning := range report.Warnings {
			fmt.Fprintf(tw, "  - %s\n", warning)
		}
	}
}

func sideCPU(side *workloadSide) string {
	if side == nil {
		return "-"
	}
	return formatCPUMilli(side.PodCPUMilli)
}

func sideMemory(side *workloadSide) string {
	if side == nil {
		return "-"
	}
	return formatMemoryBytes(side.PodMemoryBytes)
}

func sideReplicas(side *workloadSide) string {
	if side == nil {
		return "-"
	}
	if side.ReplicasEstimated {
		return "~1"
	}
	return fmt.Sprintf("%d", side.Replicas)
}

func formatSignedUSD(value float64) string {
	switch {
	case value > 0:
		return "+" + formatUSD(value)
	case value < 0:
		return "-" + formatUSD(-value)
	default:
		return "$0.00"
	}
}

func formatSignedCarbon(grams float64) string {
	switch {
	case grams > 0:
		return "+" + formatCarbon(grams)
	case grams < 0:
		return "-" + formatCarbon(-grams)
	default:
		return "0 g CO2e"
	}
}

func init() {
	diffCmd.Flags().String("manifests", "", "Manifest file or directory to diff (required)")
	diffCmd.Flags().String("base", "HEAD", "Git ref to compare the working tree against")
	diffCmd.Flags().String("base-manifests", "", "Optional baseline manifest file or directory (used instead of git in offline/air-gapped mode)")
	diffCmd.Flags().Bool("offline", false, "Run fully offline using bundled catalog pricing and static regional carbon intensity")
	diffCmd.Flags().String("region", "", "Carbon intensity region (for example aws-eu-west-1); empty uses the global average")
	diffCmd.Flags().Int32("min-replicas", 1, "Minimum replicas before flagging a guardrail violation (0 disables)")
	diffCmd.Flags().Int32("max-replicas", 0, "Maximum replicas before flagging a guardrail violation (0 disables)")
	diffCmd.Flags().Float64("max-increase-percent", 0, "Maximum per-pod CPU/memory request increase before flagging a violation (0 disables)")
	diffCmd.Flags().Bool("all", false, "Include unchanged workloads in the table output")
	addOutputFlag(diffCmd)
	addFailOnFlag(diffCmd, "Comma-separated CI conditions: cost-increase, carbon-increase, guardrail")
	_ = diffCmd.MarkFlagRequired("manifests")
	RootCmd.AddCommand(diffCmd)
}
