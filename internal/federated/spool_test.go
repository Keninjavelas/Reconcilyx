package federated

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/reconcilyx/operator/pkg/pipelinemetrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmitterDiskSpoolAndFIFORecovery(t *testing.T) {
	spoolDir := t.TempDir()

	var hubUp atomic.Bool
	var receivedBatches []DeltaBatch
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hubUp.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var b DeltaBatch
		require.NoError(t, json.Unmarshal(raw, &b))
		receivedBatches = append(receivedBatches, b)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	e := NewEmitter(EmitterConfig{
		HubBaseURL:     ts.URL,
		ClusterName:    "spoke-west",
		FlushThreshold: 100,
		FlushInterval:  time.Hour,
		SpoolDir:       spoolDir,
		MaxSpoolBytes:  DefaultMaxSpoolBytes,
	})

	// Hub is down: flush two batches sequentially -> both persist to disk spool.
	hubUp.Store(false)
	e.Emit(QTableDelta{StateKey: QStateKey{CPUBucket: 1}, ActionKey: QActionKey{CPUAdjPct: -10}, DeltaQ: 0.11})
	require.Error(t, e.Flush(context.Background()))

	e.Emit(QTableDelta{StateKey: QStateKey{CPUBucket: 2}, ActionKey: QActionKey{CPUAdjPct: -20}, DeltaQ: 0.22})
	require.Error(t, e.Flush(context.Background()))

	queued, err := e.spool.Count()
	require.NoError(t, err)
	assert.Equal(t, 2, queued)
	assert.Equal(t, float64(2), testutil.ToFloat64(pipelinemetrics.SpokeSpoolQueuedBatches.WithLabelValues("spoke-west")))

	// Hub recovers: new flush drains batch 1, then batch 2 FIFO, then batch 3.
	hubUp.Store(true)
	e.Emit(QTableDelta{StateKey: QStateKey{CPUBucket: 3}, ActionKey: QActionKey{CPUAdjPct: -30}, DeltaQ: 0.33})
	require.NoError(t, e.Flush(context.Background()))

	queuedAfter, err := e.spool.Count()
	require.NoError(t, err)
	assert.Equal(t, 0, queuedAfter)
	assert.Equal(t, float64(0), testutil.ToFloat64(pipelinemetrics.SpokeSpoolQueuedBatches.WithLabelValues("spoke-west")))

	require.Len(t, receivedBatches, 3)
	assert.Equal(t, int64(1), receivedBatches[0].Deltas[0].StateKey.CPUBucket)
	assert.Equal(t, int64(2), receivedBatches[1].Deltas[0].StateKey.CPUBucket)
	assert.Equal(t, int64(3), receivedBatches[2].Deltas[0].StateKey.CPUBucket)
	assert.NotEmpty(t, receivedBatches[0].BatchID)
}

func TestAggregatorDeduplicatesBatchID(t *testing.T) {
	agg := NewAggregator(100)
	deltas := []QTableDelta{{
		ClusterName: "spoke-a",
		StateKey:    QStateKey{CPUBucket: 5, MemBucket: 2},
		ActionKey:   QActionKey{CPUAdjPct: -10},
		DeltaQ:      0.5,
		VisitCount:  4,
		Epoch:       1,
		Timestamp:   time.Unix(1700000000, 0).UTC(),
	}}
	batchID := ComputeBatchID("spoke-a", deltas)

	assert.True(t, agg.AddBatch(batchID, deltas), "first batch delivery should be accepted")
	assert.False(t, agg.AddBatch(batchID, deltas), "replayed duplicate batchID must be dropped")
	assert.Equal(t, 1, agg.PendingDeltaCount())
}

func TestDiskSpoolEnforcesMaxBytesFIFO(t *testing.T) {
	spool, err := NewDiskSpool(t.TempDir(), 450)
	require.NoError(t, err)

	for i := 1; i <= 3; i++ {
		err := spool.Enqueue(DeltaBatch{
			ClusterName: "capped",
			Epoch:       int64(i),
			Deltas: []QTableDelta{{
				ClusterName: "capped",
				StateKey:    QStateKey{CPUBucket: int64(i)},
				DeltaQ:      float64(i) * 0.1,
				Epoch:       int64(i),
				Timestamp:   time.Unix(1700000000+int64(i), 0).UTC(),
			}},
		})
		require.NoError(t, err)
	}

	entries, err := spool.List()
	require.NoError(t, err)
	require.LessOrEqual(t, len(entries), 2, "oldest entries must be evicted to stay within MaxSpoolBytes")
	// Newest batch (CPUBucket 3) must remain preserved.
	assert.Equal(t, int64(3), entries[len(entries)-1].Batch.Deltas[0].StateKey.CPUBucket)
}
