package engine

import (
	"testing"

	"github.com/reconcilyx/operator/internal/telemetry"
	"github.com/stretchr/testify/assert"
)

func TestScorer_Score(t *testing.T) {
	scorer := NewScorer()

	t.Run("Perfect Score", func(t *testing.T) {
		metrics := &telemetry.WorkloadMetrics{
			Containers: map[string]*telemetry.ContainerMetrics{
				"app": {
					Restarts: 0,
					CPU: telemetry.CPUMetrics{ThrottledSecs: 0},
					Memory: telemetry.MemoryMetrics{OOMCount: 0},
				},
			},
		}
		c, r := scorer.Score(metrics, false)
		assert.Equal(t, int32(100), c)
		assert.Equal(t, int32(0), r)
	})

	t.Run("Partial Data Penalty", func(t *testing.T) {
		metrics := &telemetry.WorkloadMetrics{}
		c, r := scorer.Score(metrics, true)
		assert.Equal(t, int32(50), c)
		assert.Equal(t, int32(0), r)
	})

	t.Run("High Risk", func(t *testing.T) {
		metrics := &telemetry.WorkloadMetrics{
			Containers: map[string]*telemetry.ContainerMetrics{
				"app": {
					Restarts: 5,
					CPU: telemetry.CPUMetrics{ThrottledSecs: 100},
					Memory: telemetry.MemoryMetrics{OOMCount: 2},
				},
			},
		}
		c, r := scorer.Score(metrics, false)
		assert.Equal(t, int32(80), c) // 100 - 20 (restarts)
		assert.Equal(t, int32(100), r) // 50 (throttled) + 80 (OOM) = 130 -> clamped to 100
	})
}
