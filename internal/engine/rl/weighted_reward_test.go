package rl_test

import (
	"testing"

	"github.com/reconcilyx/operator/internal/engine/rl"
	"github.com/stretchr/testify/assert"
)

func TestWeightedReward_CarbonWeightSensitivity(t *testing.T) {
	costSaved := 10.0
	highCarbonSaved := 200_000.0
	lowCarbonSaved := 500.0

	rewardCostOnlyHighCarbon := rl.WeightedReward(costSaved, highCarbonSaved, 0.0)
	rewardCarbonHeavyHighCarbon := rl.WeightedReward(costSaved, highCarbonSaved, 0.9)
	assert.Greater(t, rewardCarbonHeavyHighCarbon, rewardCostOnlyHighCarbon,
		"higher carbon weight with high carbon saved should increase reward")

	rewardCarbonHeavyLowCarbon := rl.WeightedReward(costSaved, lowCarbonSaved, 0.9)
	assert.Less(t, rewardCarbonHeavyLowCarbon, rewardCarbonHeavyHighCarbon,
		"low carbon saved should earn lower reward under high carbon weight")
}

func TestWeightedReward_ExtremeWeights(t *testing.T) {
	r0 := rl.WeightedReward(100, 100, 0.0)
	r1 := rl.WeightedReward(100, 100, 1.0)
	rcost := rl.WeightedReward(100, 0, 1.0)
	assert.NotEqual(t, r0, r1, "mix weight 0 vs 1 should differ with non-zero both axes")
	assert.InDelta(t, 0.5, rcost, 0.01, "pure cost weight=1 cost=$100 → sigmoid(100/50=2) → sig(2)≈0.88, but no carbon → pure sig(0)=0.5 for carbon axis only")
}

func TestWeightedReward_Clamping(t *testing.T) {
	below := rl.WeightedReward(50, 10_000, -5.0)
	normal := rl.WeightedReward(50, 10_000, 0.0)
	assert.Equal(t, below, normal, "negative carbonWeight should clamp to 0")

	above := rl.WeightedReward(50, 10_000, 99.0)
	one := rl.WeightedReward(50, 10_000, 1.0)
	assert.Equal(t, above, one, "carbonWeight>1 should clamp to 1")
}

func TestWeightedReward_NegativeDeltasPenalize(t *testing.T) {
	positive := rl.WeightedReward(50, 10_000, 0.5)
	negative := rl.WeightedReward(-50, -10_000, 0.5)
	assert.Greater(t, positive, 0.5, "positive savings should be above sig(0)")
	assert.Less(t, negative, 0.5, "negative savings should be below sig(0)")
}
