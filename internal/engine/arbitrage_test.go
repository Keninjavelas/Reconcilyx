package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/reconcilyx/operator/internal/cloud"
	"github.com/reconcilyx/operator/internal/federated"
)

func TestArbitrageAnalyzer(t *testing.T) {
	ctx := context.Background()

	// 1. Initialize Aggregator with a STABLE snapshot
	agg := federated.NewAggregator(1000)
	agg.LoadSnapshot(&federated.GlobalQTable{
		Version:  10,
		IsStable: true,
		Entries: map[string]federated.GlobalQEntry{
			"test-key": {QValue: 0.8, VisitCount: 100},
		},
	})

	analyzer := NewArbitrageAnalyzer(agg)

	// 2. Evaluate Arbitrage for a workload running in AWS us-east-1 (m6i.xlarge)
	options, proposal, err := analyzer.EvaluateArbitrage(
		ctx,
		"payment-service",
		"production",
		4.0,  // 4 vCPUs
		16.0, // 16 GB RAM
		cloud.ProviderAWS,
		"us-east-1",
	)

	if err != nil {
		t.Fatalf("EvaluateArbitrage failed: %v", err)
	}

	if len(options) == 0 {
		t.Fatalf("Expected non-empty arbitrage options matrix")
	}

	// 3. Verify proposal generation
	if proposal == nil {
		t.Fatalf("Expected MigrateCloud proposal to be generated for cheaper/greener target cloud")
	}

	if proposal.Spec.Recommendation != "MigrateCloud" {
		t.Errorf("Expected recommendation type MigrateCloud, got %s", proposal.Spec.Recommendation)
	}

	if proposal.Spec.MigrationPlan == nil {
		t.Fatalf("Expected MigrationPlan struct to be populated")
	}

	plan := proposal.Spec.MigrationPlan
	if plan.TargetCloud == "" || plan.TargetRegion == "" || plan.TargetInstance == "" {
		t.Errorf("Incomplete MigrationPlan fields: %+v", plan)
	}

	// 4. Verify Velero migration script is embedded
	if !strings.Contains(plan.MigrationScript, "velero backup create") || !strings.Contains(plan.MigrationScript, "velero restore create") {
		t.Errorf("MigrationScript does not contain valid Velero backup/restore commands:\n%s", plan.MigrationScript)
	}
}
