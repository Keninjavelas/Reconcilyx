package telemetry

import (
	"context"
	"strings"
	"testing"

	"github.com/prometheus/common/model"
)

func TestScrapeGPUMetrics(t *testing.T) {
	ctx := context.Background()
	client := &MockMetricsClient{
		QueryFunc: func(_ context.Context, query string) (model.Value, error) {
			switch {
			case strings.HasPrefix(strings.TrimSpace(query), "count("):
				return model.Vector{
					{Metric: model.Metric{"container": "trainer"}, Value: 1, Timestamp: 0},
				}, nil
			case contains(query, "DCGM_FI_DEV_GPU_UTIL"):
				return model.Vector{
					{Metric: model.Metric{"container": "trainer"}, Value: 14.5, Timestamp: 0},
				}, nil
			case contains(query, "DCGM_FI_DEV_FB_USED"):
				return model.Vector{
					{Metric: model.Metric{"container": "trainer"}, Value: 1536, Timestamp: 0},
				}, nil
			case contains(query, "DCGM_FI_DEV_FB_TOTAL"):
				return model.Vector{
					{Metric: model.Metric{"container": "trainer"}, Value: 16384, Timestamp: 0},
				}, nil
			case contains(query, "DCGM_FI_DEV_GPU_TEMP"):
				return model.Vector{
					{Metric: model.Metric{"container": "trainer"}, Value: 42, Timestamp: 0},
				}, nil
			default:
				return model.Vector{}, nil
			}
		},
	}

	gpu, err := ScrapeGPUMetrics(ctx, client, "pytorch-training", "analytics")
	if err != nil {
		t.Fatalf("ScrapeGPUMetrics failed: %v", err)
	}
	if gpu.GPUCount != 1 {
		t.Errorf("expected GPUCount 1, got %d", gpu.GPUCount)
	}
	if gpu.GPUUtilizationPct != 14.5 {
		t.Errorf("expected utilization 14.5, got %f", gpu.GPUUtilizationPct)
	}
	if gpu.GPUMemoryUsedBytes != 1536*1024*1024 {
		t.Errorf("expected used memory in bytes, got %f", gpu.GPUMemoryUsedBytes)
	}
}

func TestScrapeGPUMetricsWithoutExporterReturnsZero(t *testing.T) {
	client := &MockMetricsClient{
		QueryFunc: func(context.Context, string) (model.Value, error) {
			return model.Vector{}, nil
		},
	}
	gpu, err := ScrapeGPUMetrics(context.Background(), client, "web", "default")
	if err != nil {
		t.Fatalf("ScrapeGPUMetrics failed: %v", err)
	}
	if gpu.GPUCount != 0 || gpu.GPUUtilizationPct != 0 {
		t.Fatalf("expected zero GPU metrics without DCGM, got %+v", gpu)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
