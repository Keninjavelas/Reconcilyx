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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmitterConfig_WithDefaults(t *testing.T) {
	cfg := EmitterConfig{}.withDefaults()
	assert.Equal(t, 5000, cfg.BufferCapacity)
	assert.Equal(t, time.Hour, cfg.FlushInterval)
	assert.Equal(t, 100, cfg.FlushThreshold)
	assert.NotNil(t, cfg.HTTPClient)
	assert.Equal(t, 30*time.Second, cfg.HTTPClient.Timeout)
}

func TestEmitterConfig_WithDefaults_PreservesUserValues(t *testing.T) {
	customClient := &http.Client{Timeout: 10 * time.Second}
	cfg := EmitterConfig{
		BufferCapacity: 10,
		FlushInterval:  2 * time.Minute,
		FlushThreshold: 5,
		HTTPClient:     customClient,
	}.withDefaults()
	assert.Equal(t, 10, cfg.BufferCapacity)
	assert.Equal(t, 2*time.Minute, cfg.FlushInterval)
	assert.Equal(t, 5, cfg.FlushThreshold)
	assert.Same(t, customClient, cfg.HTTPClient)
}

func TestNewEmitter(t *testing.T) {
	e := NewEmitter(EmitterConfig{
		ClusterName: "cluster-1",
		HubBaseURL:  "http://hub.example",
	})
	assert.Equal(t, "cluster-1", e.cfg.ClusterName)
	assert.Equal(t, "http://hub.example", e.cfg.HubBaseURL)
	assert.Equal(t, 0, e.BufferedCount())
}

func TestEmitter_Emit_SingleDelta(t *testing.T) {
	e := NewEmitter(EmitterConfig{ClusterName: "alpha"})
	delta := QTableDelta{
		StateKey:  QStateKey{CPUBucket: 1},
		ActionKey: QActionKey{CPUAdjPct: 10},
		DeltaQ:    0.5,
	}
	n := e.Emit(delta)
	assert.Equal(t, 1, n)
	assert.Equal(t, 1, e.BufferedCount())
}

func TestEmitter_Emit_SetsClusterNameAndTimestamp(t *testing.T) {
	e := NewEmitter(EmitterConfig{ClusterName: "beta"})
	e.Emit(QTableDelta{
		StateKey:  QStateKey{CPUBucket: 1},
		ActionKey: QActionKey{CPUAdjPct: 0},
		DeltaQ:    0.1,
	})
	assert.Equal(t, 1, len(e.buffer))
	assert.Equal(t, "beta", e.buffer[0].ClusterName)
	assert.False(t, e.buffer[0].Timestamp.IsZero())
	assert.Equal(t, int64(1), e.buffer[0].Epoch)
}

func TestEmitter_Emit_IncrementEpoch(t *testing.T) {
	e := NewEmitter(EmitterConfig{ClusterName: "c"})
	for i := 0; i < 3; i++ {
		e.Emit(QTableDelta{
			StateKey:  QStateKey{CPUBucket: int64(i)},
			ActionKey: QActionKey{CPUAdjPct: 0},
			DeltaQ:    float64(i) * 0.1,
		})
	}
	assert.Equal(t, int64(1), e.buffer[0].Epoch)
	assert.Equal(t, int64(2), e.buffer[1].Epoch)
	assert.Equal(t, int64(3), e.buffer[2].Epoch)
}

func TestEmitter_Emit_ObserverCalled(t *testing.T) {
	e := NewEmitter(EmitterConfig{ClusterName: "obs"})
	var observed []QTableDelta
	var count int32
	e.SetObserver(func(d QTableDelta) {
		atomic.AddInt32(&count, 1)
		observed = append(observed, d)
	})
	e.Emit(QTableDelta{
		StateKey:  QStateKey{CPUBucket: 5},
		ActionKey: QActionKey{CPUAdjPct: -10},
		DeltaQ:    -0.3,
	})
	assert.Equal(t, int32(1), atomic.LoadInt32(&count))
	assert.Len(t, observed, 1)
	assert.Equal(t, "obs", observed[0].ClusterName)
	assert.InDelta(t, -0.3, observed[0].DeltaQ, 1e-9)
}

func TestEmitter_Emit_AutoFlushAtThreshold(t *testing.T) {
	var received int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&received, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	e := NewEmitter(EmitterConfig{
		HubBaseURL:     ts.URL,
		ClusterName:    "spoke-1",
		FlushThreshold: 2,
		FlushInterval:  24 * time.Hour,
	})
	e.Emit(QTableDelta{
		StateKey:  QStateKey{CPUBucket: 1},
		ActionKey: QActionKey{CPUAdjPct: 10},
		DeltaQ:    0.1,
	})
	assert.Equal(t, 1, e.BufferedCount())
	assert.Equal(t, int32(0), atomic.LoadInt32(&received))
	e.Emit(QTableDelta{
		StateKey:  QStateKey{CPUBucket: 2},
		ActionKey: QActionKey{CPUAdjPct: -10},
		DeltaQ:    0.2,
	})
	assert.Eventually(t, func() bool {
		return atomic.LoadInt32(&received) == 1
	}, 2*time.Second, 10*time.Millisecond)
	assert.Equal(t, 0, e.BufferedCount(), "buffer should be emptied after flush")
}

func TestEmitter_Flush_EmptyBufferIsNoOp(t *testing.T) {
	e := NewEmitter(EmitterConfig{HubBaseURL: "http://unused", ClusterName: "n"})
	err := e.Flush(context.Background())
	assert.NoError(t, err)
}

func TestEmitter_Flush_NoHubURLReturnsError(t *testing.T) {
	e := NewEmitter(EmitterConfig{ClusterName: "no-hub"})
	e.Emit(QTableDelta{
		StateKey:  QStateKey{CPUBucket: 1},
		ActionKey: QActionKey{CPUAdjPct: 0},
		DeltaQ:    0.1,
	})
	err := e.Flush(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "HubBaseURL is not configured")
}

func TestEmitter_SendBatch_Success(t *testing.T) {
	var captured DeltaBatch
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/federated/deltas", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &captured))
		w.WriteHeader(http.StatusAccepted)
	}))
	defer ts.Close()

	e := NewEmitter(EmitterConfig{
		HubBaseURL:  ts.URL,
		ClusterName: "ok-cluster",
	})
	e.Emit(QTableDelta{
		StateKey:   QStateKey{CPUBucket: 3, MemBucket: 2},
		ActionKey:  QActionKey{CPUAdjPct: 20, MemAdjPct: 10},
		DeltaQ:     0.42,
		VisitCount: 7,
	})
	err := e.Flush(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "ok-cluster", captured.ClusterName)
	assert.Equal(t, 1, len(captured.Deltas))
	assert.InDelta(t, 0.42, captured.Deltas[0].DeltaQ, 1e-9)
	assert.Equal(t, uint64(7), captured.Deltas[0].VisitCount)
}

func TestEmitter_SendBatch_WithAPIKey(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "secret-key", r.Header.Get("X-API-Key"))
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	e := NewEmitter(EmitterConfig{
		HubBaseURL:  ts.URL,
		ClusterName: "auth-cluster",
		APIKey:      "secret-key",
	})
	e.Emit(QTableDelta{
		StateKey:  QStateKey{CPUBucket: 1},
		ActionKey: QActionKey{CPUAdjPct: 0},
		DeltaQ:    0.1,
	})
	err := e.Flush(context.Background())
	assert.NoError(t, err)
}

func TestEmitter_SendBatch_HubRejects(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	e := NewEmitter(EmitterConfig{
		HubBaseURL:  ts.URL,
		ClusterName: "err-cluster",
	})
	e.Emit(QTableDelta{
		StateKey:  QStateKey{CPUBucket: 1},
		ActionKey: QActionKey{CPUAdjPct: 0},
		DeltaQ:    0.1,
	})
	err := e.Flush(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "hub rejected batch")
}

func TestEmitter_Run_ContextCancelFlushes(t *testing.T) {
	var flushed int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&flushed, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	e := NewEmitter(EmitterConfig{
		HubBaseURL:     ts.URL,
		ClusterName:    "ctx-cluster",
		FlushInterval:  24 * time.Hour,
		FlushThreshold: 999,
	})
	e.Emit(QTableDelta{
		StateKey:  QStateKey{CPUBucket: 9},
		ActionKey: QActionKey{CPUAdjPct: 5},
		DeltaQ:    0.5,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- e.Run(ctx)
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
	assert.Eventually(t, func() bool {
		return atomic.LoadInt32(&flushed) >= 1
	}, 2*time.Second, 10*time.Millisecond)
}

func TestEmitter_Run_PeriodicFlush(t *testing.T) {
	var flushed int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&flushed, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	e := NewEmitter(EmitterConfig{
		HubBaseURL:     ts.URL,
		ClusterName:    "periodic",
		FlushInterval:  40 * time.Millisecond,
		FlushThreshold: 999,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = e.Run(ctx) }()

	time.Sleep(20 * time.Millisecond)
	e.Emit(QTableDelta{
		StateKey:  QStateKey{CPUBucket: 1},
		ActionKey: QActionKey{CPUAdjPct: 0},
		DeltaQ:    0.1,
	})
	assert.Eventually(t, func() bool {
		return atomic.LoadInt32(&flushed) >= 1
	}, 400*time.Millisecond, 10*time.Millisecond, "first periodic flush should fire via ticker")

	e.Emit(QTableDelta{
		StateKey:  QStateKey{CPUBucket: 2},
		ActionKey: QActionKey{CPUAdjPct: 10},
		DeltaQ:    0.2,
	})
	assert.Eventually(t, func() bool {
		return atomic.LoadInt32(&flushed) >= 2
	}, 400*time.Millisecond, 10*time.Millisecond, "second flush should fire on next ticker interval")
}

func TestItoa(t *testing.T) {
	assert.Equal(t, "0", itoa(0))
	assert.Equal(t, "1", itoa(1))
	assert.Equal(t, "42", itoa(42))
	assert.Equal(t, "1234567890", itoa(1234567890))
	assert.Equal(t, "-7", itoa(-7))
	assert.Equal(t, "-12345", itoa(-12345))
}
