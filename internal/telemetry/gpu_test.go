package telemetry

import (
	"context"
	"testing"
)

func TestScrapeGPUMetrics(t *testing.T) {
	ctx := context.Background()

	gpu, err := ScrapeGPUMetrics(ctx, nil, "pytorch-training", "analytics")
	if err != nil {
		t.Fatalf("ScrapeGPUMetrics failed: %v", err)
	}

	if gpu.GPUCount <= 0 {
		t.Errorf("Expected GPUCount > 0, got %d", gpu.GPUCount)
	}

	if gpu.GPUUtilizationPct < 0 || gpu.GPUUtilizationPct > 100 {
		t.Errorf("Invalid GPUUtilizationPct: %f", gpu.GPUUtilizationPct)
	}
}
