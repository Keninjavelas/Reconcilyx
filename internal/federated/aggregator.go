package federated

import (
	"crypto/sha256"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

type Aggregator struct {
	mu sync.RWMutex

	global      *GlobalQTable
	deltas      []QTableDelta
	maxDeltas   int
	lastCompact time.Time

	trimRatio          float64
	maxDeltaCutoff     float64
	stabilityThreshold float64
}

func NewAggregator(maxBuffer int) *Aggregator {
	if maxBuffer <= 0 {
		maxBuffer = 200000
	}
	return &Aggregator{
		global: &GlobalQTable{
			Version:          0,
			VersionStr:       "v0",
			CreatedAt:        time.Now().UTC(),
			Entries:          make(map[string]GlobalQEntry),
			IsStable:         false,
			ConvergenceScore: 1.0,
			MinSpokeVersion:  "v1.0.0",
		},
		deltas:             make([]QTableDelta, 0, 1024),
		maxDeltas:          maxBuffer,
		trimRatio:          DefaultTrimRatio,
		maxDeltaCutoff:     DefaultMaxDeltaQ,
		stabilityThreshold: DefaultStabilityThreshold,
	}
}

func (a *Aggregator) AddDeltas(batch []QTableDelta) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.deltas = append(a.deltas, batch...)
	if len(a.deltas) > a.maxDeltas {
		trim := len(a.deltas) - a.maxDeltas
		a.deltas = a.deltas[trim:]
	}
}

// CompactAndSnapshot performs Byzantine-resistant Trimmed Mean Aggregation,
// computes model convergence score, and persists a global snapshot.
func (a *Aggregator) CompactAndSnapshot() *GlobalQTable {
	a.mu.Lock()
	defer a.mu.Unlock()

	type deltaSample struct {
		deltaQ     float64
		visitCount uint64
	}

	grouped := make(map[string][]deltaSample, len(a.deltas))
	var rejectedCount uint64

	// Step 1: Filter impossible magnitude deltas (Outlier Rejection)
	for _, d := range a.deltas {
		if math.IsNaN(d.DeltaQ) || math.IsInf(d.DeltaQ, 0) || math.Abs(d.DeltaQ) > a.maxDeltaCutoff {
			rejectedCount++
			continue
		}
		k := EntryKey(d.StateKey, d.ActionKey)
		vc := d.VisitCount
		if vc == 0 {
			vc = 1
		}
		grouped[k] = append(grouped[k], deltaSample{
			deltaQ:     d.DeltaQ,
			visitCount: vc,
		})
	}

	a.global.OutlierRejectedCount += rejectedCount

	if len(grouped) == 0 && len(a.global.Entries) > 0 {
		out := *a.global
		out.CreatedAt = time.Now().UTC()
		return &out
	}

	var maxShift float64

	// Step 2: Byzantine-Resistant Trimmed Mean per key
	for k, samples := range grouped {
		if len(samples) == 0 {
			continue
		}

		var avgQ float64
		var totalVisits uint64

		if len(samples) >= 3 && a.trimRatio > 0 {
			// Sort samples by DeltaQ for trimming
			sort.Slice(samples, func(i, j int) bool {
				return samples[i].deltaQ < samples[j].deltaQ
			})

			trimCount := int(math.Floor(float64(len(samples)) * a.trimRatio))
			if trimCount*2 < len(samples) {
				samples = samples[trimCount : len(samples)-trimCount]
			}
		}

		var weightedSum float64
		for _, s := range samples {
			weightedSum += s.deltaQ * float64(s.visitCount)
			totalVisits += s.visitCount
		}

		if totalVisits > 0 {
			avgQ = weightedSum / float64(totalVisits)
		}

		existing, ok := a.global.Entries[k]
		var newQ float64
		var newTotal uint64

		if !ok {
			newQ = avgQ
			newTotal = totalVisits
		} else {
			newTotal = existing.VisitCount + totalVisits
			if newTotal > 0 {
				newQ = (existing.QValue*float64(existing.VisitCount) + avgQ*float64(totalVisits)) / float64(newTotal)
			} else {
				newQ = existing.QValue
			}
		}

		// Compute entry shift magnitude for convergence tracking
		if ok {
			shift := math.Abs(newQ - existing.QValue)
			if shift > maxShift {
				maxShift = shift
			}
		} else {
			shift := math.Abs(newQ)
			if shift > maxShift {
				maxShift = shift
			}
		}

		a.global.Entries[k] = GlobalQEntry{
			QValue:     newQ,
			VisitCount: newTotal,
		}
	}

	a.deltas = a.deltas[:0]
	a.global.Version++
	a.global.VersionStr = fmt.Sprintf("v%d", a.global.Version)
	a.global.CreatedAt = time.Now().UTC()
	a.lastCompact = a.global.CreatedAt

	// Step 3: Compute Model Convergence & Stability
	a.global.ConvergenceScore = maxShift
	if maxShift < a.stabilityThreshold && len(a.global.Entries) > 0 {
		a.global.IsStable = true
	} else {
		a.global.IsStable = false
	}

	// Step 4: Compute Model Hash
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%d:%d:%f", a.global.Version, len(a.global.Entries), maxShift)))
	a.global.ModelHash = fmt.Sprintf("%x", h.Sum(nil))[:16]

	out := *a.global
	out.Entries = make(map[string]GlobalQEntry, len(a.global.Entries))
	for k, v := range a.global.Entries {
		out.Entries[k] = v
	}
	return &out
}

func (a *Aggregator) CurrentSnapshot() *GlobalQTable {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := *a.global
	out.CreatedAt = time.Now().UTC()
	out.Entries = make(map[string]GlobalQEntry, len(a.global.Entries))
	for k, v := range a.global.Entries {
		out.Entries[k] = v
	}
	return &out
}

func (a *Aggregator) LoadSnapshot(snap *GlobalQTable) {
	if snap == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	entries := make(map[string]GlobalQEntry, len(snap.Entries))
	for k, v := range snap.Entries {
		entries[k] = v
	}
	a.global = &GlobalQTable{
		Version:              snap.Version,
		VersionStr:           snap.VersionStr,
		CreatedAt:            time.Now().UTC(),
		Entries:              entries,
		IsStable:             snap.IsStable,
		ConvergenceScore:     snap.ConvergenceScore,
		ModelHash:            snap.ModelHash,
		MinSpokeVersion:      snap.MinSpokeVersion,
		OutlierRejectedCount: snap.OutlierRejectedCount,
	}
}

func (a *Aggregator) PendingDeltaCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.deltas)
}
