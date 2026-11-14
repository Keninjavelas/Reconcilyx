package federated

import (
	"math"
	"testing"
	"time"
)

func TestByzantineOutlierRejection(t *testing.T) {
	agg := NewAggregator(1000)

	state := QStateKey{CPUBucket: 5, MemBucket: 10, CPUReqMilli: 500, MemReqMilli: 1024}
	action := QActionKey{CPUAdjPct: -10, MemAdjPct: -5}

	// 1. Submit normal cluster deltas
	normalDeltas := []QTableDelta{
		{
			ClusterName: "cluster-alpha",
			StateKey:    state,
			ActionKey:   action,
			DeltaQ:      0.25,
			VisitCount:  10,
			Timestamp:   time.Now(),
		},
		{
			ClusterName: "cluster-beta",
			StateKey:    state,
			ActionKey:   action,
			DeltaQ:      0.28,
			VisitCount:  12,
			Timestamp:   time.Now(),
		},
	}

	// 2. Submit rogue poisoned delta (+100.0)
	rogueDelta := QTableDelta{
		ClusterName: "rogue-malicious-cluster",
		StateKey:    state,
		ActionKey:   action,
		DeltaQ:      100.0, // Poisoned delta
		VisitCount:  1000,
		Timestamp:   time.Now(),
	}

	agg.AddDeltas(normalDeltas)
	agg.AddDeltas([]QTableDelta{rogueDelta})

	snap := agg.CompactAndSnapshot()

	if snap.OutlierRejectedCount != 1 {
		t.Errorf("Expected 1 rejected outlier delta, got %d", snap.OutlierRejectedCount)
	}

	key := EntryKey(state, action)
	entry, ok := snap.Entries[key]
	if !ok {
		t.Fatalf("Expected key %s to exist in global model", key)
	}

	// Verify QValue wasn't poisoned to near 100
	if entry.QValue > 1.0 {
		t.Errorf("Byzantine aggregation failed! QValue was poisoned to %f", entry.QValue)
	}
}

func TestTrimmedMeanAggregation(t *testing.T) {
	agg := NewAggregator(1000)

	state := QStateKey{CPUBucket: 2, MemBucket: 4, CPUReqMilli: 200, MemReqMilli: 512}
	action := QActionKey{CPUAdjPct: 5, MemAdjPct: 5}

	// Submit 5 deltas: 0.10, 0.40, 0.42, 0.45, 2.50 (2.50 is high end, 0.10 low end)
	deltas := []QTableDelta{
		{ClusterName: "c1", StateKey: state, ActionKey: action, DeltaQ: 0.10, VisitCount: 1},
		{ClusterName: "c2", StateKey: state, ActionKey: action, DeltaQ: 0.40, VisitCount: 1},
		{ClusterName: "c3", StateKey: state, ActionKey: action, DeltaQ: 0.42, VisitCount: 1},
		{ClusterName: "c4", StateKey: state, ActionKey: action, DeltaQ: 0.45, VisitCount: 1},
		{ClusterName: "c5", StateKey: state, ActionKey: action, DeltaQ: 2.50, VisitCount: 1},
	}

	agg.AddDeltas(deltas)
	snap := agg.CompactAndSnapshot()

	key := EntryKey(state, action)
	entry := snap.Entries[key]

	// Expected trimmed mean should ignore outliers and remain near 0.42
	if entry.QValue > 1.0 || entry.QValue < 0.3 {
		t.Errorf("Trimmed mean failed! Expected QValue ~0.42, got %f", entry.QValue)
	}
}

func TestModelStabilityAndConvergence(t *testing.T) {
	agg := NewAggregator(1000)

	state := QStateKey{CPUBucket: 1, MemBucket: 1, CPUReqMilli: 100, MemReqMilli: 256}
	action := QActionKey{CPUAdjPct: 0, MemAdjPct: 0}

	// 1st iteration: large delta
	agg.AddDeltas([]QTableDelta{
		{ClusterName: "c1", StateKey: state, ActionKey: action, DeltaQ: 0.50, VisitCount: 10},
	})
	snap1 := agg.CompactAndSnapshot()
	if snap1.IsStable {
		t.Errorf("Expected model to NOT be stable on 1st large iteration")
	}

	// 2nd iteration: tiny delta < 0.005
	agg.AddDeltas([]QTableDelta{
		{ClusterName: "c1", StateKey: state, ActionKey: action, DeltaQ: 0.5001, VisitCount: 1000},
	})
	snap2 := agg.CompactAndSnapshot()
	if snap2.ConvergenceScore >= 0.005 {
		t.Errorf("Expected convergence score < 0.005, got %f", snap2.ConvergenceScore)
	}
	if !snap2.IsStable {
		t.Errorf("Expected model to be flagged IS_STABLE=true")
	}
}

func TestAggregator_ShouldCompactDualTrigger(t *testing.T) {
	agg := NewAggregator(100)
	state := QStateKey{CPUBucket: 1, MemBucket: 2, CPUReqMilli: 100, MemReqMilli: 128}
	action := QActionKey{CPUAdjPct: -10, MemAdjPct: 0}

	// No pending deltas -> never compact
	if agg.ShouldCompact(1, time.Millisecond) {
		t.Fatalf("expected ShouldCompact=false with 0 pending deltas")
	}

	agg.AddDeltas([]QTableDelta{
		{ClusterName: "s1", StateKey: state, ActionKey: action, DeltaQ: 0.2, VisitCount: 1},
		{ClusterName: "s2", StateKey: state, ActionKey: action, DeltaQ: 0.3, VisitCount: 1},
	})

	// Below threshold and below maxAge -> false
	if agg.ShouldCompact(5, time.Hour) {
		t.Fatalf("expected ShouldCompact=false when pending=2 < 5 and interval=1h")
	}
	// At/above threshold (even non-500 multiple) -> true
	if !agg.ShouldCompact(2, time.Hour) {
		t.Fatalf("expected ShouldCompact=true when pending=2 >= threshold=2")
	}

	// Simulate aged lastCompact -> true even with pending < threshold
	agg.mu.Lock()
	agg.lastCompact = time.Now().UTC().Add(-20 * time.Minute)
	agg.mu.Unlock()
	if !agg.ShouldCompact(500, 15*time.Minute) {
		t.Fatalf("expected ShouldCompact=true when compactMaxAge elapsed")
	}
}

func TestAggregator_CanonicalModelHashDeterminism(t *testing.T) {
	s1 := QStateKey{CPUBucket: 1, MemBucket: 1, CPUReqMilli: 100, MemReqMilli: 128}
	s2 := QStateKey{CPUBucket: 2, MemBucket: 2, CPUReqMilli: 200, MemReqMilli: 256}
	a1 := QActionKey{CPUAdjPct: -10, MemAdjPct: -10}

	aggA := NewAggregator(100)
	aggA.AddDeltas([]QTableDelta{
		{ClusterName: "c1", StateKey: s1, ActionKey: a1, DeltaQ: 0.25, VisitCount: 5},
		{ClusterName: "c2", StateKey: s2, ActionKey: a1, DeltaQ: 0.45, VisitCount: 8},
	})
	snapA := aggA.CompactAndSnapshot()

	aggB := NewAggregator(100)
	aggB.AddDeltas([]QTableDelta{
		{ClusterName: "c2", StateKey: s2, ActionKey: a1, DeltaQ: 0.45, VisitCount: 8},
		{ClusterName: "c1", StateKey: s1, ActionKey: a1, DeltaQ: 0.25, VisitCount: 5},
	})
	snapB := aggB.CompactAndSnapshot()

	if snapA.ModelHash == "" || snapA.ModelHash != snapB.ModelHash {
		t.Fatalf("expected identical deterministic ModelHash, got %q vs %q", snapA.ModelHash, snapB.ModelHash)
	}
}

func FuzzTrimmedMeanAggregation(f *testing.F) {
	f.Add(0.20, 0.22, 0.25, 0.28, 999.0, uint64(10000))
	f.Add(-0.30, -0.25, -0.20, -0.15, -500.0, uint64(5000))
	f.Add(0.10, 0.12, 0.14, 0.16, 4.99, uint64(1))

	state := QStateKey{CPUBucket: 3, MemBucket: 4, CPUReqMilli: 300, MemReqMilli: 512}
	action := QActionKey{CPUAdjPct: -10, MemAdjPct: -10}
	key := EntryKey(state, action)

	f.Fuzz(func(t *testing.T, q1, q2, q3, q4, poisonQ float64, poisonVisits uint64) {
		for _, v := range []float64{q1, q2, q3, q4} {
			if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 1.0 {
				return
			}
		}
		agg := NewAggregator(1000)
		agg.AddDeltas([]QTableDelta{
			{ClusterName: "h1", StateKey: state, ActionKey: action, DeltaQ: q1, VisitCount: 10},
			{ClusterName: "h2", StateKey: state, ActionKey: action, DeltaQ: q2, VisitCount: 10},
			{ClusterName: "h3", StateKey: state, ActionKey: action, DeltaQ: q3, VisitCount: 10},
			{ClusterName: "h4", StateKey: state, ActionKey: action, DeltaQ: q4, VisitCount: 10},
			{ClusterName: "poison", StateKey: state, ActionKey: action, DeltaQ: poisonQ, VisitCount: poisonVisits},
		})
		snap := agg.CompactAndSnapshot()
		entry, ok := snap.Entries[key]
		if !ok {
			t.Fatalf("missing aggregated entry for %s", key)
		}
		if math.IsNaN(entry.QValue) || math.IsInf(entry.QValue, 0) {
			t.Fatalf("aggregated QValue must be finite, got %v", entry.QValue)
		}
		if math.Abs(entry.QValue) > DefaultMaxDeltaQ {
			t.Fatalf("poisoned delta skewed QValue beyond cutoff: %v", entry.QValue)
		}
	})
}
