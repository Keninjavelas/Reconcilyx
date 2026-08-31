package llm

import (
	"context"
	"regexp"
	"strings"
)

type RouteResult struct {
	FunctionName string                 `json:"functionName"`
	Arguments    map[string]interface{} `json:"arguments"`
	IsFastPath   bool                   `json:"isFastPath"`
}

type patternEntry struct {
	fnName string
	pat    *regexp.Regexp
}

type HybridRouter struct {
	provider Provider
	patterns []patternEntry
}

func NewHybridRouter(provider Provider) *HybridRouter {
	if provider == nil {
		p, _ := NewProvider("ollama", nil)
		provider = p
	}

	return &HybridRouter{
		provider: provider,
		patterns: []patternEntry{
			{fnName: "getProposal", pat: regexp.MustCompile(`(?i)(proposal\s+details|show\s+proposal|inspect\s+proposal|what.*about\s+proposal|describe\s+proposal)`)},
			{fnName: "approveProposal", pat: regexp.MustCompile(`(?i)(approve|execute|apply|accept|go\s+ahead)\s*(proposal|recommendation|change)?`)},
			{fnName: "rejectProposal", pat: regexp.MustCompile(`(?i)(reject|deny|veto|decline|disapprove|block|stop)\s*(proposal|recommendation|change)?`)},
			{fnName: "rollbackProposal", pat: regexp.MustCompile(`(?i)(rollback|roll\s*back|revert|undo|restore|back\s*out)\s*(proposal|pod|change)?`)},
			{fnName: "getCarbon", pat: regexp.MustCompile(`(?i)(carbon|co2|emissions?|green|footprint)`)},
			{fnName: "getSavings", pat: regexp.MustCompile(`(?i)(cost|bill|money|savings?|saved|spend)`)},
			{fnName: "evaluateArbitrage", pat: regexp.MustCompile(`(?i)(migrate|migration|cloud|aws|gcp|azure)`)},
			{fnName: "forecastWorkload", pat: regexp.MustCompile(`(?i)(forecast|predict|future|confidence|trend)`)},
			{fnName: "getAnomalies", pat: regexp.MustCompile(`(?i)(anomaly|anomalies|spike|regression|unusual)`)},
			{fnName: "getPolicies", pat: regexp.MustCompile(`(?i)(polic(y|ies)|weights?|reward)`)},
			{fnName: "getClusters", pat: regexp.MustCompile(`(?i)(clusters?|nodes?|network|federat(ed|ion)|spoke|hub)`)},
			{fnName: "getMetrics", pat: regexp.MustCompile(`(?i)(metrics|telemetry|prometheus|usage|cpu\s*usage|memory\s*usage|resource\s*usage)`)},
			{fnName: "getProposals", pat: regexp.MustCompile(`(?i)(proposals?|recommendations?|pending|actions?)`)},
			{fnName: "getWorkloads", pat: regexp.MustCompile(`(?i)(workloads?|deployments?|statefulsets?|daemonsets?|pods?|apps)`)},
			{fnName: "explain", pat: regexp.MustCompile(`(?i)(explain|what\s*is|define|describe|teach|how\s*(does|do|can))`)},
		},
	}
}

func (r *HybridRouter) Route(ctx context.Context, userQuery string) (*RouteResult, error) {
	queryLower := strings.TrimSpace(userQuery)
	args := extractNamedArguments(queryLower)

	for _, entry := range r.patterns {
		fnName := entry.fnName
		pat := entry.pat
		if !pat.MatchString(queryLower) {
			continue
		}
		merged := make(map[string]interface{}, len(args))
		for k, v := range args {
			merged[k] = v
		}
		switch fnName {
		case "forecastWorkload", "evaluateArbitrage", "getMetrics":
			if _, ok := merged["workload"]; !ok {
				merged["workload"] = "payment-service"
			}
		case "explain":
			if _, ok := merged["topic"]; !ok {
				merged["topic"] = firstNoun(queryLower)
			}
		}
		return &RouteResult{
			FunctionName: fnName,
			Arguments:    merged,
			IsFastPath:   true,
		}, nil
	}

	messages := []Message{
		{
			Role:    RoleSystem,
			Content: "You are Reconcilyx FinOps AI Assistant. Match user queries to control plane tool calls.",
		},
		{
			Role:    RoleUser,
			Content: userQuery,
		},
	}

	tools := GetReconcilyxTools()
	resp, err := r.provider.Chat(ctx, messages, tools)
	if err != nil || resp == nil || len(resp.ToolCalls) == 0 {
		return &RouteResult{
			FunctionName: "getSavings",
			Arguments:    map[string]interface{}{},
			IsFastPath:   true,
		}, nil
	}

	call := resp.ToolCalls[0]
	return &RouteResult{
		FunctionName: call.Function.Name,
		Arguments:    call.Function.Arguments,
		IsFastPath:   false,
	}, nil
}

var (
	quoteTokenRE  = regexp.MustCompile(`["'\x60]([^"'\x60]+)["'\x60]`)
	nameCaptureRE = regexp.MustCompile(`(?i)\b(?:named?|called|for|on|ns:|namespace:?)\s+["'\x60]?([A-Za-z0-9][A-Za-z0-9\-\._]+)["'\x60]?`)
)

func extractNamedArguments(q string) map[string]interface{} {
	out := map[string]interface{}{}
	for _, m := range nameCaptureRE.FindAllStringSubmatch(q, -1) {
		if _, ok := out["name"]; !ok {
			out["name"] = m[1]
		} else if _, ok := out["namespace"]; !ok {
			out["namespace"] = m[1]
		}
	}
	tokens := quoteTokenRE.FindAllStringSubmatch(q, -1)
	for i, m := range tokens {
		if i == 0 && len(m) > 1 {
			if _, ok := out["name"]; !ok {
				out["name"] = m[1]
			} else if _, ok := out["namespace"]; !ok {
				out["namespace"] = m[1]
			}
		}
	}
	lower := strings.ToLower(q)
	for _, hint := range []string{"workload", "deployment", "service", "app"} {
		if re := regexp.MustCompile(`(?i)` + hint + `\s+["'\x60]?([A-Za-z0-9][A-Za-z0-9\-\._/]+)`); re.MatchString(lower) {
			if sm := re.FindStringSubmatch(lower); len(sm) > 1 {
				out["workload"] = sm[1]
				if _, ok := out["name"]; !ok {
					out["name"] = sm[1]
				}
				break
			}
		}
	}
	return out
}

func firstNoun(q string) string {
	fields := strings.Fields(strings.TrimSpace(q))
	for _, w := range fields {
		w = strings.Trim(w, ".,?!;:")
		if len(w) >= 4 && !strings.ContainsAny(w, "!?@#$%^&*") {
			return w
		}
	}
	if len(fields) > 0 {
		return fields[0]
	}
	return "topic"
}
