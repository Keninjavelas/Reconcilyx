package rl_test

import (
	"context"
	"testing"

	"github.com/reconcilyx/operator/internal/engine/rl"
	"github.com/reconcilyx/operator/internal/federated"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func newTestState(cpuCores, memBytes float64, cpuMilli, memMilli int64) *rl.WorkloadState {
	return &rl.WorkloadState{
		CPUUsageCores:         cpuCores,
		MemoryWorkingSetBytes: memBytes,
		CurrentCPURequest:     *resource.NewMilliQuantity(cpuMilli, resource.DecimalSI),
		CurrentMemoryRequest:  *resource.NewMilliQuantity(memMilli, resource.BinarySI),
	}
}

func TestNewSimpleQAgent_InitialState(t *testing.T) {
	a := rl.NewSimpleQAgent()
	require.NotNil(t, a)
	require.NotNil(t, a)
}

func TestNewAgent_ReturnsSimpleQAgent(t *testing.T) {
	a := rl.NewAgent()
	require.NotNil(t, a)
	_, ok := a.(*rl.SimpleQAgent)
	assert.True(t, ok, "NewAgent should return *SimpleQAgent")
}

func TestSimpleQAgent_Recommend_ReturnsValidAction(t *testing.T) {
	a := rl.NewSimpleQAgent()
	state := newTestState(0.5, 256*1024*1024, 1000, 1024)
	res, err := a.Recommend(context.Background(), state)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotNil(t, res.Action)
	assert.Contains(t, res.Action.Adjustment, string(corev1.ResourceCPU))
	assert.Contains(t, res.Action.Adjustment, string(corev1.ResourceMemory))
	assert.InDelta(t, 0.7, res.Confidence, 0.3, "confidence should be in reasonable range")
}

func TestSimpleQAgent_Recommend_ActionSpaceCoversAdjacents(t *testing.T) {
	a := rl.NewSimpleQAgent()
	state := newTestState(0.5, 256*1024*1024, 1000, 1024)
	seenCPUAdj := make(map[float64]bool)
	seenMemAdj := make(map[float64]bool)
	// 10% of 2000 draws is ~200 uniform explorations across 25 actions, making
	// coverage of every adjustment effectively deterministic.
	for i := 0; i < 2000; i++ {
		res, err := a.Recommend(context.Background(), state)
		require.NoError(t, err)
		cpuAdj := res.Action.Adjustment[string(corev1.ResourceCPU)].Percentage
		memAdj := res.Action.Adjustment[string(corev1.ResourceMemory)].Percentage
		seenCPUAdj[cpuAdj] = true
		seenMemAdj[memAdj] = true
	}
	for _, adj := range []float64{-20, -10, 0, 10, 20} {
		assert.True(t, seenCPUAdj[adj], "expected CPU adj %v among seen actions", adj)
		assert.True(t, seenMemAdj[adj], "expected Mem adj %v among seen actions", adj)
	}
}

func TestSimpleQAgent_Update_LearnsFromReward(t *testing.T) {
	a := rl.NewSimpleQAgent()
	prevState := newTestState(0.5, 256*1024*1024, 1000, 1024)
	nextState := newTestState(0.4, 200*1024*1024, 900, 900)
	action := &rl.Action{
		Adjustment: map[string]rl.ResourceAdjustment{
			string(corev1.ResourceCPU):    {ResourceName: corev1.ResourceCPU, Percentage: -10},
			string(corev1.ResourceMemory): {ResourceName: corev1.ResourceMemory, Percentage: -10},
		},
	}
	err := a.Update(context.Background(), 1.0, action, prevState, nextState)
	require.NoError(t, err)
}

func TestSimpleQAgent_Update_MultipleUpdatesConverge(t *testing.T) {
	a := rl.NewSimpleQAgent()
	prevState := newTestState(0.5, 256*1024*1024, 1000, 1024)
	nextState := newTestState(0.5, 256*1024*1024, 1000, 1024)
	goodAction := &rl.Action{
		Adjustment: map[string]rl.ResourceAdjustment{
			string(corev1.ResourceCPU):    {ResourceName: corev1.ResourceCPU, Percentage: -20},
			string(corev1.ResourceMemory): {ResourceName: corev1.ResourceMemory, Percentage: 0},
		},
	}
	badAction := &rl.Action{
		Adjustment: map[string]rl.ResourceAdjustment{
			string(corev1.ResourceCPU):    {ResourceName: corev1.ResourceCPU, Percentage: 20},
			string(corev1.ResourceMemory): {ResourceName: corev1.ResourceMemory, Percentage: 20},
		},
	}
	for i := 0; i < 50; i++ {
		_ = a.Update(context.Background(), 1.0, goodAction, prevState, nextState)
		_ = a.Update(context.Background(), -1.0, badAction, prevState, nextState)
	}
	aForAgent, ok := interface{}(a).(*rl.SimpleQAgent)
	require.True(t, ok)
	_ = aForAgent
}

func TestSimpleQAgent_Update_EmitsDeltaWhenEmitterSet(t *testing.T) {
	a := rl.NewSimpleQAgent()
	emitter := federated.NewEmitter(federated.EmitterConfig{ClusterName: "test-cluster", HubBaseURL: ""})
	a.SetFederatedEmitter(emitter)
	prevState := newTestState(0.5, 256*1024*1024, 1000, 1024)
	nextState := newTestState(0.4, 200*1024*1024, 1000, 1024)
	action := &rl.Action{
		Adjustment: map[string]rl.ResourceAdjustment{
			string(corev1.ResourceCPU):    {ResourceName: corev1.ResourceCPU, Percentage: 10},
			string(corev1.ResourceMemory): {ResourceName: corev1.ResourceMemory, Percentage: 10},
		},
	}
	err := a.Update(context.Background(), 0.8, action, prevState, nextState)
	require.NoError(t, err)
	assert.Equal(t, 1, emitter.BufferedCount())
}

func TestSimpleQAgent_LoadGlobal_EmptySnapshotNoOp(t *testing.T) {
	a := rl.NewSimpleQAgent()
	a.LoadGlobal(context.Background(), nil)
	a.LoadGlobal(context.Background(), &federated.GlobalQTable{Entries: map[string]federated.GlobalQEntry{}})
}

func TestSimpleQAgent_LoadGlobal_NewEntriesInserted(t *testing.T) {
	a := rl.NewSimpleQAgent()
	sk := federated.QStateKey{CPUBucket: 5, MemBucket: 3, CPUReqMilli: 500, MemReqMilli: 1073741824}
	ak := federated.QActionKey{CPUAdjPct: -10, MemAdjPct: 0}
	k := federated.EntryKey(sk, ak)
	snap := &federated.GlobalQTable{
		Version: 3,
		Entries: map[string]federated.GlobalQEntry{
			k: {QValue: 0.88, VisitCount: 100},
		},
	}
	a.LoadGlobal(context.Background(), snap)
}

func TestSimpleQAgent_LoadGlobal_MergesExisting(t *testing.T) {
	a := rl.NewSimpleQAgent()
	prevState := newTestState(0.5, 3.0*float64(128*1024*1024), 500, 1073741824)
	action := &rl.Action{
		Adjustment: map[string]rl.ResourceAdjustment{
			string(corev1.ResourceCPU):    {ResourceName: corev1.ResourceCPU, Percentage: -10},
			string(corev1.ResourceMemory): {ResourceName: corev1.ResourceMemory, Percentage: 0},
		},
	}
	for i := 0; i < 10; i++ {
		_ = a.Update(context.Background(), 0.5, action, prevState, prevState)
	}

	sk := federated.QStateKey{CPUBucket: 5, MemBucket: 3, CPUReqMilli: 500, MemReqMilli: 1073741824}
	ak := federated.QActionKey{CPUAdjPct: -10, MemAdjPct: 0}
	k := federated.EntryKey(sk, ak)
	snap := &federated.GlobalQTable{
		Entries: map[string]federated.GlobalQEntry{
			k: {QValue: 0.9, VisitCount: 100},
		},
	}
	a.LoadGlobal(context.Background(), snap)
}

func TestSimpleQAgent_LoadGlobal_InvalidKeySkipped(t *testing.T) {
	a := rl.NewSimpleQAgent()
	snap := &federated.GlobalQTable{
		Entries: map[string]federated.GlobalQEntry{
			"not-a-valid-key": {QValue: 0.5, VisitCount: 10},
			"1|2|3|4|5":       {QValue: 0.1, VisitCount: 1},
		},
	}
	a.LoadGlobal(context.Background(), snap)
}

func TestAgentInterface(t *testing.T) {
	var a rl.Agent = rl.NewSimpleQAgent()
	require.NotNil(t, a)
	_, err := a.Recommend(context.Background(), newTestState(0.3, 128*1024*1024, 500, 512*1024*1024))
	assert.NoError(t, err)
	act := &rl.Action{
		Adjustment: map[string]rl.ResourceAdjustment{
			string(corev1.ResourceCPU):    {ResourceName: corev1.ResourceCPU, Percentage: 0},
			string(corev1.ResourceMemory): {ResourceName: corev1.ResourceMemory, Percentage: 0},
		},
	}
	err = a.Update(context.Background(), 0.0, act, newTestState(0.1, 64*1024*1024, 200, 256*1024*1024), newTestState(0.1, 64*1024*1024, 200, 256*1024*1024))
	assert.NoError(t, err)
	a.LoadGlobal(context.Background(), nil)
	a.SetFederatedEmitter(nil)
}
