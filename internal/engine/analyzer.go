package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/reconcilyx/operator/api/v1alpha1"
	"github.com/reconcilyx/operator/internal/engine/forecasting"
	"github.com/reconcilyx/operator/internal/telemetry"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type defaultRuleRegistry struct {
	rules []Rule
}

func NewRuleRegistry() RuleRegistry {
	return &defaultRuleRegistry{}
}

func (r *defaultRuleRegistry) Register(rule Rule) {
	r.rules = append(r.rules, rule)
}

func (r *defaultRuleRegistry) All() []Rule {
	return r.rules
}

type defaultAnalyzer struct {
	telemetryProvider telemetry.TelemetryProvider
	registry          RuleRegistry
	scorer            Scorer
	predictor         forecasting.Predictor
}

// NewAnalyzer creates the core optimization engine.
func NewAnalyzer(provider telemetry.TelemetryProvider, registry RuleRegistry, scorer Scorer, predictor forecasting.Predictor) Analyzer {
	return &defaultAnalyzer{
		telemetryProvider: provider,
		registry:          registry,
		scorer:            scorer,
		predictor:         predictor,
	}
}

func (a *defaultAnalyzer) Analyze(ctx context.Context, ref corev1.ObjectReference, currentResources map[string]corev1.ResourceRequirements) (*v1alpha1.OptimizationProposal, error) {
	startTime := time.Now()
	defer func() {
		telemetry.OptimizationLatency.Observe(time.Since(startTime).Seconds())
	}()

	// 0. Forecasting Check (Hybrid Predictive ML Stub)
	prediction, _ := a.predictor.Forecast(ctx, ref)
	if prediction != nil && prediction.Confidence > 80 {
		// Example implementation of how the ML engine overrides the rules
	}

	// 1. Collect Telemetry
	metrics, err := a.telemetryProvider.GetWorkloadMetrics(ctx, ref)
	if err != nil && err != telemetry.ErrPartialTelemetry {
		return nil, fmt.Errorf("failed to get telemetry: %w", err)
	}

	isPartial := err == telemetry.ErrPartialTelemetry

	// 2. Score
	confidence, risk := a.scorer.Score(metrics, isPartial)

	// 3. Evaluate Rules
	var allFindings []*Finding
	proposedReqs := make(map[string]corev1.ResourceList)

	for containerName, containerMetrics := range metrics.Containers {
		currentRes, ok := currentResources[containerName]
		if !ok {
			continue // No existing resources to optimize
		}

		// Keep original requests to start with
		containerReqs := make(corev1.ResourceList)
		if currentRes.Requests != nil {
			for k, v := range currentRes.Requests {
				containerReqs[k] = v.DeepCopy()
			}
		}

		for _, rule := range a.registry.All() {
			finding, err := rule.Evaluate(containerName, containerMetrics, currentRes)
			if err != nil {
				continue
			}
			if finding != nil {
				allFindings = append(allFindings, finding)
				
				// Overlay finding proposals
				if finding.ProposedRequest != nil {
					for k, v := range *finding.ProposedRequest {
						containerReqs[k] = v.DeepCopy()
					}
				}
			}
		}

		if len(containerReqs) > 0 {
			proposedReqs[containerName] = containerReqs
		}
	}

	if len(allFindings) == 0 {
		return nil, nil // No optimizations needed
	}

	// 4. Calculate Expected Impact
	var cpuDeltaMilli int64
	var memDeltaBytes int64

	for containerName, proposedList := range proposedReqs {
		currentRes := currentResources[containerName]
		if reqCPU, ok := currentRes.Requests[corev1.ResourceCPU]; ok {
			if propCPU, ok := proposedList[corev1.ResourceCPU]; ok {
				cpuDeltaMilli += reqCPU.MilliValue() - propCPU.MilliValue()
			}
		}
		if reqMem, ok := currentRes.Requests[corev1.ResourceMemory]; ok {
			if propMem, ok := proposedList[corev1.ResourceMemory]; ok {
				memDeltaBytes += reqMem.Value() - propMem.Value()
			}
		}
	}

	// Calculate rough monthly savings
	// $0.05 per CPU/hr -> $36.5/mo per CPU = $0.0365 per milliCPU
	// $0.01 per GB/hr -> $7.30/mo per GB
	monthlySavingsUSD := (float64(cpuDeltaMilli) * 0.0365) + (float64(memDeltaBytes) / (1024*1024*1024) * 7.30)
	// Apply replicas multiplier
	monthlySavingsUSD = monthlySavingsUSD * float64(metrics.Replicas)

	// Build Proposal
	var rationales []string
	var primaryRec = v1alpha1.RecommendationRightSize

	for _, f := range allFindings {
		rationales = append(rationales, fmt.Sprintf("[%s]: %s", f.ContainerName, f.Rationale))
		if f.Recommendation == v1alpha1.RecommendationScaleZero {
			primaryRec = v1alpha1.RecommendationScaleZero
		}
	}

	savingsQuantity := resource.MustParse(fmt.Sprintf("%d", int(monthlySavingsUSD)))

	// Aggregate all proposed resources for the Proposal CR (flattened for MVP presentation, though API only holds one set)
	// Note: API `proposedResources` is a single struct, so for MVP we just sum them up or take the primary container.
	// Actually, the API might need to be container-aware too in the future.
	// For now, we sum the map to fit the existing API `ProposedResources`.
	totalProposedReqs := make(corev1.ResourceList)
	for _, reqs := range proposedReqs {
		for k, v := range reqs {
			if existing, ok := totalProposedReqs[k]; ok {
				existing.Add(v)
				totalProposedReqs[k] = existing
			} else {
				totalProposedReqs[k] = v.DeepCopy()
			}
		}
	}

	proposal := &v1alpha1.OptimizationProposal{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-opt-", ref.Name),
			Namespace:    ref.Namespace,
		},
		Spec: v1alpha1.OptimizationProposalSpec{
			TargetWorkload: ref,
			Recommendation: primaryRec,
			ProposedResources: corev1.ResourceRequirements{
				Requests: totalProposedReqs,
			},
			ExpectedImpact: v1alpha1.ExpectedImpact{
				EstimatedSavingsUSD: &savingsQuantity,
			},
			DecisionMetadata: v1alpha1.DecisionMetadata{
				ConfidenceScore: confidence,
				RiskScore:       risk,
				Rationale:       strings.Join(rationales, "\n"),
			},
		},
	}

	return proposal, nil
}
