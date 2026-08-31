package engine

import (
	"context"
	"fmt"
	"sort"
	"time"

	optimizationv1alpha1 "github.com/reconcilyx/operator/api/v1alpha1"
	"github.com/reconcilyx/operator/internal/cloud"
	"github.com/reconcilyx/operator/internal/federated"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ArbitrageOption struct {
	Instance            cloud.InstanceType
	MonthlyCostUSD      float64
	MonthlyCarbonKg     float64
	CostSavingUSD       float64
	CarbonSavingKg      float64
	IsCurrent           bool
}

type ArbitrageAnalyzer struct {
	providers  []cloud.PricingProvider
	aggregator *federated.Aggregator
}

func NewArbitrageAnalyzer(agg *federated.Aggregator) *ArbitrageAnalyzer {
	return NewArbitrageAnalyzerWithProviders(agg, nil)
}

func NewArbitrageAnalyzerWithProviders(agg *federated.Aggregator, providers []cloud.PricingProvider) *ArbitrageAnalyzer {
	if len(providers) == 0 {
		providers = []cloud.PricingProvider{
			cloud.NewCachedPricingProvider(cloud.NewAWSPricingProvider(), 1*time.Hour),
			cloud.NewCachedPricingProvider(cloud.NewGCPPricingProvider(), 1*time.Hour),
			cloud.NewCachedPricingProvider(cloud.NewAzurePricingProvider(), 1*time.Hour),
		}
	}
	return &ArbitrageAnalyzer{
		providers:  providers,
		aggregator: agg,
	}
}

func (a *ArbitrageAnalyzer) EvaluateArbitrage(
	ctx context.Context,
	workloadName, namespace string,
	reqVCPU, reqMemGB float64,
	currentCloud cloud.ProviderName,
	currentRegion string,
) ([]ArbitrageOption, *optimizationv1alpha1.OptimizationProposal, error) {
	hoursPerMonth := 730.0

	var currentCost float64
	var currentCarbon float64
	var allOptions []ArbitrageOption

	// Query each cloud pricing provider across regions
	regions := map[cloud.ProviderName][]string{
		cloud.ProviderAWS:   {"us-east-1", "us-west-2", "eu-west-1"},
		cloud.ProviderGCP:   {"us-central1", "us-west1", "europe-west1"},
		cloud.ProviderAzure: {"eastus", "westus2", "westeurope"},
	}

	for _, p := range a.providers {
		regList := regions[p.Name()]
		for _, reg := range regList {
			matched, err := p.ListMatchingInstances(ctx, reqVCPU, reqMemGB, reg)
			if err != nil {
				continue
			}
			for _, inst := range matched {
				mCost := inst.OnDemandPrice * hoursPerMonth
				// Carbon = kWh (estimated kW * 730h) * gCO2/kWh / 1000
				kw := (inst.VCPU * 0.015) + (inst.MemoryGB * 0.003)
				mCarbon := (kw * hoursPerMonth * inst.CarbonIntensity) / 1000.0

				isCur := (p.Name() == currentCloud && reg == currentRegion)
				if isCur && currentCost == 0 {
					currentCost = mCost
					currentCarbon = mCarbon
				}

				allOptions = append(allOptions, ArbitrageOption{
					Instance:        inst,
					MonthlyCostUSD:  mCost,
					MonthlyCarbonKg: mCarbon,
					IsCurrent:       isCur,
				})
			}
		}
	}

	if currentCost == 0 {
		currentCost = (reqVCPU * 0.05 + reqMemGB * 0.006) * hoursPerMonth
		currentCarbon = (reqVCPU * 0.015 * hoursPerMonth * 350.0) / 1000.0
	}

	// Calculate savings relative to current cost & carbon
	for i := range allOptions {
		allOptions[i].CostSavingUSD = currentCost - allOptions[i].MonthlyCostUSD
		allOptions[i].CarbonSavingKg = currentCarbon - allOptions[i].MonthlyCarbonKg
	}

	// Sort by highest combined cost & carbon savings
	sort.Slice(allOptions, func(i, j int) bool {
		scoreI := allOptions[i].CostSavingUSD*0.6 + allOptions[i].CarbonSavingKg*0.4
		scoreJ := allOptions[j].CostSavingUSD*0.6 + allOptions[j].CarbonSavingKg*0.4
		return scoreI > scoreJ
	})

	// Read Federated Model stability flag
	minSavingsThreshold := 50.0 // Default $50/mo minimum savings threshold
	if a.aggregator != nil {
		snap := a.aggregator.CurrentSnapshot()
		if snap != nil && snap.IsStable {
			minSavingsThreshold = 15.0 // Bolder cross-cloud recommendations when model is STABLE
		}
	}

	var proposal *optimizationv1alpha1.OptimizationProposal

	if len(allOptions) > 0 && !allOptions[0].IsCurrent && allOptions[0].CostSavingUSD >= minSavingsThreshold {
		best := allOptions[0]

		veleroScript := fmt.Sprintf(
			"#!/usr/bin/env bash\n"+
				"# Automated Reconcilyx Cross-Cloud Migration Script\n"+
				"# Target: %s (%s) [%s]\n"+
				"set -e\n"+
				"echo \"🚀 Backing up workload %s in namespace %s...\"\n"+
				"velero backup create migrate-%s --include-namespaces %s --selector app=%s --wait\n"+
				"echo \"🌐 Restoring workload into target cluster on %s (%s)...\"\n"+
				"velero restore create --from-backup migrate-%s --wait\n"+
				"echo \"✅ Cross-cloud migration complete! Cost saved: $%.2f/mo | Carbon reduced: %.1f kg CO2e/mo\"\n",
			best.Instance.Provider, best.Instance.Region, best.Instance.Name,
			workloadName, namespace, workloadName, namespace, workloadName,
			best.Instance.Provider, best.Instance.Region, workloadName,
			best.CostSavingUSD, best.CarbonSavingKg,
		)

		proposal = &optimizationv1alpha1.OptimizationProposal{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("prop-migrate-%s-%s", workloadName, best.Instance.Region),
				Namespace: namespace,
			},
			Spec: optimizationv1alpha1.OptimizationProposalSpec{
				TargetWorkload: corev1.ObjectReference{
					Kind:      "Deployment",
					Name:      workloadName,
					Namespace: namespace,
				},
				Recommendation: optimizationv1alpha1.RecommendationMigrateCloud,
				ExpectedImpact: optimizationv1alpha1.ExpectedImpact{
					EstimatedSavingsUSD: resource.NewQuantity(int64(best.CostSavingUSD), resource.DecimalSI),
					CarbonSavedGramsCO2: resource.NewQuantity(int64(best.CarbonSavingKg*1000), resource.DecimalSI),
				},
				DecisionMetadata: optimizationv1alpha1.DecisionMetadata{
					ConfidenceScore: 92,
					RiskScore:       15,
					Rationale:       fmt.Sprintf("Multi-cloud arbitrage recommended migrating from %s to %s (%s) saving $%.2f/mo and %.1f kg CO2e/mo", currentCloud, best.Instance.Provider, best.Instance.Name, best.CostSavingUSD, best.CarbonSavingKg),
				},
				MigrationPlan: &optimizationv1alpha1.MigrationPlan{
					SourceCluster:    fmt.Sprintf("cluster-%s", currentCloud),
					TargetCloud:      string(best.Instance.Provider),
					TargetRegion:     best.Instance.Region,
					TargetInstance:   best.Instance.Name,
					CostDeltaUSD:     best.CostSavingUSD,
					CarbonSavedGrams: best.CarbonSavingKg * 1000,
					MigrationScript:  veleroScript,
				},
			},
		}
	}

	return allOptions, proposal, nil
}
