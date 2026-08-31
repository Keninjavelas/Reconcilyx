package rl

import (
	"context"
	"math"
	"math/rand"

	"github.com/reconcilyx/operator/internal/federated"
	corev1 "k8s.io/api/core/v1"
)

type Agent interface {
	Recommend(ctx context.Context, state *WorkloadState) (*RecommendationResult, error)
	Update(ctx context.Context, reward float64, action *Action, previousState *WorkloadState, nextState *WorkloadState) error
	LoadGlobal(ctx context.Context, snap *federated.GlobalQTable)
	SetFederatedEmitter(e *federated.Emitter)
}

type qStateKey struct {
	CPUBucket   int64
	MemBucket   int64
	CPUReqMilli int64
	MemReqMilli int64
	JVMBucket   int64
	GPUBucket   int64
}

func stateKeyToFederated(k qStateKey) federated.QStateKey {
	return federated.QStateKey{
		CPUBucket:   k.CPUBucket,
		MemBucket:   k.MemBucket,
		CPUReqMilli: k.CPUReqMilli,
		MemReqMilli: k.MemReqMilli,
		JVMBucket:   k.JVMBucket,
		GPUBucket:   k.GPUBucket,
	}
}

type qActionKey struct {
	CPUAdjPct int
	MemAdjPct int
}

func actionKeyToFederated(k qActionKey) federated.QActionKey {
	return federated.QActionKey{
		CPUAdjPct: k.CPUAdjPct,
		MemAdjPct: k.MemAdjPct,
	}
}

type qKey struct {
	S qStateKey
	A qActionKey
}

type SimpleQAgent struct {
	qTable     map[qKey]float64
	visitCount map[qKey]uint64
	alpha      float64
	gamma      float64
	epsilon    float64
	minEpsilon float64
	decayRate  float64
	emitter    *federated.Emitter
	epoch      int64
}

func NewSimpleQAgent() *SimpleQAgent {
	return &SimpleQAgent{
		qTable:     make(map[qKey]float64),
		visitCount: make(map[qKey]uint64),
		alpha:      0.1,
		gamma:      0.95,
		epsilon:    0.1,
		minEpsilon: 0.01,
		decayRate:  0.995,
	}
}

func stateKeyStruct(state *WorkloadState) qStateKey {
	cpuBucket := int64(math.Floor(state.CPUUsageCores / 0.1))
	memBucket := int64(math.Floor(state.MemoryWorkingSetBytes / float64(128*1024*1024)))
	cpuReqBucket := (state.CurrentCPURequest.MilliValue() / 100) * 100
	memReqBucket := (state.CurrentMemoryRequest.Value() / int64(128*1024*1024)) * int64(128*1024*1024)
	return qStateKey{
		CPUBucket:   cpuBucket,
		MemBucket:   memBucket,
		CPUReqMilli: cpuReqBucket,
		MemReqMilli: memReqBucket,
		JVMBucket:   state.JVMBucket,
		GPUBucket:   state.GPUBucket,
	}
}

func (a *SimpleQAgent) getPossibleActions() []*Action {
	adjustments := []float64{-20, -10, 0, 10, 20}
	actions := make([]*Action, 0, len(adjustments)*len(adjustments))
	for _, cpuAdj := range adjustments {
		for _, memAdj := range adjustments {
			actions = append(actions, &Action{
				Adjustment: map[string]ResourceAdjustment{
					string(corev1.ResourceCPU): {
						ResourceName: corev1.ResourceCPU,
						Percentage:   cpuAdj,
					},
					string(corev1.ResourceMemory): {
						ResourceName: corev1.ResourceMemory,
						Percentage:   memAdj,
					},
				},
			})
		}
	}
	return actions
}

func actionKeyStruct(action *Action) qActionKey {
	cpuAdj := action.Adjustment[string(corev1.ResourceCPU)].Percentage
	memAdj := action.Adjustment[string(corev1.ResourceMemory)].Percentage
	return qActionKey{CPUAdjPct: int(cpuAdj), MemAdjPct: int(memAdj)}
}

func (a *SimpleQAgent) Recommend(ctx context.Context, state *WorkloadState) (*RecommendationResult, error) {
	sk := stateKeyStruct(state)
	possibleActions := a.getPossibleActions()
	if rand.Float64() < a.epsilon {
		randIndex := rand.Intn(len(possibleActions))
		return &RecommendationResult{
			Action:     possibleActions[randIndex],
			Confidence: 0.3 + (1-a.epsilon)*0.7,
		}, nil
	}
	bestAction := possibleActions[0]
	bestQ := -math.MaxFloat64
	for _, action := range possibleActions {
		key := qKey{S: sk, A: actionKeyStruct(action)}
		q := a.qTable[key]
		if q > bestQ {
			bestQ = q
			bestAction = action
		}
	}
	return &RecommendationResult{
		Action:     bestAction,
		Confidence: 0.7 + a.epsilon*0.3,
	}, nil
}

func (a *SimpleQAgent) Update(ctx context.Context, reward float64, action *Action, previousState *WorkloadState, nextState *WorkloadState) error {
	prevSK := stateKeyStruct(previousState)
	nextSK := stateKeyStruct(nextState)
	ak := actionKeyStruct(action)
	key := qKey{S: prevSK, A: ak}
	oldQ, ok := a.qTable[key]
	if !ok {
		oldQ = 0.0
	}
	possibleActions := a.getPossibleActions()
	maxNextQ := 0.0
	for _, nextAction := range possibleActions {
		nextKey := qKey{S: nextSK, A: actionKeyStruct(nextAction)}
		if q, exists := a.qTable[nextKey]; exists && q > maxNextQ {
			maxNextQ = q
		}
	}
	newQ := oldQ + a.alpha*(reward+a.gamma*maxNextQ-oldQ)
	a.qTable[key] = newQ
	a.visitCount[key]++
	vc := a.visitCount[key]
	delta := newQ - oldQ
	if !isFinite(delta) {
		delta = 0.0
	}
	if a.emitter != nil {
		a.epoch++
		a.emitter.Emit(federated.QTableDelta{
			StateKey:   stateKeyToFederated(prevSK),
			ActionKey:  actionKeyToFederated(ak),
			DeltaQ:     delta,
			VisitCount: vc,
			Epoch:      a.epoch,
		})
	}
	if a.epsilon > a.minEpsilon {
		a.epsilon *= a.decayRate
	}
	return nil
}

func (a *SimpleQAgent) SetFederatedEmitter(e *federated.Emitter) { a.emitter = e }

func (a *SimpleQAgent) LoadGlobal(_ context.Context, snap *federated.GlobalQTable) {
	if snap == nil || len(snap.Entries) == 0 {
		return
	}
	for fedKey, entry := range snap.Entries {
		k, ok := parseEntryKey(fedKey)
		if !ok {
			continue
		}
		_, exists := a.qTable[k]
		if exists {
			total := a.visitCount[k] + entry.VisitCount
			if total == 0 {
				continue
			}
			mixed := (a.qTable[k]*float64(a.visitCount[k]) + entry.QValue*float64(entry.VisitCount)) / float64(total)
			a.qTable[k] = mixed
			a.visitCount[k] = total
			continue
		}
		a.qTable[k] = entry.QValue
		if entry.VisitCount > 0 {
			a.visitCount[k] = entry.VisitCount
		} else {
			a.visitCount[k] = 1
		}
	}
}

func parseEntryKey(s string) (qKey, bool) {
	out := qKey{}
	fields := splitN(s, "|", 8)
	n := len(fields)
	if n != 6 && n != 8 {
		return out, false
	}
	cpuBucket, ok := parseInt64(fields[0])
	if !ok {
		return out, false
	}
	memBucket, ok := parseInt64(fields[1])
	if !ok {
		return out, false
	}
	cpuReq, ok := parseInt64(fields[2])
	if !ok {
		return out, false
	}
	memReq, ok := parseInt64(fields[3])
	if !ok {
		return out, false
	}
	var jvmBucket, gpuBucket int64
	var cpuAdjIdx, memAdjIdx int
	if n == 8 {
		jvmBucket, ok = parseInt64(fields[4])
		if !ok {
			return out, false
		}
		gpuBucket, ok = parseInt64(fields[5])
		if !ok {
			return out, false
		}
		cpuAdjIdx = 6
		memAdjIdx = 7
	} else {
		cpuAdjIdx = 4
		memAdjIdx = 5
	}
	cpuAdj, ok := parseInt(fields[cpuAdjIdx])
	if !ok {
		return out, false
	}
	memAdj, ok := parseInt(fields[memAdjIdx])
	if !ok {
		return out, false
	}
	out.S.CPUBucket = cpuBucket
	out.S.MemBucket = memBucket
	out.S.CPUReqMilli = cpuReq
	out.S.MemReqMilli = memReq
	out.S.JVMBucket = jvmBucket
	out.S.GPUBucket = gpuBucket
	out.A.CPUAdjPct = cpuAdj
	out.A.MemAdjPct = memAdj
	return out, true
}

func isFinite(v float64) bool { return v == v && v-1 != v && v+1 != v || v == 0 }

func splitN(s, sep string, n int) []string {
	if n <= 0 {
		return nil
	}
	if n == 1 {
		return []string{s}
	}
	out := make([]string, 0, n)
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep[0] {
			out = append(out, s[start:i])
			start = i + 1
			if len(out) == n-1 {
				break
			}
		}
	}
	out = append(out, s[start:])
	return out
}

func parseInt64(s string) (int64, bool) {
	if s == "" {
		return 0, false
	}
	neg := false
	if s[0] == '-' {
		neg = true
		s = s[1:]
		if s == "" {
			return 0, false
		}
	}
	var v int64
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		v = v*10 + int64(c-'0')
	}
	if neg {
		v = -v
	}
	return v, true
}

func parseInt(s string) (int, bool) {
	v, ok := parseInt64(s)
	return int(v), ok
}

func NewAgent() Agent {
	return NewSimpleQAgent()
}
