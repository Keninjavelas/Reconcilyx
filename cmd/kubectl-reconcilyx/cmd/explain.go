package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/spf13/cobra"
)

type explainResourceRequirements struct {
	Requests map[string]string `json:"requests,omitempty"`
	Limits   map[string]string `json:"limits,omitempty"`
}

type explainLearningResult struct {
	CPUAdjustmentPercent    int                `json:"cpuAdjustmentPercent,omitempty"`
	MemoryAdjustmentPercent int                `json:"memoryAdjustmentPercent,omitempty"`
	CostSavedUSD            *resource.Quantity `json:"costSavedUSD,omitempty"`
	CarbonSavedGramsCO2     *resource.Quantity `json:"carbonSavedGramsCO2,omitempty"`
	Reward                  float64            `json:"reward,omitempty"`
}

type explainLearning struct {
	Results     map[string]explainLearningResult `json:"results,omitempty"`
	Reward      float64                          `json:"reward,omitempty"`
	Outcome     string                           `json:"outcome,omitempty"`
	Finalized   bool                             `json:"finalized,omitempty"`
	SkipReason  string                           `json:"skipReason,omitempty"`
	CompletedAt *time.Time                       `json:"completedAt,omitempty"`
}

type explainExecutionSnapshot struct {
	ReplicaCount       int32                               `json:"replicaCount"`
	Timestamp          *time.Time                          `json:"timestamp,omitempty"`
	ContainerResources map[string]explainSnapshotContainer `json:"containerResources,omitempty"`
}

type explainSnapshotContainer struct {
	Resources explainResourceRequirements `json:"resources,omitempty"`
}

type explainProposal struct {
	Metadata struct {
		Name              string     `json:"name"`
		Namespace         string     `json:"namespace"`
		CreationTimestamp *time.Time `json:"creationTimestamp,omitempty"`
	} `json:"metadata"`
	Spec struct {
		TargetClusterName string `json:"targetClusterName,omitempty"`
		TargetWorkload    struct {
			APIVersion string `json:"apiVersion,omitempty"`
			Kind       string `json:"kind"`
			Namespace  string `json:"namespace,omitempty"`
			Name       string `json:"name"`
		} `json:"targetWorkload"`
		Recommendation    string `json:"recommendation"`
		ObservedResources struct {
			CurrentResources explainResourceRequirements `json:"currentResources"`
			P95Usage         map[string]string           `json:"p95Usage,omitempty"`
		} `json:"observedResources"`
		ProposedResources explainResourceRequirements `json:"proposedResources"`
		ExpectedImpact    struct {
			ResourceDelta       map[string]string  `json:"resourceDelta,omitempty"`
			EstimatedSavingsUSD *resource.Quantity `json:"estimatedSavingsUSD,omitempty"`
			CarbonSavedGramsCO2 *resource.Quantity `json:"carbonSavedGramsCO2,omitempty"`
		} `json:"expectedImpact"`
		DecisionMetadata struct {
			ConfidenceScore int32  `json:"confidenceScore"`
			RiskScore       int32  `json:"riskScore"`
			Rationale       string `json:"rationale,omitempty"`
		} `json:"decisionMetadata"`
	} `json:"spec"`
	Status struct {
		Phase              string                    `json:"phase,omitempty"`
		ApprovalState      string                    `json:"approvalState,omitempty"`
		ExecutionState     string                    `json:"executionState,omitempty"`
		PolicyApplied      string                    `json:"policyApplied,omitempty"`
		Reason             string                    `json:"reason,omitempty"`
		ExternalPRID       string                    `json:"externalPRID,omitempty"`
		LastEvaluationTime *time.Time                `json:"lastEvaluationTime,omitempty"`
		ExpirationTime     *time.Time                `json:"expirationTime,omitempty"`
		ExecutionSnapshot  *explainExecutionSnapshot `json:"executionSnapshot,omitempty"`
		Learning           *explainLearning          `json:"learning,omitempty"`
	} `json:"status"`
}

type explainTarget struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
}

type explainContainerReceipt struct {
	Name                    string  `json:"name"`
	CPUAdjustmentPercent    int     `json:"cpuAdjustmentPercent"`
	MemoryAdjustmentPercent int     `json:"memoryAdjustmentPercent"`
	MonthlySavingsUSD       float64 `json:"monthlySavingsUSD"`
	CarbonGrams             float64 `json:"carbonGrams"`
	Reward                  float64 `json:"reward"`
}

type explainImpact struct {
	MonthlySavingsUSD *float64 `json:"monthlySavingsUSD,omitempty"`
	CarbonGrams       *float64 `json:"carbonGrams,omitempty"`
	CarbonKg          *float64 `json:"carbonKg,omitempty"`
}

type explainRealized struct {
	State             string                    `json:"state"`
	Detail            string                    `json:"detail,omitempty"`
	MonthlySavingsUSD *float64                  `json:"monthlySavingsUSD,omitempty"`
	CarbonGrams       *float64                  `json:"carbonGrams,omitempty"`
	CarbonKg          *float64                  `json:"carbonKg,omitempty"`
	Reward            *float64                  `json:"reward,omitempty"`
	Outcome           string                    `json:"outcome,omitempty"`
	CompletedAt       *time.Time                `json:"completedAt,omitempty"`
	Containers        []explainContainerReceipt `json:"containers,omitempty"`
}

type explainVariance struct {
	MonthlySavingsUSD    *float64 `json:"monthlySavingsUSD,omitempty"`
	MonthlySavingsChange *float64 `json:"monthlySavingsChangePercent,omitempty"`
	CarbonGrams          *float64 `json:"carbonGrams,omitempty"`
	CarbonChange         *float64 `json:"carbonChangePercent,omitempty"`
}

type explainEvent struct {
	Timestamp *time.Time `json:"timestamp,omitempty"`
	Type      string     `json:"type,omitempty"`
	Reason    string     `json:"reason,omitempty"`
	Message   string     `json:"message,omitempty"`
	Count     int32      `json:"count,omitempty"`
}

type explainSnapshot struct {
	Replicas  int32      `json:"replicas"`
	Timestamp *time.Time `json:"timestamp,omitempty"`
}

type explainResources struct {
	CPUBefore    string `json:"cpuBefore,omitempty"`
	CPUAfter     string `json:"cpuAfter,omitempty"`
	CPUDelta     string `json:"cpuDelta,omitempty"`
	MemoryBefore string `json:"memoryBefore,omitempty"`
	MemoryAfter  string `json:"memoryAfter,omitempty"`
	MemoryDelta  string `json:"memoryDelta,omitempty"`
}

type explainReport struct {
	Name               string           `json:"name"`
	Namespace          string           `json:"namespace"`
	Cluster            string           `json:"cluster,omitempty"`
	Target             explainTarget    `json:"target"`
	Recommendation     string           `json:"recommendation"`
	Phase              string           `json:"phase,omitempty"`
	ApprovalState      string           `json:"approvalState,omitempty"`
	ExecutionState     string           `json:"executionState,omitempty"`
	PolicyApplied      string           `json:"policyApplied,omitempty"`
	Reason             string           `json:"reason,omitempty"`
	ExternalPRID       string           `json:"externalPRID,omitempty"`
	RiskScore          int32            `json:"riskScore"`
	ConfidenceScore    int32            `json:"confidenceScore"`
	Rationale          string           `json:"rationale,omitempty"`
	CreatedAt          *time.Time       `json:"createdAt,omitempty"`
	LastEvaluationTime *time.Time       `json:"lastEvaluationTime,omitempty"`
	ExpirationTime     *time.Time       `json:"expirationTime,omitempty"`
	Resources          explainResources `json:"resources"`
	Predicted          explainImpact    `json:"predicted"`
	Realized           explainRealized  `json:"realized"`
	Variance           explainVariance  `json:"variance"`
	Snapshot           *explainSnapshot `json:"snapshot,omitempty"`
	Events             []explainEvent   `json:"events"`
}

var explainCmd = &cobra.Command{
	Use:   "explain [proposal-name]",
	Short: "Explain a proposal with predicted versus realized savings and carbon receipts",
	Long: `Explain fetches a proposal from the Hub and prints its policy/gate path,
resource changes, predicted savings, and the realized learning receipt recorded
after execution. Use --output json for machine-readable reconciliation data.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		format, err := resolveOutputFormat(cmd)
		if err != nil {
			return err
		}
		name := args[0]
		ns, err := resolveProposalNamespace(name)
		if err != nil {
			return err
		}
		path := fmt.Sprintf("/api/proposals/%s?namespace=%s", url.PathEscape(name), url.QueryEscape(ns))
		body, status, err := doAPI(http.MethodGet, path, nil)
		if err != nil {
			return fmt.Errorf("explain failed: cannot reach Reconcilyx API at %s: %w", apiServerURL, err)
		}
		if status < 200 || status >= 300 {
			hint := ""
			if status == http.StatusNotFound {
				hint = " (use -n <namespace> if the proposal lives outside 'default')"
			}
			return fmt.Errorf("explain failed (HTTP %d): %s%s", status, extractAPIMessage(body), hint)
		}

		report, err := buildExplainReport(body)
		if err != nil {
			return err
		}
		switch format {
		case "json":
			return writeJSONOutput(cmd.OutOrStdout(), report)
		case "markdown":
			writeExplainMarkdown(cmd.OutOrStdout(), report)
			return nil
		case "sarif":
			return writeExplainSARIF(cmd.OutOrStdout(), report)
		default:
			writeExplainReport(cmd, report)
			return nil
		}
	},
}

func resolveProposalNamespace(name string) (string, error) {
	if namespace != "" {
		return namespace, nil
	}
	body, status, err := doAPI(http.MethodGet, "/api/proposals", nil)
	if err != nil {
		return "", fmt.Errorf("explain failed: cannot reach Reconcilyx API at %s: %w", apiServerURL, err)
	}
	if status < 200 || status >= 300 {
		return "", fmt.Errorf("explain failed (HTTP %d): %s", status, extractAPIMessage(body))
	}
	var env proposalListResponse
	if err := json.Unmarshal(body, &env); err != nil {
		return "", fmt.Errorf("explain failed: cannot parse Hub response: %w", err)
	}
	matches := []string{}
	for _, item := range env.Items {
		if item.Name == name {
			matches = append(matches, item.Namespace)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("proposal %q not found; pass -n <namespace> if it has not been listed yet", name)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("proposal %q exists in multiple namespaces (%s); pass -n <namespace>", name, strings.Join(matches, ", "))
	}
}

func buildExplainReport(body []byte) (explainReport, error) {
	var detail proposalDetailResponse
	if err := json.Unmarshal(body, &detail); err != nil {
		return explainReport{}, fmt.Errorf("explain failed: cannot parse proposal response: %w", err)
	}
	if len(detail.Proposal) == 0 {
		return explainReport{}, fmt.Errorf("explain failed: Hub response did not include a proposal")
	}
	var p explainProposal
	if err := json.Unmarshal(detail.Proposal, &p); err != nil {
		return explainReport{}, fmt.Errorf("explain failed: cannot parse proposal object: %w", err)
	}

	report := explainReport{
		Name:               p.Metadata.Name,
		Namespace:          p.Metadata.Namespace,
		Cluster:            p.Spec.TargetClusterName,
		Target:             explainTarget{Kind: p.Spec.TargetWorkload.Kind, Namespace: p.Spec.TargetWorkload.Namespace, Name: p.Spec.TargetWorkload.Name},
		Recommendation:     p.Spec.Recommendation,
		Phase:              p.Status.Phase,
		ApprovalState:      p.Status.ApprovalState,
		ExecutionState:     p.Status.ExecutionState,
		PolicyApplied:      p.Status.PolicyApplied,
		Reason:             p.Status.Reason,
		ExternalPRID:       p.Status.ExternalPRID,
		RiskScore:          p.Spec.DecisionMetadata.RiskScore,
		ConfidenceScore:    p.Spec.DecisionMetadata.ConfidenceScore,
		Rationale:          p.Spec.DecisionMetadata.Rationale,
		CreatedAt:          p.Metadata.CreationTimestamp,
		LastEvaluationTime: p.Status.LastEvaluationTime,
		ExpirationTime:     p.Status.ExpirationTime,
		Events:             make([]explainEvent, 0, len(detail.Events)),
	}

	current := p.Spec.ObservedResources.CurrentResources
	proposed := p.Spec.ProposedResources
	report.Resources = explainResources{
		CPUBefore:    resourceString(current, "cpu"),
		CPUAfter:     resourceString(proposed, "cpu"),
		CPUDelta:     resourceDeltaString(current, proposed, "cpu"),
		MemoryBefore: resourceString(current, "memory"),
		MemoryAfter:  resourceString(proposed, "memory"),
		MemoryDelta:  resourceDeltaString(current, proposed, "memory"),
	}

	if q := p.Spec.ExpectedImpact.EstimatedSavingsUSD; q != nil {
		v := q.AsApproximateFloat64()
		report.Predicted.MonthlySavingsUSD = &v
	}
	if q := p.Spec.ExpectedImpact.CarbonSavedGramsCO2; q != nil {
		grams := q.AsApproximateFloat64()
		kg := grams / 1000
		report.Predicted.CarbonGrams = &grams
		report.Predicted.CarbonKg = &kg
	}

	report.Realized = buildRealizedReceipt(p.Status.Learning)
	if p.Status.ExecutionSnapshot != nil {
		report.Snapshot = &explainSnapshot{
			Replicas:  p.Status.ExecutionSnapshot.ReplicaCount,
			Timestamp: p.Status.ExecutionSnapshot.Timestamp,
		}
	}

	for _, event := range detail.Events {
		report.Events = append(report.Events, explainEvent{
			Timestamp: event.timestamp(),
			Type:      event.Type,
			Reason:    event.Reason,
			Message:   event.Message,
			Count:     event.Count,
		})
	}
	sort.SliceStable(report.Events, func(i, j int) bool {
		if report.Events[i].Timestamp == nil {
			return false
		}
		if report.Events[j].Timestamp == nil {
			return true
		}
		return report.Events[i].Timestamp.Before(*report.Events[j].Timestamp)
	})

	report.Variance = computeVariance(report.Predicted, report.Realized)
	return report, nil
}

func buildRealizedReceipt(learning *explainLearning) explainRealized {
	receipt := explainRealized{State: "missing", Detail: "no learning receipt recorded (execution has not completed or learning is disabled)"}
	if learning == nil {
		return receipt
	}
	if learning.SkipReason != "" {
		receipt.State = "skipped"
		receipt.Detail = learning.SkipReason
		return receipt
	}
	if !learning.Finalized {
		receipt.State = "pending"
		receipt.Detail = "learning receipt is not finalized yet"
		return receipt
	}

	receipt.State = "finalized"
	receipt.Outcome = learning.Outcome
	receipt.CompletedAt = learning.CompletedAt
	reward := learning.Reward
	receipt.Reward = &reward

	names := make([]string, 0, len(learning.Results))
	for name := range learning.Results {
		names = append(names, name)
	}
	sort.Strings(names)

	totalCost := 0.0
	totalCarbon := 0.0
	for _, name := range names {
		result := learning.Results[name]
		cost := 0.0
		if result.CostSavedUSD != nil {
			cost = result.CostSavedUSD.AsApproximateFloat64()
		}
		carbon := 0.0
		if result.CarbonSavedGramsCO2 != nil {
			carbon = result.CarbonSavedGramsCO2.AsApproximateFloat64()
		}
		totalCost += cost
		totalCarbon += carbon
		receipt.Containers = append(receipt.Containers, explainContainerReceipt{
			Name:                    name,
			CPUAdjustmentPercent:    result.CPUAdjustmentPercent,
			MemoryAdjustmentPercent: result.MemoryAdjustmentPercent,
			MonthlySavingsUSD:       cost,
			CarbonGrams:             carbon,
			Reward:                  result.Reward,
		})
	}
	kg := totalCarbon / 1000
	receipt.MonthlySavingsUSD = &totalCost
	receipt.CarbonGrams = &totalCarbon
	receipt.CarbonKg = &kg
	return receipt
}

func computeVariance(predicted explainImpact, realized explainRealized) explainVariance {
	var variance explainVariance
	if predicted.MonthlySavingsUSD != nil && realized.MonthlySavingsUSD != nil {
		delta := *realized.MonthlySavingsUSD - *predicted.MonthlySavingsUSD
		variance.MonthlySavingsUSD = &delta
		if *predicted.MonthlySavingsUSD != 0 {
			pct := delta / *predicted.MonthlySavingsUSD * 100
			variance.MonthlySavingsChange = &pct
		}
	}
	if predicted.CarbonGrams != nil && realized.CarbonGrams != nil {
		delta := *realized.CarbonGrams - *predicted.CarbonGrams
		variance.CarbonGrams = &delta
		if *predicted.CarbonGrams != 0 {
			pct := delta / *predicted.CarbonGrams * 100
			variance.CarbonChange = &pct
		}
	}
	return variance
}

func writeExplainReport(cmd *cobra.Command, r explainReport) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Proposal: %s (%s)\n", r.Name, r.Namespace)
	fmt.Fprintf(out, "Target: %s %s/%s\n", r.Target.Kind, r.Target.Namespace, r.Target.Name)
	fmt.Fprintf(out, "Recommendation: %s\n", r.Recommendation)
	fmt.Fprintf(out, "Lifecycle: %s | approval %s | execution %s\n", dash(r.Phase), dash(r.ApprovalState), dash(r.ExecutionState))
	fmt.Fprintf(out, "Gate path: policy %s -> approval %s -> execution %s\n", quoteOrNone(r.PolicyApplied), dash(r.ApprovalState), dash(r.ExecutionState))
	fmt.Fprintf(out, "Risk %d | confidence %d\n", r.RiskScore, r.ConfidenceScore)
	if r.Rationale != "" {
		fmt.Fprintf(out, "Rationale: %s\n", r.Rationale)
	}
	if r.Reason != "" {
		fmt.Fprintf(out, "Reason: %s\n", r.Reason)
	}
	if r.Cluster != "" {
		fmt.Fprintf(out, "Target cluster: %s\n", r.Cluster)
	}
	if r.ExternalPRID != "" {
		fmt.Fprintf(out, "External PR/MR: %s\n", r.ExternalPRID)
	}

	if r.Resources.CPUBefore != "" || r.Resources.CPUAfter != "" || r.Resources.MemoryBefore != "" || r.Resources.MemoryAfter != "" {
		fmt.Fprintln(out, "Resources:")
		printResourceLine(out, "  CPU", r.Resources.CPUBefore, r.Resources.CPUAfter, r.Resources.CPUDelta)
		printResourceLine(out, "  Memory", r.Resources.MemoryBefore, r.Resources.MemoryAfter, r.Resources.MemoryDelta)
	}

	fmt.Fprintf(out, "Predicted: %s | %s\n", impactUSD(r.Predicted.MonthlySavingsUSD), impactCarbon(r.Predicted.CarbonGrams))
	fmt.Fprintf(out, "Realized:  %s | %s\n", realizedCost(r.Realized), realizedCarbon(r.Realized))
	if r.Realized.Detail != "" {
		fmt.Fprintf(out, "  Receipt: %s (%s)\n", r.Realized.State, r.Realized.Detail)
	}
	if r.Realized.Outcome != "" {
		fmt.Fprintf(out, "  Outcome: %s", r.Realized.Outcome)
		if r.Realized.Reward != nil {
			fmt.Fprintf(out, " | reward %.3f", *r.Realized.Reward)
		}
		if r.Realized.CompletedAt != nil {
			fmt.Fprintf(out, " | completed %s", r.Realized.CompletedAt.Format(time.RFC3339))
		}
		fmt.Fprintln(out)
	}
	if r.Variance.MonthlySavingsUSD != nil || r.Variance.CarbonGrams != nil {
		fmt.Fprintf(out, "Variance (realized - predicted): %s | %s\n", varianceUSD(r.Variance), varianceCarbon(r.Variance))
	}
	if r.Snapshot != nil {
		ts := "-"
		if r.Snapshot.Timestamp != nil {
			ts = r.Snapshot.Timestamp.Format(time.RFC3339)
		}
		fmt.Fprintf(out, "Snapshot: %d replicas at %s\n", r.Snapshot.Replicas, ts)
	}
	if len(r.Realized.Containers) > 0 {
		fmt.Fprintln(out, "Container receipts:")
		for _, c := range r.Realized.Containers {
			fmt.Fprintf(out, "  %s: CPU %+d%%, memory %+d%%, %s/mo, %s, reward %.3f\n",
				c.Name, c.CPUAdjustmentPercent, c.MemoryAdjustmentPercent,
				formatUSD(c.MonthlySavingsUSD), formatCarbon(c.CarbonGrams), c.Reward)
		}
	}
	if len(r.Events) > 0 {
		fmt.Fprintln(out, "Events:")
		start := 0
		if len(r.Events) > 10 {
			start = len(r.Events) - 10
		}
		for _, e := range r.Events[start:] {
			ts := "-"
			if e.Timestamp != nil {
				ts = e.Timestamp.Format(time.RFC3339)
			}
			fmt.Fprintf(out, "  %s  %s/%s  %s\n", ts, dash(e.Type), dash(e.Reason), e.Message)
		}
		if start > 0 {
			fmt.Fprintf(out, "  (%d older events omitted)\n", start)
		}
	}
}

func printResourceLine(out io.Writer, label, before, after, delta string) {
	fmt.Fprintf(out, "%s: %s -> %s", label, dash(before), dash(after))
	if delta != "" {
		fmt.Fprintf(out, " (delta %s)", delta)
	}
	fmt.Fprintln(out)
}

func resourceString(reqs explainResourceRequirements, name string) string {
	if v, ok := reqs.Requests[name]; ok && v != "" {
		return v
	}
	if v, ok := reqs.Limits[name]; ok && v != "" {
		return v
	}
	return ""
}

func resourceDeltaString(before, after explainResourceRequirements, name string) string {
	b := resourceString(before, name)
	a := resourceString(after, name)
	if b == "" || a == "" {
		return ""
	}
	qb, err := resource.ParseQuantity(b)
	if err != nil {
		return ""
	}
	qa, err := resource.ParseQuantity(a)
	if err != nil {
		return ""
	}
	qb.Sub(qa)
	return qb.String()
}

func quoteOrNone(value string) string {
	if value == "" {
		return "(none recorded)"
	}
	return fmt.Sprintf("%q", value)
}

func dash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func formatUSD(value float64) string {
	if value < 0 {
		return fmt.Sprintf("-$%.2f", -value)
	}
	return fmt.Sprintf("$%.2f", value)
}

func formatCarbon(grams float64) string {
	switch {
	case grams >= 1_000_000:
		return fmt.Sprintf("%.2f t CO2e", grams/1_000_000)
	case grams >= 1000:
		return fmt.Sprintf("%.2f kg CO2e", grams/1000)
	default:
		return fmt.Sprintf("%.0f g CO2e", grams)
	}
}

func impactUSD(v *float64) string {
	if v == nil {
		return "savings unavailable"
	}
	return formatUSD(*v) + "/mo"
}

func impactCarbon(v *float64) string {
	if v == nil {
		return "carbon unavailable"
	}
	return formatCarbon(*v) + "/mo"
}

func realizedCost(r explainRealized) string {
	if r.MonthlySavingsUSD == nil {
		return "receipt " + r.State
	}
	return formatUSD(*r.MonthlySavingsUSD) + "/mo"
}

func realizedCarbon(r explainRealized) string {
	if r.CarbonGrams == nil {
		return "receipt " + r.State
	}
	return formatCarbon(*r.CarbonGrams) + "/mo"
}

func varianceUSD(v explainVariance) string {
	if v.MonthlySavingsUSD == nil {
		return "-"
	}
	s := formatUSD(*v.MonthlySavingsUSD) + "/mo"
	if v.MonthlySavingsChange != nil {
		s += fmt.Sprintf(" (%+.1f%%)", *v.MonthlySavingsChange)
	}
	return s
}

func varianceCarbon(v explainVariance) string {
	if v.CarbonGrams == nil {
		return "-"
	}
	s := formatCarbon(*v.CarbonGrams) + "/mo"
	if v.CarbonChange != nil {
		s += fmt.Sprintf(" (%+.1f%%)", *v.CarbonChange)
	}
	return s
}

func init() {
	addOutputFlag(explainCmd)
	RootCmd.AddCommand(explainCmd)
}
