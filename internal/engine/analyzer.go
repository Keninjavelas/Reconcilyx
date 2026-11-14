package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/reconcilyx/operator/api/v1alpha1"
	"github.com/reconcilyx/operator/internal/carbon"
	"github.com/reconcilyx/operator/internal/engine/anomaly"
	"github.com/reconcilyx/operator/internal/engine/forecasting"
	"github.com/reconcilyx/operator/internal/engine/rl"
	"github.com/reconcilyx/operator/internal/telemetry"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	kWhPerMilliCPUMonth = 0.288
	kWhPerGBMemMonth    = 0.36
	bytesPerGB          = 1024 * 1024 * 1024
	gramsCO2PerKGFactor = 1.0
	defaultRegion       = ""
	defaultCarbonWeight = 0.0
	usdPerMilliCPUMonth = 0.0365
	usdPerGBMemMonth    = 7.30
)

func ComputeCostSavedUSDPerMonth(cpuDeltaMilli, memDeltaBytes int64, replicas int32) float64 {
	if replicas <= 0 {
		replicas = 1
	}
	usd := (float64(cpuDeltaMilli) * usdPerMilliCPUMonth) + (float64(memDeltaBytes)/float64(bytesPerGB))*usdPerGBMemMonth
	return usd * float64(replicas)
}

func ComputeCarbonSavedGramsPerMonth(cpuDeltaMilli, memDeltaBytes int64, replicas int32, intensityGCO2PerKWh float64) float64 {
	if intensityGCO2PerKWh <= 0 || replicas <= 0 {
		if replicas <= 0 {
			replicas = 1
		}
		if intensityGCO2PerKWh <= 0 {
			return 0
		}
	}
	cpuKWh := float64(cpuDeltaMilli) * kWhPerMilliCPUMonth
	memKWh := (float64(memDeltaBytes) / float64(bytesPerGB)) * kWhPerGBMemMonth
	totalKWh := (cpuKWh + memKWh) * float64(replicas)
	if totalKWh <= 0 {
		return 0
	}
	return totalKWh * intensityGCO2PerKWh * gramsCO2PerKGFactor
}

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
	rlAgent           rl.Agent
	anomalyDetector   anomaly.Detector
	carbonProvider    carbon.Provider
	workloadRegion    string
	carbonWeight      float64
	omitCPULimits     bool
}

func defaultCarbonProvider() carbon.Provider {
	return carbon.NewStaticProvider()
}

func NewAnalyzer(provider telemetry.TelemetryProvider, registry RuleRegistry, scorer Scorer, predictor forecasting.Predictor, opts ...AnalyzerOption) Analyzer {
	a := &defaultAnalyzer{
		telemetryProvider: provider,
		registry:          registry,
		scorer:            scorer,
		predictor:         predictor,
		rlAgent:           rl.NewSimpleQAgent(),
		anomalyDetector:   anomaly.NewZScoreDetector(3.0),
		carbonProvider:    defaultCarbonProvider(),
		workloadRegion:    defaultRegion,
		carbonWeight:      defaultCarbonWeight,
		omitCPULimits:     true,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

func NewAnalyzerWithRL(provider telemetry.TelemetryProvider, registry RuleRegistry, scorer Scorer, predictor forecasting.Predictor, agent rl.Agent, opts ...AnalyzerOption) Analyzer {
	allOpts := append([]AnalyzerOption{WithRLAgent(agent)}, opts...)
	return NewAnalyzer(provider, registry, scorer, predictor, allOpts...)
}

func collectHistoricalSeries(containerMetrics *telemetry.ContainerMetrics) ([]float64, []float64) {
	cpuHist := make([]float64, 0, len(containerMetrics.CPU.HistoricalCores))
	for _, s := range containerMetrics.CPU.HistoricalCores {
		cpuHist = append(cpuHist, s)
	}
	memHist := make([]float64, 0, len(containerMetrics.Memory.HistoricalWorkingSetBytes))
	for _, s := range containerMetrics.Memory.HistoricalWorkingSetBytes {
		memHist = append(memHist, float64(s))
	}
	return cpuHist, memHist
}

func (a *defaultAnalyzer) Analyze(ctx context.Context, ref corev1.ObjectReference, currentResources map[string]corev1.ResourceRequirements) (*v1alpha1.OptimizationProposal, error) {
	startTime := time.Now()
	defer func() {
		telemetry.OptimizationLatency.Observe(time.Since(startTime).Seconds())
	}()
	workload := ref.Name
	ns := ref.Namespace

	prediction, _ := a.predictor.Forecast(ctx, ref)
	if prediction != nil {
		telemetry.ForecastConfidence.WithLabelValues(workload, ns).Set(float64(prediction.Confidence))
		telemetry.ForecastCPU.WithLabelValues(workload, ns).Set(float64(prediction.MaxCPU.MilliValue()) / 1000.0)
		telemetry.ForecastMemory.WithLabelValues(workload, ns).Set(float64(prediction.MaxMemory.Value()))
	}

	metrics, err := a.telemetryProvider.GetWorkloadMetrics(ctx, ref)
	if err != nil && err != telemetry.ErrPartialTelemetry {
		return nil, fmt.Errorf("failed to get telemetry: %w", err)
	}

	isPartial := err == telemetry.ErrPartialTelemetry

	confidence, risk := a.scorer.Score(metrics, isPartial)

	var allFindings []*Finding
	var policyNotes []string
	proposedReqs := make(map[string]corev1.ResourceList)
	proposedLimits := make(map[string]corev1.ResourceList)

	for containerName, containerMetrics := range metrics.Containers {
		currentRes, ok := currentResources[containerName]
		if !ok {
			continue
		}

		containerReqs := make(corev1.ResourceList)
		if currentRes.Requests != nil {
			for k, v := range currentRes.Requests {
				containerReqs[k] = v.DeepCopy()
			}
		}
		containerLimits := make(corev1.ResourceList)
		if currentRes.Limits != nil {
			for k, v := range currentRes.Limits {
				containerLimits[k] = v.DeepCopy()
			}
		}

		unstableContainer := containerMetrics.Memory.OOMCount > 0 || containerMetrics.Restarts >= 3

		cpuHist, memHist := collectHistoricalSeries(containerMetrics)
		cpuAnomaly, _ := a.anomalyDetector.Detect(cpuHist, containerMetrics.CPU.UsageCores)
		memAnomaly, _ := a.anomalyDetector.Detect(memHist, float64(containerMetrics.Memory.WorkingSetBytes))

		if cpuAnomaly != nil {
			telemetry.AnomalySeverity.WithLabelValues(workload, ns, "cpu").Set(cpuAnomaly.Severity)
			if cpuAnomaly.Type != anomaly.AnomalyNone {
				sevLabel := "low"
				if cpuAnomaly.Severity > 0.5 {
					sevLabel = "high"
				} else if cpuAnomaly.Severity > 0.2 {
					sevLabel = "medium"
				}
				telemetry.AnomalyEventsTotal.WithLabelValues(workload, ns, "cpu", sevLabel).Inc()
			}
		}
		if memAnomaly != nil {
			telemetry.AnomalySeverity.WithLabelValues(workload, ns, "memory").Set(memAnomaly.Severity)
			if memAnomaly.Type != anomaly.AnomalyNone {
				sevLabel := "low"
				if memAnomaly.Severity > 0.5 {
					sevLabel = "high"
				} else if memAnomaly.Severity > 0.2 {
					sevLabel = "medium"
				}
				telemetry.AnomalyEventsTotal.WithLabelValues(workload, ns, "memory", sevLabel).Inc()
			}
		}

		hasHighAnomaly := (cpuAnomaly != nil && cpuAnomaly.Severity > 0.7) || (memAnomaly != nil && memAnomaly.Severity > 0.7)

		jvmHeapMB := int64(containerMetrics.JVM.XmxBytes / (1024 * 1024))
		hasGPU := containerMetrics.GPU.GPUCount > 0

		state := &rl.WorkloadState{
			CPUUsageCores:         containerMetrics.CPU.UsageCores,
			CPUThrottledSecs:      containerMetrics.CPU.ThrottledSecs,
			MemoryWorkingSetBytes: float64(containerMetrics.Memory.WorkingSetBytes),
			OOMCount:              containerMetrics.Memory.OOMCount,
			CurrentCPURequest:     currentRes.Requests.Cpu().DeepCopy(),
			CurrentMemoryRequest:  currentRes.Requests.Memory().DeepCopy(),
			Region:                a.workloadRegion,
			CarbonWeight:          &a.carbonWeight,
			JVMBucket:             rl.DetermineJVMBucket(jvmHeapMB, containerMetrics.JVM.IsJVMWorkload),
			GPUBucket:             rl.DetermineGPUBucket(containerMetrics.GPU.GPUUtilizationPct, hasGPU),
		}
		rlResult, rlErr := a.rlAgent.Recommend(ctx, state)
		actionType := "none"
		if rlErr == nil && rlResult != nil && rlResult.Action != nil {
			cpuAdj := rlResult.Action.Adjustment[string(corev1.ResourceCPU)].Percentage
			memAdj := rlResult.Action.Adjustment[string(corev1.ResourceMemory)].Percentage
			switch {
			case cpuAdj < 0 || memAdj < 0:
				actionType = "downscale"
			case cpuAdj > 0 || memAdj > 0:
				actionType = "upscale"
			default:
				actionType = "hold"
			}
		}
		telemetry.RLRecommendationsTotal.WithLabelValues(workload, ns, actionType).Inc()

		if !hasHighAnomaly && !unstableContainer && rlErr == nil && rlResult != nil && rlResult.Action != nil {
			for resourceName, adj := range rlResult.Action.Adjustment {
				resName := corev1.ResourceName(resourceName)
				if adj.NewRequest.IsZero() && adj.Percentage != 0 {
					current := currentRes.Requests[resName]
					milli := current.MilliValue()
					newMilli := int64(float64(milli) * (1 + adj.Percentage/100))
					adj.NewRequest = resource.MustParse(fmt.Sprintf("%dm", newMilli))
				}
				if !adj.NewRequest.IsZero() {
					containerReqs[resName] = adj.NewRequest.DeepCopy()
				}
			}
		}

		if !hasHighAnomaly && !unstableContainer && prediction != nil && prediction.Confidence > 80 {
			if !prediction.MaxCPU.IsZero() {
				containerReqs[corev1.ResourceCPU] = prediction.MaxCPU.DeepCopy()
			}
			if !prediction.MaxMemory.IsZero() {
				containerReqs[corev1.ResourceMemory] = prediction.MaxMemory.DeepCopy()
			}
		}

		for _, rule := range a.registry.All() {
			finding, err := rule.Evaluate(containerName, containerMetrics, currentRes)
			if err != nil {
				continue
			}
			if finding != nil {
				allFindings = append(allFindings, finding)
				if finding.ProposedRequest != nil {
					for k, v := range *finding.ProposedRequest {
						containerReqs[k] = v.DeepCopy()
					}
				}
				if finding.ProposedLimit != nil {
					for k, v := range *finding.ProposedLimit {
						containerLimits[k] = v.DeepCopy()
					}
				}
			}
		}

		if a.omitCPULimits {
			if _, hadCPULimit := containerLimits[corev1.ResourceCPU]; hadCPULimit {
				delete(containerLimits, corev1.ResourceCPU)
				policyNotes = append(policyNotes, fmt.Sprintf("[%s]: Omitted CPU limit per omitCpuLimits policy to prevent CFS throttling", containerName))
			}
		}
		if memReq, ok := containerReqs[corev1.ResourceMemory]; ok {
			if memLim, hasLim := containerLimits[corev1.ResourceMemory]; !hasLim || memLim.Cmp(memReq) < 0 {
				containerLimits[corev1.ResourceMemory] = memReq.DeepCopy()
			}
		}

		changed := false
		for k, v := range containerReqs {
			if current, ok := currentRes.Requests[k]; !ok || !current.Equal(v) {
				changed = true
				break
			}
		}
		for k := range currentRes.Requests {
			if _, ok := containerReqs[k]; !ok {
				changed = true
				break
			}
		}
		if !changed {
			for k, v := range containerLimits {
				if current, ok := currentRes.Limits[k]; !ok || !current.Equal(v) {
					changed = true
					break
				}
			}
			for k := range currentRes.Limits {
				if _, ok := containerLimits[k]; !ok {
					changed = true
					break
				}
			}
		}
		if changed {
			proposedReqs[containerName] = containerReqs
			proposedLimits[containerName] = containerLimits
		}
	}

	if len(allFindings) == 0 && len(proposedReqs) == 0 {
		return nil, nil
	}

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

	monthlySavingsUSD := ComputeCostSavedUSDPerMonth(cpuDeltaMilli, memDeltaBytes, metrics.Replicas)
	if monthlySavingsUSD < 0 {
		monthlySavingsUSD = 0
	}

	telemetry.OptimizationSavingsUSD.WithLabelValues(workload, ns).Set(monthlySavingsUSD)

	effectiveRegion := a.workloadRegion
	if effectiveRegion == "" {
		effectiveRegion = defaultRegion
	}

	var carbonSavedGramsCO2 *resource.Quantity
	if carbonSaved, carbonErr := a.computeCarbonSaved(ctx, cpuDeltaMilli, memDeltaBytes, metrics.Replicas, effectiveRegion); carbonErr == nil && carbonSaved > 0 {
		q := resource.MustParse(fmt.Sprintf("%d", int64(carbonSaved)))
		carbonSavedGramsCO2 = &q
		telemetry.OptimizationCarbonSavedGrams.WithLabelValues(workload, ns).Set(carbonSaved)
	}

	var rationales []string
	var primaryRec = v1alpha1.RecommendationRightSize

	for _, f := range allFindings {
		rationales = append(rationales, fmt.Sprintf("[%s]: %s", f.ContainerName, f.Rationale))
		if f.Recommendation == v1alpha1.RecommendationScaleZero {
			primaryRec = v1alpha1.RecommendationScaleZero
		}
	}

	rationales = append(rationales, policyNotes...)
	if prediction != nil && prediction.Confidence > 80 {
		rationales = append(rationales, fmt.Sprintf("Forecasted resource needs (confidence: %d%%)", prediction.Confidence))
	}

	savingsQuantity := resource.MustParse(fmt.Sprintf("%d", int(monthlySavingsUSD)))

	totalProposedReqs := make(corev1.ResourceList)
	totalProposedLimits := make(corev1.ResourceList)
	proposedByContainer := make(map[string]corev1.ResourceRequirements, len(proposedReqs))
	for name, reqs := range proposedReqs {
		for k, v := range reqs {
			if existing, ok := totalProposedReqs[k]; ok {
				existing.Add(v)
				totalProposedReqs[k] = existing
			} else {
				totalProposedReqs[k] = v.DeepCopy()
			}
		}
		lims := proposedLimits[name]
		for k, v := range lims {
			if existing, ok := totalProposedLimits[k]; ok {
				existing.Add(v)
				totalProposedLimits[k] = existing
			} else {
				totalProposedLimits[k] = v.DeepCopy()
			}
		}
		containerSpec := corev1.ResourceRequirements{Requests: reqs.DeepCopy()}
		if len(lims) > 0 {
			containerSpec.Limits = lims.DeepCopy()
		}
		proposedByContainer[name] = containerSpec
	}

	resourceDelta := make(corev1.ResourceList)
	if cpuDeltaMilli != 0 {
		resourceDelta[corev1.ResourceCPU] = *resource.NewMilliQuantity(cpuDeltaMilli, resource.DecimalSI)
	}
	if memDeltaBytes != 0 {
		resourceDelta[corev1.ResourceMemory] = *resource.NewQuantity(memDeltaBytes, resource.BinarySI)
	}

	expectedImpact := v1alpha1.ExpectedImpact{
		EstimatedSavingsUSD: &savingsQuantity,
		ResourceDelta:       resourceDelta,
	}
	if carbonSavedGramsCO2 != nil {
		expectedImpact.CarbonSavedGramsCO2 = carbonSavedGramsCO2
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
				Limits:   totalProposedLimits,
			},
			ProposedResourcesByContainer: proposedByContainer,
			ExpectedImpact:               expectedImpact,
			DecisionMetadata: v1alpha1.DecisionMetadata{
				ConfidenceScore: confidence,
				RiskScore:       risk,
				Rationale:       strings.Join(rationales, "\n"),
			},
		},
	}

	return proposal, nil
}

func (a *defaultAnalyzer) computeCarbonSaved(ctx context.Context, cpuDeltaMilli, memDeltaBytes int64, replicas int32, region string) (float64, error) {
	if a.carbonProvider == nil {
		return 0, nil
	}
	intensity, err := a.carbonProvider.IntensityForRegion(ctx, region)
	if err != nil {
		return 0, err
	}
	return ComputeCarbonSavedGramsPerMonth(cpuDeltaMilli, memDeltaBytes, replicas, intensity.Value), nil
}
