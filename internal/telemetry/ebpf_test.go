package telemetry

import (
	"context"
	"testing"
	"time"
)

func TestEBPFMetricsStreamer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()

	streamer := NewEBPFMetricsStreamer()
	ch, err := streamer.StreamRealtimeMetrics(ctx, "payment-service-x1", "production")
	if err != nil {
		t.Fatalf("StreamRealtimeMetrics failed: %v", err)
	}

	samplesCount := 0
	for sample := range ch {
		samplesCount++
		if sample.PodName != "payment-service-x1" {
			t.Errorf("Unexpected pod name: %s", sample.PodName)
		}
	}

	if samplesCount < 2 {
		t.Errorf("Expected sub-second streaming to produce >= 2 samples, got %d", samplesCount)
	}
}
