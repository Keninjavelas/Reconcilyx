package federated

import (
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
