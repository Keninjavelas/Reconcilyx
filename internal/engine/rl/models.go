package rl

import (
	"math"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	JVMBucketNone   int64 = 0
	JVMBucketSmall  int64 = 1
	JVMBucketMedium int64 = 2
	JVMBucketLarge  int64 = 3

	GPUBucketNone   int64 = 0
	GPUBucketIdle   int64 = 1
	GPUBucketMedium int64 = 2
	GPUBucketHigh   int64 = 3
)

type WorkloadState struct {
	CPUUsageCores         float64
	CPUThrottledSecs      float64
	MemoryWorkingSetBytes float64
	OOMCount              int32
	CurrentCPURequest     resource.Quantity
	CurrentMemoryRequest  resource.Quantity
	Region string
	CarbonWeight *float64
	JVMBucket    int64
	GPUBucket    int64
}

func DetermineJVMBucket(heapMB int64, isJVMWorkload bool) int64 {
	if !isJVMWorkload {
		return JVMBucketNone
	}
	if heapMB < 512 {
		return JVMBucketSmall
	}
	if heapMB < 2048 {
		return JVMBucketMedium
	}
	return JVMBucketLarge
}

func DetermineGPUBucket(gpuUtilPct float64, hasGPU bool) int64 {
	if !hasGPU {
		return GPUBucketNone
	}
	if gpuUtilPct < 20 {
		return GPUBucketIdle
	}
	if gpuUtilPct < 70 {
		return GPUBucketMedium
	}
	return GPUBucketHigh
}

// Action represents a resource adjustment action recommended by the RL agent.
type Action struct {
	Adjustment map[string]ResourceAdjustment
}

// ResourceAdjustment represents the adjustment for a specific resource type.
type ResourceAdjustment struct {
	ResourceName corev1.ResourceName
	Percentage   float64
	NewRequest   resource.Quantity
}

// RecommendationResult holds the recommended action and confidence score.
type RecommendationResult struct {
	Action     *Action
	Confidence float64
}

// sigmoid01 maps any real value into approximately (0, 1).
func sigmoid01(x float64) float64 {
	return 1.0 / (1.0 + math.Exp(-x))
}

// WeightedReward combines cost and carbon savings into a single reward signal.
//
// Both inputs are raw deltas where positive values = savings (good).
// The function normalizes each via a scaled logistic, then mixes with
// carbonWeight ∈ [0.0, 1.0]. carbonWeight=0 → cost-only (legacy behaviour).
func WeightedReward(costSavedUSDPerMonth float64, carbonSavedGramsPerMonth float64, carbonWeight float64) float64 {
	if carbonWeight < 0 {
		carbonWeight = 0
	}
	if carbonWeight > 1.0 {
		carbonWeight = 1.0
	}
	// Scales chosen so that $100/month saved ≈ reward 0.73 (near-asymptotic),
	// and 100 kg CO2/month saved ≈ reward 0.73 on the carbon axis.
	costNorm := sigmoid01(costSavedUSDPerMonth / 50.0)
	carbonNorm := sigmoid01(carbonSavedGramsPerMonth / 50_000.0)
	return (1.0-carbonWeight)*costNorm + carbonWeight*carbonNorm
}

