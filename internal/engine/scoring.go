package engine

import (
	"github.com/reconcilyx/operator/internal/telemetry"
)

type defaultScorer struct{}

func NewScorer() Scorer {
	return &defaultScorer{}
}

func (s *defaultScorer) Score(metrics *telemetry.WorkloadMetrics, isPartial bool) (int32, int32) {
	confidence := int32(100)
	risk := int32(0)

	if isPartial {
		confidence -= 50
	}

	for _, container := range metrics.Containers {
		if container.Restarts > 0 {
			confidence -= 20
		}
		if container.CPU.ThrottledSecs > 0 {
			risk += 50
		}
		if container.Memory.OOMCount > 0 {
			risk += 80
		}
	}

	// Clamp limits
	if confidence < 0 {
		confidence = 0
	}
	if risk > 100 {
		risk = 100
	}

	return confidence, risk
}
