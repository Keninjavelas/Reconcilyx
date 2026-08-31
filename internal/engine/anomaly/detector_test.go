package anomaly

import (
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestZScoreDetector(t *testing.T) {
	t.Run("Detects Spike", func(t *testing.T) {
		detector := NewZScoreDetector(3.0)
		historical := []float64{1.0, 1.1, 1.2, 1.0, 1.1}
		result, err := detector.Detect(historical, 5.0)
		assert.NoError(t, err)
		assert.Equal(t, AnomalySpike, result.Type)
		assert.Greater(t, result.Severity, 0.0)
	})

	t.Run("Detects Drop", func(t *testing.T) {
		detector := NewZScoreDetector(3.0)
		historical := []float64{5.0, 5.1, 5.2, 5.0, 5.1}
		result, err := detector.Detect(historical, 1.0)
		assert.NoError(t, err)
		assert.Equal(t, AnomalyDrop, result.Type)
		assert.Greater(t, result.Severity, 0.0)
	})

	t.Run("No Anomaly", func(t *testing.T) {
		detector := NewZScoreDetector(3.0)
		historical := []float64{1.0, 1.1, 1.2, 1.0, 1.1}
		result, err := detector.Detect(historical, 1.2)
		assert.NoError(t, err)
		assert.Equal(t, AnomalyNone, result.Type)
		assert.Equal(t, 0.0, result.Severity)
	})

	t.Run("Not Enough Data", func(t *testing.T) {
		detector := NewZScoreDetector(3.0)
		historical := []float64{1.0, 1.1}
		result, err := detector.Detect(historical, 5.0)
		assert.NoError(t, err)
		assert.Equal(t, AnomalyNone, result.Type)
	})
}
