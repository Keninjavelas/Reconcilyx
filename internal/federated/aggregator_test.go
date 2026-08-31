package federated

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewAggregator_Defaults(t *testing.T) {
	a := NewAggregator(0)
	assert.NotNil(t, a)
	assert.Equal(t, 200000, a.maxDeltas)
	assert.Equal(t, int64(0), a.global.Version)
	assert.NotNil(t, a.global.Entries)
	assert.Equal(t, 0, len(a.global.Entries))
}

func TestNewAggregator_CustomMaxBuffer(t *testing.T) {
	a := NewAggregator(500)
	assert.Equal(t, 500, a.maxDeltas)
}

func TestAggregator_AddDeltas(t *testing.T) {
	a := NewAggregator(100)
	batch := []QTableDelta{
		{
			StateKey:   QStateKey{CPUBucket: 1, MemBucket: 2, CPUReqMilli: 100, MemReqMilli: 256},
			ActionKey:  QActionKey{CPUAdjPct: 10, MemAdjPct: 0},
			DeltaQ:     0.5,
			VisitCount: 10,
		},
		{
			StateKey:   QStateKey{CPUBucket: 1, MemBucket: 2, CPUReqMilli: 100, MemReqMilli: 256},
			ActionKey:  QActionKey{CPUAdjPct: -10, MemAdjPct: 0},
			DeltaQ:     -0.3,
			VisitCount: 5,
		},
	}
	a.AddDeltas(batch)
	assert.Equal(t, 2, a.PendingDeltaCount())
}

func TestAggregator_AddDeltas_TrimsOverflow(t *testing.T) {
	a := NewAggregator(3)
	deltas := make([]QTableDelta, 5)
	for i := 0; i < 5; i++ {
		deltas[i] = QTableDelta{
			StateKey:   QStateKey{CPUBucket: int64(i)},
			ActionKey:  QActionKey{CPUAdjPct: 10},
			DeltaQ:     float64(i),
			VisitCount: 1,
		}
	}
	a.AddDeltas(deltas)
	assert.Equal(t, 3, a.PendingDeltaCount())
}

func TestAggregator_CompactAndSnapshot_EmptyDeltasReturnsExisting(t *testing.T) {
	a := NewAggregator(100)
	a.global.Entries[EntryKey(QStateKey{CPUBucket: 1}, QActionKey{CPUAdjPct: 10})] = GlobalQEntry{
		QValue:     0.8,
		VisitCount: 100,
	}
	snap := a.CompactAndSnapshot()
	assert.NotNil(t, snap)
	assert.Equal(t, 1, len(snap.Entries))
	assert.InDelta(t, 0.8, snap.Entries[EntryKey(QStateKey{CPUBucket: 1}, QActionKey{CPUAdjPct: 10})].QValue, 1e-9)
}

func TestAggregator_CompactAndSnapshot_SingleCluster(t *testing.T) {
	a := NewAggregator(100)
	deltas := []QTableDelta{
		{
			StateKey:   QStateKey{CPUBucket: 5, MemBucket: 3, CPUReqMilli: 500, MemReqMilli: 1024},
			ActionKey:  QActionKey{CPUAdjPct: 20, MemAdjPct: 10},
			DeltaQ:     0.6,
			VisitCount: 20,
		},
		{
			StateKey:   QStateKey{CPUBucket: 5, MemBucket: 3, CPUReqMilli: 500, MemReqMilli: 1024},
			ActionKey:  QActionKey{CPUAdjPct: 20, MemAdjPct: 10},
			DeltaQ:     0.8,
			VisitCount: 10,
		},
	}
	a.AddDeltas(deltas)
	snap := a.CompactAndSnapshot()

	k := EntryKey(
		QStateKey{CPUBucket: 5, MemBucket: 3, CPUReqMilli: 500, MemReqMilli: 1024},
		QActionKey{CPUAdjPct: 20, MemAdjPct: 10},
	)
	assert.Equal(t, 1, len(snap.Entries))
	entry := snap.Entries[k]
	expectedAvg := (0.6*20 + 0.8*10) / 30.0
	assert.InDelta(t, expectedAvg, entry.QValue, 1e-9)
	assert.Equal(t, uint64(30), entry.VisitCount)
	assert.Equal(t, 0, a.PendingDeltaCount())
	assert.Equal(t, int64(1), snap.Version)
}

func TestAggregator_CompactAndSnapshot_MergesWithExistingGlobal(t *testing.T) {
	a := NewAggregator(100)
	sk := QStateKey{CPUBucket: 1, MemBucket: 1, CPUReqMilli: 200, MemReqMilli: 512}
	ak := QActionKey{CPUAdjPct: 0, MemAdjPct: 0}
	k := EntryKey(sk, ak)
	a.global.Entries[k] = GlobalQEntry{
		QValue:     0.5,
		VisitCount: 100,
	}
	a.AddDeltas([]QTableDelta{{
		StateKey:   sk,
		ActionKey:  ak,
		DeltaQ:     0.9,
		VisitCount: 50,
	}})
	snap := a.CompactAndSnapshot()

	entry := snap.Entries[k]
	expectedMixed := (0.5*100 + 0.9*50) / 150.0
	assert.InDelta(t, expectedMixed, entry.QValue, 1e-9)
	assert.Equal(t, uint64(150), entry.VisitCount)
}

func TestAggregator_CompactAndSnapshot_TreatsZeroVisitAsOne(t *testing.T) {
	a := NewAggregator(100)
	sk := QStateKey{CPUBucket: 2}
	ak := QActionKey{CPUAdjPct: 10}
	k := EntryKey(sk, ak)
	a.AddDeltas([]QTableDelta{{
		StateKey:   sk,
		ActionKey:  ak,
		DeltaQ:     0.7,
		VisitCount: 0,
	}})
	snap := a.CompactAndSnapshot()
	assert.Equal(t, 1, len(snap.Entries))
	assert.InDelta(t, 0.7, snap.Entries[k].QValue, 1e-9)
	assert.Equal(t, uint64(1), snap.Entries[k].VisitCount)
}

func TestAggregator_CurrentSnapshot_Isolation(t *testing.T) {
	a := NewAggregator(100)
	sk := QStateKey{CPUBucket: 7}
	ak := QActionKey{CPUAdjPct: -20}
	k := EntryKey(sk, ak)
	a.global.Entries[k] = GlobalQEntry{QValue: 0.42, VisitCount: 7}
	snap1 := a.CurrentSnapshot()
	snap2 := a.CurrentSnapshot()
	snap1.Entries[k] = GlobalQEntry{QValue: 999.0, VisitCount: 1}
	assert.InDelta(t, 0.42, snap2.Entries[k].QValue, 1e-9, "snapshots must be independent copies")
	assert.Equal(t, uint64(7), a.global.Entries[k].VisitCount, "original must not be mutated by snapshot copy edits")
}

func TestAggregator_LoadSnapshot(t *testing.T) {
	a := NewAggregator(100)
	sk := QStateKey{CPUBucket: 9, MemBucket: 4}
	ak := QActionKey{CPUAdjPct: 10, MemAdjPct: -10}
	k := EntryKey(sk, ak)
	snap := &GlobalQTable{
		Version:   42,
		CreatedAt: time.Now(),
		Entries: map[string]GlobalQEntry{
			k: {QValue: 0.95, VisitCount: 500},
		},
	}
	a.LoadSnapshot(snap)
	assert.Equal(t, int64(42), a.global.Version)
	assert.Equal(t, uint64(500), a.global.Entries[k].VisitCount)
	assert.InDelta(t, 0.95, a.global.Entries[k].QValue, 1e-9)

	a.LoadSnapshot(nil)
	assert.Equal(t, int64(42), a.global.Version, "nil snapshot should be no-op")
}

func TestAggregator_LoadSnapshot_Isolation(t *testing.T) {
	a := NewAggregator(100)
	entries := make(map[string]GlobalQEntry)
	k := EntryKey(QStateKey{CPUBucket: 1}, QActionKey{CPUAdjPct: 5})
	entries[k] = GlobalQEntry{QValue: 0.1, VisitCount: 1}
	snap := &GlobalQTable{Version: 1, Entries: entries}
	a.LoadSnapshot(snap)
	entries[k] = GlobalQEntry{QValue: 999, VisitCount: 999}
	assert.InDelta(t, 0.1, a.global.Entries[k].QValue, 1e-9, "load should make a copy of entries")
}

func TestAggregator_VersionIncrementsEachCompact(t *testing.T) {
	a := NewAggregator(100)
	a.AddDeltas([]QTableDelta{{
		StateKey:   QStateKey{CPUBucket: 1},
		ActionKey:  QActionKey{CPUAdjPct: 0},
		DeltaQ:     0.1,
		VisitCount: 1,
	}})
	s1 := a.CompactAndSnapshot()
	assert.Equal(t, int64(1), s1.Version)
	a.AddDeltas([]QTableDelta{{
		StateKey:   QStateKey{CPUBucket: 2},
		ActionKey:  QActionKey{CPUAdjPct: 10},
		DeltaQ:     0.2,
		VisitCount: 1,
	}})
	s2 := a.CompactAndSnapshot()
	assert.Equal(t, int64(2), s2.Version)
}
