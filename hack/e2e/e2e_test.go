package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/reconcilyx/operator/internal/cloud"
	"github.com/reconcilyx/operator/internal/engine"
	"github.com/reconcilyx/operator/internal/federated"
	"github.com/reconcilyx/operator/internal/llm"
	"github.com/reconcilyx/operator/internal/telemetry"
)

func TestFullPlatformIntegrationPipeline(t *testing.T) {
	ctx := context.Background()

	// -------------------------------------------------------------
	// Part 1: Spoke Telemetry & Federated Aggregation (Pillar 1)
	// -------------------------------------------------------------
	agg := federated.NewAggregator(1000)

	// Simulate Spoke 1 & Spoke 2 emitting RL Q-table deltas
	spoke1Delta := federated.QTableDelta{
		ClusterName: "spoke-us-east-1",
		StateKey:    federated.QStateKey{CPUBucket: 5, MemBucket: 10, CPUReqMilli: 1000, MemReqMilli: 2048, JVMBucket: 1},
		ActionKey:   federated.QActionKey{CPUAdjPct: -20, MemAdjPct: -10},
		DeltaQ:      0.45,
		VisitCount:  25,
		Timestamp:   time.Now(),
	}

	spoke2Delta := federated.QTableDelta{
		ClusterName: "spoke-eu-west-1",
		StateKey:    federated.QStateKey{CPUBucket: 5, MemBucket: 10, CPUReqMilli: 1000, MemReqMilli: 2048, JVMBucket: 1},
		ActionKey:   federated.QActionKey{CPUAdjPct: -20, MemAdjPct: -10},
		DeltaQ:      0.48,
		VisitCount:  30,
		Timestamp:   time.Now(),
	}

	agg.AddDeltas([]federated.QTableDelta{spoke1Delta, spoke2Delta})
	snap := agg.CompactAndSnapshot()

	if len(snap.Entries) == 0 {
		t.Fatalf("E2E Pipeline Failure: Global Q-table entries empty after aggregation")
	}

	// Mark model stable for testing cross-cloud & cold-start bootstrap
	agg.LoadSnapshot(&federated.GlobalQTable{
		Version:  1,
		IsStable: true,
		Entries:  snap.Entries,
	})

	// -------------------------------------------------------------
	// Part 2: Multi-Cloud Arbitrage & Migration Proposals (Pillar 2)
	// -------------------------------------------------------------
	arbitrageAnalyzer := engine.NewArbitrageAnalyzer(agg)
	options, proposal, err := arbitrageAnalyzer.EvaluateArbitrage(
		ctx,
		"payment-service",
		"production",
		4.0,
		16.0,
		cloud.ProviderAWS,
		"us-east-1",
	)

	if err != nil {
		t.Fatalf("E2E Pipeline Failure: EvaluateArbitrage error: %v", err)
	}

	if len(options) == 0 || proposal == nil {
		t.Fatalf("E2E Pipeline Failure: Expected MigrateCloud proposal for cross-cloud arbitrage")
	}

	if proposal.Spec.MigrationPlan == nil || !strings.Contains(proposal.Spec.MigrationPlan.MigrationScript, "velero backup create") {
		t.Fatalf("E2E Pipeline Failure: Migration script not embedded in proposal")
	}

	// -------------------------------------------------------------
	// Part 3: Application-Layer JVM & GPU Intelligence (Pillar 3)
	// -------------------------------------------------------------
	cmd := []string{"java"}
	args := []string{"-Xms512m", "-Xmx2048m", "-jar", "app.jar"}
	newMemBytes := 1024.0 * 1024.0 * 1024.0 // 1GB memory limit

	_, newArgs := engine.GenerateJVMCommandOverride(cmd, args, newMemBytes)
	if newArgs[1] != "-Xmx768m" { // 75% of 1024MB = 768MB
		t.Fatalf("E2E Pipeline Failure: JVM -Xmx command override failed, got %s", newArgs[1])
	}

	gpuClient := &telemetry.MockMetricsClient{
		QueryFunc: func(_ context.Context, query string) (model.Value, error) {
			switch {
			case strings.HasPrefix(strings.TrimSpace(query), "count("):
				return model.Vector{{Metric: model.Metric{"container": "trainer"}, Value: 2}}, nil
			case strings.Contains(query, "DCGM_FI_DEV_GPU_UTIL"):
				return model.Vector{{Metric: model.Metric{"container": "trainer"}, Value: 87.5}}, nil
			case strings.Contains(query, "DCGM_FI_DEV_FB_USED"):
				return model.Vector{{Metric: model.Metric{"container": "trainer"}, Value: 8192}}, nil
			case strings.Contains(query, "DCGM_FI_DEV_FB_TOTAL"):
				return model.Vector{{Metric: model.Metric{"container": "trainer"}, Value: 16384}}, nil
			case strings.Contains(query, "DCGM_FI_DEV_GPU_TEMP"):
				return model.Vector{{Metric: model.Metric{"container": "trainer"}, Value: 68}}, nil
			default:
				return model.Vector{}, nil
			}
		},
	}
	gpuMetrics, err := telemetry.ScrapeGPUMetrics(ctx, gpuClient, "pytorch-training", "analytics")
	if err != nil || gpuMetrics.GPUCount <= 0 {
		t.Fatalf("E2E Pipeline Failure: GPU telemetry scraping failed: %v", err)
	}

	// -------------------------------------------------------------
	// Part 4: BYOK ChatOps Natural Language Assistant (Pillar 4)
	// -------------------------------------------------------------
	provider, _ := llm.NewProvider("ollama", nil)
	router := llm.NewHybridRouter(provider)

	routeResult, err := router.Route(ctx, "Why is my bill so high?")
	if err != nil || routeResult.FunctionName != "getSavings" {
		t.Fatalf("E2E Pipeline Failure: Hybrid router mapped to %s instead of getSavings", routeResult.FunctionName)
	}

	t.Logf("✅ Full Reconcilyx Platform Integration Pipeline PASSED! (All 4 Pillars Verified)")
}
