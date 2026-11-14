package telemetry

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/reconcilyx/operator/pkg/pipelinemetrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockWindowedClient struct {
	cpuMatrix model.Matrix
	memMatrix model.Matrix
	calls     int
}

func (m *mockWindowedClient) QueryRangeWindow(_ context.Context, query string, _ time.Time, _ time.Time, _ time.Duration) (model.Value, error) {
	m.calls++
	if strings.Contains(query, "container_cpu_usage_seconds_total") {
		return m.cpuMatrix, nil
	}
	return m.memMatrix, nil
}

func TestBackfillEngineIdempotentWindowReplayAndWatermarks(t *testing.T) {
	t0 := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(5 * time.Minute)
	t2 := t0.Add(10 * time.Minute)

	metric := model.Metric{
		"namespace": "payments",
		"pod":       "checkout-api-abc",
		"uid":       "uid-checkout-1",
	}

	client := &mockWindowedClient{
		cpuMatrix: model.Matrix{
			&model.SampleStream{
				Metric: metric,
				Values: []model.SamplePair{
					{Timestamp: model.TimeFromUnix(t0.Unix()), Value: 0.25},
					{Timestamp: model.TimeFromUnix(t1.Unix()), Value: 0.42},
				},
			},
		},
		memMatrix: model.Matrix{
			&model.SampleStream{
				Metric: metric,
				Values: []model.SamplePair{
					{Timestamp: model.TimeFromUnix(t0.Unix()), Value: 256 * 1024 * 1024},
					{Timestamp: model.TimeFromUnix(t1.Unix()), Value: 300 * 1024 * 1024},
				},
			},
		},
	}

	engine := NewBackfillEngine(client)
	engine.RecordEvaluation("payments", t0)
	assert.True(t, engine.NeedsBackfill("payments", t2, time.Minute), "gap 10m > 2*1m must require backfill")
	assert.False(t, engine.NeedsBackfill("payments", t0.Add(time.Minute), time.Minute), "gap 1m <= 2*1m must not trigger backfill")

	beforeIngested := testutil.ToFloat64(pipelinemetrics.BackfillWindowsTotal.WithLabelValues("payments", "ingested"))
	beforeSkipped := testutil.ToFloat64(pipelinemetrics.BackfillWindowsTotal.WithLabelValues("payments", "skipped_duplicate"))

	// First run ingests 2 buckets.
	rep1, err := engine.Backfill(context.Background(), BackfillRequest{
		Namespace: "payments",
		From:      t0,
		To:        t2,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, rep1.BucketsIngested)
	assert.Equal(t, 0, rep1.BucketsSkippedDuplicate)
	assert.Equal(t, 2, engine.BucketCount())
	assert.Equal(t, t2, rep1.NewWatermark)

	// Re-running exact same [from, to] window must skip both buckets with zero double-counting.
	rep2, err := engine.Backfill(context.Background(), BackfillRequest{
		Namespace: "payments",
		From:      t0,
		To:        t2,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, rep2.BucketsIngested, "re-running identical window must ingest 0 duplicates")
	assert.Equal(t, 2, rep2.BucketsSkippedDuplicate)
	assert.Equal(t, 2, engine.BucketCount(), "bucket store cardinality must remain unchanged")

	afterIngested := testutil.ToFloat64(pipelinemetrics.BackfillWindowsTotal.WithLabelValues("payments", "ingested"))
	afterSkipped := testutil.ToFloat64(pipelinemetrics.BackfillWindowsTotal.WithLabelValues("payments", "skipped_duplicate"))
	assert.Equal(t, float64(2), afterIngested-beforeIngested)
	assert.Equal(t, float64(2), afterSkipped-beforeSkipped)
}

func TestBackfillEngineRejectsInvalidTimeRange(t *testing.T) {
	engine := NewBackfillEngine(&mockWindowedClient{})
	t0 := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	_, err := engine.Backfill(context.Background(), BackfillRequest{
		Namespace: "default",
		From:      t0,
		To:        t0,
	})
	require.Error(t, err)
}
