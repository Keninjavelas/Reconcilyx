package rl

import (
	"testing"

	"github.com/reconcilyx/operator/internal/federated"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestDetermineJVMBucket_NonJVMWorkload(t *testing.T) {
	b := DetermineJVMBucket(4096, false)
	assert.Equal(t, JVMBucketNone, b)
}

func TestDetermineJVMBucket_Boundaries(t *testing.T) {
	tests := []struct {
		name     string
		heapMB   int64
		expected int64
	}{
		{"zero", 0, JVMBucketSmall},
		{"small_low", 1, JVMBucketSmall},
		{"small_high", 511, JVMBucketSmall},
		{"medium_low", 512, JVMBucketMedium},
		{"medium_high", 2047, JVMBucketMedium},
		{"large_low", 2048, JVMBucketLarge},
		{"large_huge", 65536, JVMBucketLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := DetermineJVMBucket(tt.heapMB, true)
			assert.Equal(t, tt.expected, b)
		})
	}
}

func TestDetermineGPUBucket_NoGPU(t *testing.T) {
	b := DetermineGPUBucket(99.9, false)
	assert.Equal(t, GPUBucketNone, b)
}

func TestDetermineGPUBucket_Boundaries(t *testing.T) {
	tests := []struct {
		name     string
		utilPct  float64
		expected int64
	}{
		{"idle_zero", 0.0, GPUBucketIdle},
		{"idle_low", 1.5, GPUBucketIdle},
		{"idle_high", 19.99, GPUBucketIdle},
		{"medium_low", 20.0, GPUBucketMedium},
		{"medium_mid", 50.0, GPUBucketMedium},
		{"medium_high", 69.99, GPUBucketMedium},
		{"high_low", 70.0, GPUBucketHigh},
		{"high_max", 100.0, GPUBucketHigh},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := DetermineGPUBucket(tt.utilPct, true)
			assert.Equal(t, tt.expected, b)
		})
	}
}

func TestStateKeyStruct_IncludesJVMBucket(t *testing.T) {
	state := &WorkloadState{
		CPUUsageCores:         0.5,
		MemoryWorkingSetBytes: 256 * 1024 * 1024,
		CurrentCPURequest:     *resource.NewMilliQuantity(1000, resource.DecimalSI),
		CurrentMemoryRequest:  *resource.NewMilliQuantity(1024, resource.BinarySI),
		JVMBucket:             JVMBucketMedium,
		GPUBucket:             GPUBucketNone,
	}
	sk := stateKeyStruct(state)
	assert.Equal(t, int64(JVMBucketMedium), sk.JVMBucket)
	assert.Equal(t, int64(GPUBucketNone), sk.GPUBucket)
}

func TestStateKeyStruct_IncludesGPUBucket(t *testing.T) {
	state := &WorkloadState{
		CPUUsageCores:         0.5,
		MemoryWorkingSetBytes: 256 * 1024 * 1024,
		CurrentCPURequest:     *resource.NewMilliQuantity(1000, resource.DecimalSI),
		CurrentMemoryRequest:  *resource.NewMilliQuantity(1024, resource.BinarySI),
		JVMBucket:             JVMBucketNone,
		GPUBucket:             GPUBucketHigh,
	}
	sk := stateKeyStruct(state)
	assert.Equal(t, int64(JVMBucketNone), sk.JVMBucket)
	assert.Equal(t, int64(GPUBucketHigh), sk.GPUBucket)
}

func TestStateKeyToFederated_Roundtrip(t *testing.T) {
	sk := qStateKey{
		CPUBucket:   5,
		MemBucket:   3,
		CPUReqMilli: 500,
		MemReqMilli: 1073741824,
		JVMBucket:   JVMBucketLarge,
		GPUBucket:   GPUBucketMedium,
	}
	fed := stateKeyToFederated(sk)
	assert.Equal(t, int64(5), fed.CPUBucket)
	assert.Equal(t, int64(3), fed.MemBucket)
	assert.Equal(t, int64(500), fed.CPUReqMilli)
	assert.Equal(t, int64(1073741824), fed.MemReqMilli)
	assert.Equal(t, int64(JVMBucketLarge), fed.JVMBucket)
	assert.Equal(t, int64(GPUBucketMedium), fed.GPUBucket)

	ak := qActionKey{CPUAdjPct: -10, MemAdjPct: 10}
	fedAK := actionKeyToFederated(ak)
	entryKeyStr := federated.EntryKey(fed, fedAK)

	parsed, ok := parseEntryKey(entryKeyStr)
	assert.True(t, ok, "entry key should parse: %s", entryKeyStr)
	assert.Equal(t, sk.CPUBucket, parsed.S.CPUBucket)
	assert.Equal(t, sk.MemBucket, parsed.S.MemBucket)
	assert.Equal(t, sk.CPUReqMilli, parsed.S.CPUReqMilli)
	assert.Equal(t, sk.MemReqMilli, parsed.S.MemReqMilli)
	assert.Equal(t, sk.JVMBucket, parsed.S.JVMBucket)
	assert.Equal(t, sk.GPUBucket, parsed.S.GPUBucket)
	assert.Equal(t, ak.CPUAdjPct, parsed.A.CPUAdjPct)
	assert.Equal(t, ak.MemAdjPct, parsed.A.MemAdjPct)
}
