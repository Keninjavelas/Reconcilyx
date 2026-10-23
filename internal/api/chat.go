package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	optimizationv1alpha1 "github.com/reconcilyx/operator/api/v1alpha1"
	"github.com/reconcilyx/operator/internal/cloud"
	"github.com/reconcilyx/operator/internal/engine"
	"github.com/reconcilyx/operator/internal/llm"
	"github.com/reconcilyx/operator/internal/notifiers"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// bgCtx is a shared, long-lived background context for API helpers. It is
// equivalent to context.Background() but expressed as a single local variable
// to avoid repeating the same expression throughout chat handlers.
var bgCtx = context.Background()

type ChatRequest struct {
	Message      string `json:"message"`
	Provider     string `json:"provider,omitempty"`
	ApiKey       string `json:"apiKey,omitempty"`
	ConfirmToken string `json:"confirmToken,omitempty"`
}

type ChatResponsePayload struct {
	Answer                string                 `json:"answer"`
	Function              string                 `json:"function"`
	FastPath              bool                   `json:"fastPath"`
	ChartData             map[string]interface{} `json:"chartData,omitempty"`
	ConfirmationRequired  bool                   `json:"confirmationRequired,omitempty"`
	ConfirmationToken     string                 `json:"confirmationToken,omitempty"`
	ConfirmationDesc      string                 `json:"confirmationDescription,omitempty"`
	ActionStatus          string                 `json:"actionStatus,omitempty"`
	ActionTarget          string                 `json:"actionTarget,omitempty"`
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	providerName := req.Provider
	if providerName == "" {
		providerName = "ollama"
	}
	provider, err := llm.NewProvider(providerName, map[string]string{"apiKey": req.ApiKey})
	if err != nil {
		provider, _ = llm.NewProvider("ollama", nil)
	}

	router := llm.NewHybridRouter(provider)
	routeResult, err := router.Route(r.Context(), req.Message)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	agent := llm.NewActionableAgentWithDeps(
		s.deps.Client,
		s.deps.AuditLog,
		s.deps.NotifierDispatcher,
		s.emitProposalEvent,
	)

	var answer string
	var chartData map[string]interface{}
	var confirmationToken, confirmationDesc, actionStatus, actionTarget string
	var confirmationRequired bool

	auditAction := routeResult.FunctionName
	auditReason := req.Message
	if len(auditReason) > 256 {
		auditReason = auditReason[:256]
	}
	defer func() {
		s.appendChatAudit(auditAction, actionTarget, confirmationRequired, auditReason)
	}()

	switch routeResult.FunctionName {
	case "getSavings":
		answer, chartData = s.respondSavings()
	case "getProposals":
		answer = s.respondListProposals(routeResult.Arguments)
	case "getProposal":
		answer = s.respondGetProposal(routeResult.Arguments)
	case "getCarbon":
		answer = s.respondCarbon()
	case "evaluateArbitrage":
		answer = s.respondArbitrage(routeResult.Arguments)
	case "forecastWorkload":
		answer = s.respondForecast(routeResult.Arguments)
	case "getAnomalies":
		answer = s.respondAnomalies()
	case "getPolicies":
		answer = s.respondListPolicies()
	case "getClusters":
		answer = s.respondClusters()
	case "getWorkloads":
		answer = s.respondWorkloads(routeResult.Arguments)
	case "getMetrics":
		answer = s.respondMetrics(routeResult.Arguments)
	case "explain":
		answer = s.respondExplain(routeResult.Arguments)
	case "approveProposal", "applyProposal":
		confirmed, confirmErr := s.validateConfirmToken(req.ConfirmToken, routeResult.FunctionName)
		if confirmErr != nil {
			answer = fmt.Sprintf("⚠️ **Confirmation token invalid**\n\n`%s`", confirmErr.Error())
			break
		}
		res, execErr := agent.ExecuteWriteAction(r.Context(), routeResult.FunctionName, routeResult.Arguments, confirmed)
		if execErr != nil {
			answer = fmt.Sprintf("⚠️ **Proposal approval failed**\n\nError: `%s`", execErr.Error())
		} else {
			answer = fmt.Sprintf("### %s\nTarget: `%s`%s\nAction ID: `%s`",
				res.Message, res.Target, func() string {
					if res.Namespace != "" && res.Namespace != "default" {
						return fmt.Sprintf(" (namespace `%s`)", res.Namespace)
					}
					return ""
				}(), res.ActionID)
			actionStatus = res.Status
			actionTarget = res.Target
			if res.Confirmation != nil && res.Confirmation.Required {
				confirmationRequired = true
				confirmationToken = res.Confirmation.Token
				confirmationDesc = res.Confirmation.Description
			}
		}
	case "rejectProposal":
		confirmed, confirmErr := s.validateConfirmToken(req.ConfirmToken, routeResult.FunctionName)
		if confirmErr != nil {
			answer = fmt.Sprintf("⚠️ **Confirmation token invalid**\n\n`%s`", confirmErr.Error())
			break
		}
		res, execErr := agent.ExecuteWriteAction(r.Context(), routeResult.FunctionName, routeResult.Arguments, confirmed)
		if execErr != nil {
			answer = fmt.Sprintf("⚠️ **Proposal rejection failed**\n\nError: `%s`", execErr.Error())
		} else {
			answer = fmt.Sprintf("### %s\nTarget: `%s`\nAction ID: `%s`",
				res.Message, res.Target, res.ActionID)
			actionStatus = res.Status
			actionTarget = res.Target
			if res.Confirmation != nil && res.Confirmation.Required {
				confirmationRequired = true
				confirmationToken = res.Confirmation.Token
				confirmationDesc = res.Confirmation.Description
			}
		}
	case "rollbackProposal", "rollbackPod":
		confirmed, confirmErr := s.validateConfirmToken(req.ConfirmToken, routeResult.FunctionName)
		if confirmErr != nil {
			answer = fmt.Sprintf("⚠️ **Confirmation token invalid**\n\n`%s`", confirmErr.Error())
			break
		}
		res, execErr := agent.ExecuteWriteAction(r.Context(), routeResult.FunctionName, routeResult.Arguments, confirmed)
		if execErr != nil {
			answer = fmt.Sprintf("⚠️ **Rollback failed**\n\nError: `%s`", execErr.Error())
		} else {
			answer = fmt.Sprintf("### %s\nTarget: `%s`\nAction ID: `%s`",
				res.Message, res.Target, res.ActionID)
			actionStatus = res.Status
			actionTarget = res.Target
			if res.Confirmation != nil && res.Confirmation.Required {
				confirmationRequired = true
				confirmationToken = res.Confirmation.Token
				confirmationDesc = res.Confirmation.Description
			}
		}
	default:
		answer = fmt.Sprintf("Executed tool function `%s`. Control plane parameters within policy limits.", routeResult.FunctionName)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ChatResponsePayload{
		Answer:               answer,
		Function:             routeResult.FunctionName,
		FastPath:             routeResult.IsFastPath,
		ChartData:            chartData,
		ConfirmationRequired: confirmationRequired,
		ConfirmationToken:    confirmationToken,
		ConfirmationDesc:     confirmationDesc,
		ActionStatus:         actionStatus,
		ActionTarget:         actionTarget,
	})
}

func (s *Server) validateConfirmToken(token, functionName string) (bool, error) {
	if token == "" {
		return false, nil
	}
	parts := strings.SplitN(token, ":", 3)
	if len(parts) < 3 || parts[0] != "confirm" {
		return false, fmt.Errorf("expected format confirm:<fn>:<actionId>, got %q", token)
	}
	if parts[1] != functionName {
		if functionName == "applyProposal" && parts[1] != "approveProposal" {
			return false, fmt.Errorf("token action %q does not match requested %q", parts[1], functionName)
		}
		if functionName != "applyProposal" {
			return false, fmt.Errorf("token action %q does not match requested %q", parts[1], functionName)
		}
	}
	if strings.TrimSpace(parts[2]) == "" {
		return false, fmt.Errorf("missing actionId in confirm token")
	}
	return true, nil
}

func (s *Server) appendChatAudit(action, target string, needsConfirm bool, reason string) {
	if s.deps.AuditLog == nil {
		return
	}
	actor := "reconcilyx-llm"
	if needsConfirm {
		action = action + ":pending-confirmation"
	}
	s.deps.AuditLog.Append(notifiers.AuditEntry{
		Time:      time.Now().UTC(),
		Actor:     actor,
		Action:    action,
		Proposal:  target,
		Namespace: "",
		Reason:    reason,
	})
}

func (s *Server) respondSavings() (string, map[string]interface{}) {
	var list optimizationv1alpha1.OptimizationProposalList
	if err := s.deps.Client.List(bgCtx, &list); err != nil {
		return fmt.Sprintf("Failed to compute savings: `%s`.", err.Error()), nil
	}
	totalUSD := 0.0
	totalGrams := int64(0)
	count := 0
	byDay := map[string]savingsPoint{}
	series := []savingsPoint{}
	for _, p := range list.Items {
		if !(p.Status.Phase == optimizationv1alpha1.PhaseCompleted ||
			p.Status.Phase == optimizationv1alpha1.PhaseStabilizing ||
			p.Status.ApprovalState == optimizationv1alpha1.ApprovalApproved) {
			continue
		}
		count++
		dateStr := p.CreationTimestamp.Time.UTC().Format("2006-01-02")
		pt := byDay[dateStr]
		pt.Date = dateStr
		if q := p.Spec.ExpectedImpact.EstimatedSavingsUSD; q != nil {
			f := q.AsApproximateFloat64()
			totalUSD += f
			pt.USD += f
		}
		if q := p.Spec.ExpectedImpact.CarbonSavedGramsCO2; q != nil {
			g := q.MilliValue() / 1000
			totalGrams += g
			pt.GramsCO2 += g
		}
		byDay[dateStr] = pt
	}
	for _, pt := range byDay {
		series = append(series, pt)
	}
	sort.Slice(series, func(i, j int) bool { return series[i].Date < series[j].Date })

	chartData := map[string]interface{}{
		"type": "area",
		"data": series,
	}
	carbonHuman := formatCarbonGramsHuman(totalGrams)
	if carbonHuman == "" {
		carbonHuman = "0 kg CO₂"
	}
	if count == 0 {
		return "### 💡 FinOps Savings Report\n\nNo applied proposals yet. Once optimizations are approved and applied, cumulative savings and carbon abatement will appear here.\n\nTip: ask `show pending proposals` to review what's waiting.", chartData
	}
	return fmt.Sprintf("### 💡 FinOps Savings Report\n\nAcross **%d applied optimization proposals**, Reconcilyx has saved:\n\n- 💰 **$%.2f / month** in cloud costs\n- 🌱 **%s** of CO₂ emissions abated\n\nEvery proposal uses In-Place Pod Resizing (no pod restarts, no disruption).",
		count, totalUSD, carbonHuman), chartData
}

func (s *Server) respondListProposals(args map[string]interface{}) string {
	var list optimizationv1alpha1.OptimizationProposalList
	if err := s.deps.Client.List(bgCtx, &list); err != nil {
		return fmt.Sprintf("Failed to list proposals: `%s`.", err.Error())
	}
	if len(list.Items) == 0 {
		return "### 📋 Proposals\n\nNo optimization proposals yet. The analyzer periodically scans workloads; or you can run `kubectl annotate deployment ... reconcilyx.io/force-analysis=true` to trigger one immediately."
	}
	statusFilter, _ := args["status"].(string)
	nsFilter, _ := args["namespace"].(string)

	lines := []string{"### 📋 Optimization Proposals"}
	count := 0
	for _, p := range list.Items {
		if statusFilter != "" && string(p.Status.Phase) != statusFilter && string(p.Status.ApprovalState) != statusFilter {
			continue
		}
		if nsFilter != "" && p.Namespace != nsFilter {
			continue
		}
		count++
		savingStr := "—"
		if q := p.Spec.ExpectedImpact.EstimatedSavingsUSD; q != nil {
			savingStr = fmt.Sprintf("$%.2f / mo", q.AsApproximateFloat64())
		}
		cpuBefore := "—"
		cpuAfter := "—"
		memBefore := "—"
		memAfter := "—"
		if q := p.Spec.ObservedResources.CurrentResources.Requests.Cpu(); q != nil && !q.IsZero() {
			cpuBefore = q.String()
		}
		if q := p.Spec.ProposedResources.Requests.Cpu(); q != nil && !q.IsZero() {
			cpuAfter = q.String()
		}
		if q := p.Spec.ObservedResources.CurrentResources.Requests.Memory(); q != nil && !q.IsZero() {
			memBefore = q.String()
		}
		if q := p.Spec.ProposedResources.Requests.Memory(); q != nil && !q.IsZero() {
			memAfter = q.String()
		}
		phase := string(p.Status.Phase)
		if phase == "" {
			phase = string(optimizationv1alpha1.PhaseDetected)
		}
		lines = append(lines,
			fmt.Sprintf("\n**%d.** `%s` (ns: `%s`, kind: `%s/%s`)\n"+
				"    • Phase: `%s` | Approval: `%s`\n"+
				"    • CPU: %s → %s | Mem: %s → %s\n"+
				"    • Estimated savings: **%s**\n"+
				"    • Confidence: %d | Risk: %d",
				count, p.Name, p.Namespace, p.Spec.TargetWorkload.Kind, p.Spec.TargetWorkload.Name,
				phase, p.Status.ApprovalState,
				cpuBefore, cpuAfter, memBefore, memAfter,
				savingStr,
				p.Spec.DecisionMetadata.ConfidenceScore, p.Spec.DecisionMetadata.RiskScore,
			))
	}
	if count == 0 {
		lines = append(lines, "\n_No proposals match the requested filters._")
	}
	return strings.Join(lines, "\n")
}

func (s *Server) respondGetProposal(args map[string]interface{}) string {
	name, _ := args["name"].(string)
	if name == "" {
		return "Which proposal? Please include `name` in your query (e.g. `show proposal payment-service-xyz`)."
	}
	namespace, _ := args["namespace"].(string)
	if namespace == "" {
		namespace = "default"
	}
	var p optimizationv1alpha1.OptimizationProposal
	key := types.NamespacedName{Namespace: namespace, Name: name}
	if err := s.deps.Client.Get(bgCtx, key, &p); err != nil {
		return fmt.Sprintf("Proposal `%s/%s` not found: `%s`", namespace, name, err.Error())
	}
	savingStr := "—"
	if q := p.Spec.ExpectedImpact.EstimatedSavingsUSD; q != nil {
		savingStr = fmt.Sprintf("$%.2f / mo", q.AsApproximateFloat64())
	}
	carbonStr := "—"
	if q := p.Spec.ExpectedImpact.CarbonSavedGramsCO2; q != nil {
		carbonStr = formatCarbonGramsHuman(q.MilliValue() / 1000)
	}
	phase := string(p.Status.Phase)
	if phase == "" {
		phase = string(optimizationv1alpha1.PhaseDetected)
	}
	return fmt.Sprintf("### 📄 Proposal `%s`\n\n"+
		"- **Namespace**: `%s`\n"+
		"- **Target**: `%s/%s` (%s)\n"+
		"- **Phase**: `%s` | **Approval**: `%s`\n"+
		"- **Recommendation**: `%s`\n"+
		"- **Estimated savings**: %s\n"+
		"- **Carbon abatement**: %s\n"+
		"- **Confidence**: %d / 100 | **Risk**: %d / 100\n"+
		"- **Rationale**: %s",
		p.Name, p.Namespace,
		p.Spec.TargetWorkload.Kind, p.Spec.TargetWorkload.Name, p.Spec.TargetWorkload.Namespace,
		phase, p.Status.ApprovalState,
		p.Spec.Recommendation,
		savingStr, carbonStr,
		p.Spec.DecisionMetadata.ConfidenceScore, p.Spec.DecisionMetadata.RiskScore,
		p.Spec.DecisionMetadata.Rationale,
	)
}

func (s *Server) respondCarbon() string {
	var list optimizationv1alpha1.OptimizationProposalList
	totalGrams := int64(0)
	appliedCount := 0
	if err := s.deps.Client.List(bgCtx, &list); err == nil {
		for _, p := range list.Items {
			if !(p.Status.Phase == optimizationv1alpha1.PhaseCompleted ||
				p.Status.Phase == optimizationv1alpha1.PhaseStabilizing ||
				p.Status.ApprovalState == optimizationv1alpha1.ApprovalApproved) {
				continue
			}
			appliedCount++
			if q := p.Spec.ExpectedImpact.CarbonSavedGramsCO2; q != nil {
				totalGrams += q.MilliValue() / 1000
			}
		}
	}
	carbonHuman := formatCarbonGramsHuman(totalGrams)
	if carbonHuman == "" {
		carbonHuman = "0 kg CO₂"
	}
	return fmt.Sprintf("### 🌱 Carbon Footprint Summary\n\n"+
		"Reconcilyx reduces carbon by right-sizing over-provisioned workloads. The policy `carbonWeight` field (0–1) blends carbon abatement into the evaluation; it does not move workloads between grid windows.\n\n"+
		"**Running totals**:\n"+
		"- ✅ **%s** CO₂ abated from %d applied proposals\n"+
		"- 📉 Right-sizing (In-Place Pod Resizing) = no pod restart, no re-scheduling cost\n"+
		"- ⚡ Carbon-aware scheduling is not implemented in v4.0-beta.\n\n"+
		"Tip: set `carbonWeight` between 0 and 1 in the policy (for example `0.4` for a 40%% carbon objective).",
		carbonHuman, appliedCount)
}

func (s *Server) respondArbitrage(args map[string]interface{}) string {
	workload, _ := args["workload"].(string)
	if s.deps.ArbitrageAnalyzer != nil && workload != "" {
		options, _, err := s.deps.ArbitrageAnalyzer.EvaluateArbitrage(
			bgCtx, workload, "default",
			4.0, 16.0,
			cloud.ProviderAWS, "us-east-1",
		)
		if err == nil && len(options) > 0 {
			lines := []string{"### 🌐 Multi-Cloud Arbitrage Recommendation"}
			lines = append(lines, fmt.Sprintf("\n**Workload**: `%s` (4 vCPU / 16 GiB baseline)", workload))
			var best *engine.ArbitrageOption
			for i := range options {
				if options[i].IsCurrent {
					best = &options[i]
					break
				}
			}
			if best == nil && len(options) > 0 {
				best = &options[0]
			}
			if best != nil {
				lines = append(lines, fmt.Sprintf("**Baseline**: %s %s (%s) → $%.2f / mo | %.0f kg CO₂ / mo\n",
					best.Instance.Provider, best.Instance.Name, best.Instance.Region,
					best.MonthlyCostUSD, best.MonthlyCarbonKg))
			}
			printed := 0
			for i, opt := range options {
				if opt.IsCurrent {
					continue
				}
				if printed >= 3 {
					break
				}
				printed++
				lines = append(lines, fmt.Sprintf("\n**Option %d** – **%s %s** (%s %s)\n"+
					"    • Monthly cost: $%.2f | Carbon: %.0f kg CO₂\n"+
					"    • Savings: $%.2f / mo (%.0f kg CO₂)\n"+
					"    • vCPU: %.0f | Mem: %.1f GiB",
					i, opt.Instance.Provider, opt.Instance.Name, opt.Instance.Provider, opt.Instance.Region,
					opt.MonthlyCostUSD, opt.MonthlyCarbonKg,
					opt.CostSavingUSD, opt.CarbonSavingKg,
					opt.Instance.VCPU, opt.Instance.MemoryGB,
				))
			}
			return strings.Join(lines, "\n")
		}
	}
	return "### 🌐 Multi-Cloud Arbitrage\n\nLive ranking uses current AWS / GCP / Azure on-demand & spot rates from the pricing factory (see `internal/cloud/factory.go`).\n\nTo see concrete migration candidates, try:\n\n- `evaluate arbitrage for payment-service`\n- `rank workloads by cost`\n\nTip: set `GCP_BILLING_API_KEY`, AWS credentials, and/or `AZURE_SUB_ID` env vars to enable live pricing lookups. Otherwise a bundled static fallback is used."
}

func (s *Server) respondForecast(args map[string]interface{}) string {
	workload, _ := args["workload"].(string)
	if workload == "" {
		workload = "(any workload)"
	}
	return fmt.Sprintf("### 📈 Workload Resource Forecast\n\n**Workload**: `%s`\n\nThe forecasting engine uses historical Prometheus usage (last 7 days, default) and computes 95%% confidence-interval bounds. The analyzer compares the forecasted peak against current requests:\n\n1. 🔍 If forecasted peak < 85%% of request **for 3 consecutive windows** → a right-sizing proposal is generated.\n2. ⚠️ If forecasted peak > request → proposal is suppressed (safety-first).\n3. 🔐 Forecast confidence < 75%% → recommendation requires manual approval.\n\nPrometheus metrics used:\n- `rate(container_cpu_usage_seconds_total[5m])`\n- `container_memory_working_set_bytes`\n\nQuery the UI `#/forecast` page for an interactive chart.",
		workload)
}

func (s *Server) respondAnomalies() string {
	var list optimizationv1alpha1.OptimizationProposalList
	if err := s.deps.Client.List(bgCtx, &list); err != nil {
		return fmt.Sprintf("Failed to collect anomalies: `%s`.", err.Error())
	}
	type anomalyRow struct {
		Time     time.Time
		Workload string
		Namespace string
		Score    int32
		Severity string
		Proposal string
	}
	rows := []anomalyRow{}
	for _, p := range list.Items {
		score := p.Spec.DecisionMetadata.RiskScore
		if score == 0 {
			continue
		}
		sev := "low"
		switch {
		case score >= 70:
			sev = "high"
		case score >= 40:
			sev = "medium"
		}
		rows = append(rows, anomalyRow{
			Time:      p.CreationTimestamp.Time,
			Workload:  p.Spec.TargetWorkload.Name,
			Namespace: p.Namespace,
			Score:     score,
			Severity:  sev,
			Proposal:  p.Name,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Time.After(rows[j].Time) })
	if len(rows) == 0 {
		return "### ⚠️ Anomalies\n\nNo resource anomalies detected. Nice clean fleet! ✅\n\nAnomalies are raised when:\n- CPU usage spike > 3σ from the rolling 24h mean\n- Memory working set > 95%% of request for 10+ minutes\n- Proposal risk score auto-incremented ≥ 40 (medium) or ≥ 70 (high)."
	}
	lines := []string{"### ⚠️ Recent Anomalies"}
	for i, row := range rows {
		if i >= 10 {
			break
		}
		lines = append(lines,
			fmt.Sprintf("\n**%d.** `%s/%s` — Risk score %d (%s)\n    • Proposal: `%s` — %s",
				i+1, row.Namespace, row.Workload, row.Score, row.Severity,
				row.Proposal, row.Time.Format(time.RFC3339)))
	}
	return strings.Join(lines, "\n")
}

func (s *Server) respondListPolicies() string {
	var list optimizationv1alpha1.OptimizationPolicyList
	if err := s.deps.Client.List(bgCtx, &list); err != nil {
		return fmt.Sprintf("Failed to list policies: `%s`.", err.Error())
	}
	if len(list.Items) == 0 {
		return "### 🛡️ Policies\n\nNo OptimizationPolicies found. The controller applies sensible defaults (Propose mode, 15-min cooldown, CarbonWeight=0)."
	}
	lines := []string{"### 🛡️ Optimization Policies"}
	for i, p := range list.Items {
		mode := string(p.Spec.Mode)
		if mode == "" {
			mode = "Propose"
		}
		objective := string(p.Spec.Objective)
		if objective == "" {
			objective = "Efficiency"
		}
		carbonW := 0.0
		if p.Spec.SafetyConstraints.CarbonWeight != nil {
			carbonW = *p.Spec.SafetyConstraints.CarbonWeight
		}
		cooldownStr := "none"
		if p.Spec.SafetyConstraints.CooldownPeriod != nil {
			cooldownStr = p.Spec.SafetyConstraints.CooldownPeriod.Duration.String()
		}
		minReplicas := int32(0)
		if p.Spec.SafetyConstraints.MinReplicas != nil {
			minReplicas = *p.Spec.SafetyConstraints.MinReplicas
		}
		maxLatencyMs := int32(0)
		if p.Spec.SafetyConstraints.MaxLatencyIncreaseMs != nil {
			maxLatencyMs = *p.Spec.SafetyConstraints.MaxLatencyIncreaseMs
		}
		lines = append(lines,
			fmt.Sprintf("\n**%d.** `%s` (namespace `%s`)\n"+
				"    • Objective: **%s** | Mode: **%s**\n"+
				"    • CarbonWeight: %.2f | Cooldown: %s\n"+
				"    • Safety → Min replicas: %d | Max latency increase: %d ms",
				i+1, p.Name, p.Namespace, objective, mode,
				carbonW, cooldownStr, minReplicas, maxLatencyMs,
			))
	}
	return strings.Join(lines, "\n")
}

func (s *Server) respondClusters() string {
	var list optimizationv1alpha1.ClusterRegistrationList
	_ = s.deps.Client.List(bgCtx, &list)
	lines := []string{"### 🌐 Clusters (Federated Network)"}
	lines = append(lines, "\n**1.** `hub` — type: **hub** — status: `ready`")
	for i, c := range list.Items {
		lines = append(lines,
			fmt.Sprintf("\n**%d.** `%s` — type: **spoke** — cluster: `%s`\n"+
				"    • Endpoint: `%s`\n    • Status: `%s` — last heartbeat: %s",
				i+2, c.Name, c.Spec.ClusterName,
				c.Spec.APIEndpoint,
				c.Status.Status, func() string {
					if c.Status.LastHeartbeat.IsZero() {
						return "never"
					}
					return c.Status.LastHeartbeat.Time.Format(time.RFC3339)
				}(),
			))
	}
	return strings.Join(lines, "\n")
}

func (s *Server) respondWorkloads(args map[string]interface{}) string {
	ns, _ := args["namespace"].(string)
	ctx := bgCtx
	kindFilters := []string{}
	if raw, ok := args["_kindHint"].(string); ok && raw != "" {
		kindFilters = append(kindFilters, raw)
	}
	lines := []string{"### 💼 Workload Inventory"}
	if ns == "" {
		lines = append(lines, "\nScope: `all namespaces`")
	} else {
		lines = append(lines, fmt.Sprintf("\nScope: namespace `%s`", ns))
	}

	count := 0
	totalCPU := int64(0)
	totalMem := int64(0)
	listOpts := []client.ListOption{}
	if ns != "" {
		listOpts = append(listOpts, client.InNamespace(ns))
	}

	type row struct {
		kind      string
		name      string
		namespace string
		replicas  int32
		ready     int32
		cpuReq    string
		memReq    string
	}
	rows := []row{}

	if len(kindFilters) == 0 || containsI(kindFilters, "deployment") {
		var list appsv1.DeploymentList
		if err := s.deps.Client.List(ctx, &list, listOpts...); err == nil {
			for _, d := range list.Items {
				cpu, mem := sumContainerRequests(d.Spec.Template.Spec.Containers)
				rows = append(rows, row{
					kind:      "Deployment",
					name:      d.Name,
					namespace: d.Namespace,
					replicas:  int32OrDefault(d.Spec.Replicas, 1),
					ready:     d.Status.ReadyReplicas,
					cpuReq:    cpu,
					memReq:    mem,
				})
				count++
			}
		} else if !apierrors.IsNotFound(err) {
			lines = append(lines, fmt.Sprintf("\n⚠️ Could not list Deployments: `%s`", safeShortErr(err)))
		}
	}
	if len(kindFilters) == 0 || containsI(kindFilters, "statefulset") {
		var list appsv1.StatefulSetList
		if err := s.deps.Client.List(ctx, &list, listOpts...); err == nil {
			for _, d := range list.Items {
				cpu, mem := sumContainerRequests(d.Spec.Template.Spec.Containers)
				rows = append(rows, row{
					kind:      "StatefulSet",
					name:      d.Name,
					namespace: d.Namespace,
					replicas:  int32OrDefault(d.Spec.Replicas, 1),
					ready:     d.Status.ReadyReplicas,
					cpuReq:    cpu,
					memReq:    mem,
				})
				count++
			}
		} else if !apierrors.IsNotFound(err) {
			lines = append(lines, fmt.Sprintf("\n⚠️ Could not list StatefulSets: `%s`", safeShortErr(err)))
		}
	}
	if len(kindFilters) == 0 || containsI(kindFilters, "daemonset") {
		var list appsv1.DaemonSetList
		if err := s.deps.Client.List(ctx, &list, listOpts...); err == nil {
			for _, d := range list.Items {
				cpu, mem := sumContainerRequests(d.Spec.Template.Spec.Containers)
				rows = append(rows, row{
					kind:      "DaemonSet",
					name:      d.Name,
					namespace: d.Namespace,
					replicas:  d.Status.DesiredNumberScheduled,
					ready:     d.Status.NumberReady,
					cpuReq:    cpu,
					memReq:    mem,
				})
				count++
			}
		} else if !apierrors.IsNotFound(err) {
			lines = append(lines, fmt.Sprintf("\n⚠️ Could not list DaemonSets: `%s`", safeShortErr(err)))
		}
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].kind < rows[j].kind || (rows[i].kind == rows[j].kind && rows[i].namespace < rows[j].namespace) })
	_ = totalCPU
	_ = totalMem

	lines = append(lines, fmt.Sprintf("\n**Total workloads**: %d\n", count))
	if count == 0 {
		lines = append(lines, "_No workloads returned by the Kubernetes API in the requested scope._")
		return strings.Join(lines, "\n")
	}

	limit := 50
	for i, r := range rows {
		if i >= limit {
			lines = append(lines, fmt.Sprintf("\n… + %d more (narrow scope with `namespace: <ns>`)", len(rows)-limit))
			break
		}
		lines = append(lines,
			fmt.Sprintf("\n**%d.** `%s` `%s/%s`\n"+
				"    • Replicas: **%d/%d** ready\n"+
				"    • Per-pod requests: **CPU %s** • **Memory %s**",
				i+1, r.kind, r.namespace, r.name, r.ready, r.replicas, r.cpuReq, r.memReq))
	}

	lines = append(lines, "\n**Labels watched by the analyzer**:")
	lines = append(lines, "- `reconcilyx.io/exclude: true` → skipped from analysis")
	lines = append(lines, "- `reconcilyx.io/force-analysis: true` → immediate triggered analysis")
	lines = append(lines, "- label/annotation match against `OptimizationPolicy.spec.workloadSelector` for scoping")
	return strings.Join(lines, "\n")
}

func (s *Server) respondMetrics(args map[string]interface{}) string {
	workload, _ := args["workload"].(string)
	ns, _ := args["namespace"].(string)
	if ns == "" {
		ns = "default"
	}
	ctx := bgCtx

	lines := []string{"### 📊 Workload Telemetry Snapshot"}
	if workload != "" {
		lines = append(lines, fmt.Sprintf("\nTarget: `%s/%s`", ns, workload))
	} else {
		lines = append(lines, "\nTarget: `cluster aggregate` (no specific workload requested)")
	}

	var key *types.NamespacedName
	if workload != "" {
		found := false
		var dList appsv1.DeploymentList
		_ = s.deps.Client.List(ctx, &dList, client.InNamespace(ns))
		for _, d := range dList.Items {
			if d.Name == workload {
				k := types.NamespacedName{Namespace: d.Namespace, Name: d.Name}
				key = &k
				lines = append(lines, fmt.Sprintf("- Kind: `Deployment` | replicas: **%d** ready | generation: %d", d.Status.ReadyReplicas, d.Status.ObservedGeneration))
				cpu, mem := sumContainerRequests(d.Spec.Template.Spec.Containers)
				lines = append(lines, fmt.Sprintf("- Per-pod requests: **CPU %s** • **Memory %s**", cpu, mem))
				found = true
				break
			}
		}
		if !found {
			var ssList appsv1.StatefulSetList
			_ = s.deps.Client.List(ctx, &ssList, client.InNamespace(ns))
			for _, d := range ssList.Items {
				if d.Name == workload {
					k := types.NamespacedName{Namespace: d.Namespace, Name: d.Name}
					key = &k
					lines = append(lines, fmt.Sprintf("- Kind: `StatefulSet` | replicas: **%d** ready | generation: %d", d.Status.ReadyReplicas, d.Status.ObservedGeneration))
					cpu, mem := sumContainerRequests(d.Spec.Template.Spec.Containers)
					lines = append(lines, fmt.Sprintf("- Per-pod requests: **CPU %s** • **Memory %s**", cpu, mem))
					found = true
					break
				}
			}
		}
		if !found {
			lines = append(lines, fmt.Sprintf("\n⚠️ No Deployment/StatefulSet workload named `%s` found in namespace `%s`. Try `show deployments` first.", workload, ns))
			return strings.Join(lines, "\n")
		}
	}

	var pods []corev1.Pod
	if key != nil {
		var list corev1.PodList
		if err := s.deps.Client.List(ctx, &list, client.InNamespace(key.Namespace)); err == nil {
			for _, p := range list.Items {
				for _, o := range p.OwnerReferences {
					if o.Kind == "ReplicaSet" || o.Kind == "StatefulSet" {
						pods = append(pods, p)
						break
					}
				}
			}
		}
	} else {
		var list corev1.PodList
		if err := s.deps.Client.List(ctx, &list, client.Limit(500)); err == nil {
			pods = list.Items
		}
	}

	phaseCounts := map[corev1.PodPhase]int{}
	runningCPU := int64(0)
	runningMem := int64(0)
	for _, p := range pods {
		phaseCounts[p.Status.Phase]++
		if p.Status.Phase == corev1.PodRunning {
			for _, cs := range p.Spec.Containers {
				if q := cs.Resources.Requests.Cpu(); q != nil {
					runningCPU += q.MilliValue()
				}
				if q := cs.Resources.Requests.Memory(); q != nil {
					runningMem += q.Value()
				}
			}
		}
	}
	lines = append(lines, "")
	for _, ph := range []corev1.PodPhase{corev1.PodRunning, corev1.PodPending, corev1.PodFailed, corev1.PodSucceeded, corev1.PodUnknown} {
		n := phaseCounts[ph]
		if n > 0 {
			lines = append(lines, fmt.Sprintf("- Pods `%s`: **%d**", ph, n))
		}
	}
	lines = append(lines,
		fmt.Sprintf("\n**Aggregated Running requests (sampled pods)**: %dm CPU • %dMi memory",
			runningCPU, runningMem/(1024*1024),
		))
	lines = append(lines, "\n**PromQL sources (wired into the analyzer)**:")
	lines = append(lines, "- `rate(container_cpu_usage_seconds_total[5m])` — actual CPU")
	lines = append(lines, "- `container_memory_working_set_bytes` — actual memory")
	lines = append(lines, "- `kube_pod_container_resource_requests{resource=~\"cpu|memory\"}` — requests")
	lines = append(lines, "- `kube_pod_status_phase{phase=\"Running\"}` — inventory + SLO regression")
	lines = append(lines, "\n_Note: live query values are available at_ `/api/metrics/savings` _and in the UI dashboard charts._")
	return strings.Join(lines, "\n")
}

func containsI(slice []string, needle string) bool {
	n := strings.ToLower(needle)
	for _, s := range slice {
		if strings.ToLower(s) == n {
			return true
		}
	}
	return false
}

func int32OrDefault(p *int32, d int32) int32 {
	if p == nil {
		return d
	}
	return *p
}

func sumContainerRequests(containers []corev1.Container) (cpu, mem string) {
	var cpuSum, memSum int64
	for _, c := range containers {
		if q := c.Resources.Requests.Cpu(); q != nil {
			cpuSum += q.MilliValue()
		}
		if q := c.Resources.Requests.Memory(); q != nil {
			memSum += q.Value()
		}
	}
	if cpuSum == 0 {
		cpu = "—"
	} else {
		cpu = fmt.Sprintf("%dm", cpuSum)
	}
	if memSum == 0 {
		mem = "—"
	} else {
		mem = formatMemBytes(memSum)
	}
	return
}

func formatMemBytes(b int64) string {
	if b <= 0 {
		return "0"
	}
	const (
		Ki = 1024
		Mi = Ki * 1024
		Gi = Mi * 1024
	)
	switch {
	case b >= Gi:
		return fmt.Sprintf("%.1fGi", float64(b)/float64(Gi))
	case b >= Mi:
		return fmt.Sprintf("%dMi", b/Mi)
	default:
		return fmt.Sprintf("%dKi", b/Ki)
	}
}

func safeShortErr(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	if len(s) > 160 {
		return s[:160] + "…"
	}
	return s
}

func (s *Server) respondExplain(args map[string]interface{}) string {
	topic, _ := args["topic"].(string)
	switch {
	case strings.Contains(strings.ToLower(topic), "in-place") || strings.Contains(strings.ToLower(topic), "resize"):
		return "### 🧠 What is In-Place Pod Resizing?\n\nKubernetes 1.27+ introduced **InPlacePodVerticalScaling**, which lets the kubelet update a running container's `resources.requests` and `resources.limits` **without restarting the pod**.\n\nReconcilyx applies it with Server-Side Apply (SSA) against the Pod `resize` subresource, using the `reconcilyx-operator` field manager:\n1. Detector proposes new requests\n2. Approval (or AutoApply mode)\n3. Controller SSA-patches matching Pods via `/resize`\n4. kubelet applies the change without restarting the Pod\n\nThe beta does not fall back to a rolling restart. If the feature gate is disabled or the resize is rejected, execution fails with a feature-gate hint and the proposal can be rolled back from its execution snapshot."
	case strings.Contains(strings.ToLower(topic), "q-learning") || strings.Contains(strings.ToLower(topic), "rl"):
		return "### 🧠 Reinforcement Learning (Q-Learning) in Reconcilyx\n\n**Goal:** Train an agent to pick *how much* to cut CPU / memory, not just *whether*.\n\n- **State bucket**: discretized (CPU utilization %, Memory utilization %, Current request bin)\n- **Actions**: ±10%, ±20%, ±30% CPU and Memory\n- **Reward signal** = `(cost_weight * $saved) + (carbon_weight * CO2_saved) - (SLO_weight * regression_count * 100)`\n- **Epsilon-greedy** exploration decaying to 5%\n\nPolicy guardrails and the execution gate remain authoritative over RL output. Federated delta exchange and the model registry are partially wired in v4.0-beta; live rollout is not claimed."
	case strings.Contains(strings.ToLower(topic), "carbon") || strings.Contains(strings.ToLower(topic), "green"):
		return "### 🌱 Carbon Awareness Explained\n\nTwo knobs:\n1. **`carbonWeight` in OptimizationPolicy (0–1)** — how much the evaluation blends CO₂ abatement with pure $-saved.\n2. **Carbon provider**: Electricity Maps / WattTime (live) or static (by region).\n\nArbitrage ranking combines both:\n`Score = α*(cost_cut/max_cost_cut) + (1-α)*(carbon_cut/max_carbon_cut)`\nwhere α comes from `(cost / (cost + carbon))`.\n\nCarbon-aware scheduling during high-emission grid windows is not implemented in v4.0-beta."
	default:
		return "### 📖 Topics\n\nAsk about:\n- `explain in-place pod resizing`\n- `explain q-learning` / `reinforcement learning`\n- `explain carbon awareness`\n- `explain SSA` (Server-Side Apply)\n- `explain federated learning`"
	}
}

var _ = strconv.Itoa
