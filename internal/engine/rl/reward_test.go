package rl_test

import (
	"testing"

	"github.com/reconcilyx/operator/internal/engine/rl"
	"github.com/stretchr/testify/assert"
)

func TestWeightedReward_CarbonWeightZeroMatchesLegacyCostOnly(t *testing.T) {
	reward := rl.WeightedReward(100.0, 0.0, 0.0)
	// $100/month at denominator 50 = sigmoid(2) ≈ 0.8808
	assert.InDelta(t, 0.8808, reward, 0.01)
}

func TestWeightedReward_CarbonWeightOneIsCarbonOnly(t *testing.T) {
	reward := rl.WeightedReward(0.0, 100_000.0, 1.0)
	// 100 kg CO2 = 100000 g / 50000 = sigmoid(2) ≈ 0.8808
	assert.InDelta(t, 0.8808, reward, 0.01)
}

func TestWeightedReward_FiftyFiftyIsMidpoint(t *testing.T) {
	rewardCostOnly := rl.WeightedReward(100.0, 0.0, 0.0)
	rewardCarbonOnly := rl.WeightedReward(0.0, 100_000.0, 1.0)
	mixed := rl.WeightedReward(100.0, 100_000.0, 0.5)
	expectedMid := (rewardCostOnly + rewardCarbonOnly) / 2
	assert.InDelta(t, expectedMid, mixed, 1e-9, "50/50 weights should give arithmetic midpoint of normalized components")
}

func TestWeightedReward_NegativeSavingsProduceLowRewards(t *testing.T) {
	reward := rl.WeightedReward(-200.0, -500_000.0, 0.5)
	assert.Less(t, reward, 0.2, "heavy cost+carbon increase should yield very low reward")
}

func TestWeightedReward_ClampsCarbonWeightToZeroAndOne(t *testing.T) {
	rBelow := rl.WeightedReward(50, 10_000, -5.0)
	rZero := rl.WeightedReward(50, 10_000, 0.0)
	rAbove := rl.WeightedReward(50, 10_000, 10.0)
	rOne := rl.WeightedReward(50, 10_000, 1.0)
	assert.Equal(t, rZero, rBelow, "negative carbon weight must clamp to 0")
	assert.Equal(t, rOne, rAbove, "carbon weight >1 must clamp to 1")
}

func TestWeightedReward_ZeroSavingsBothAxes(t *testing.T) {
	reward := rl.WeightedReward(0.0, 0.0, 0.3)
	// sigmoid(0) = 0.5 on both axes regardless of mix
	assert.InDelta(t, 0.5, reward, 1e-9)
}
