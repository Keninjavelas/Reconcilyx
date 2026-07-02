package forecasting

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// Prediction represents a forecasted resource requirement over a specific time window.
type Prediction struct {
	// The forecasted maximum CPU usage.
	MaxCPU resource.Quantity
	// The forecasted maximum Memory usage.
	MaxMemory resource.Quantity
	// The time window for which this prediction is valid.
	ValidUntil time.Time
	// Confidence score of the prediction (0-100).
	Confidence int32
}

// Predictor provides an interface for hybrid forecasting models (e.g., ML cold-start, historical seasonality).
// It abstracts away the heavy ML implementation, allowing the Analyzer to request predictions seamlessly.
type Predictor interface {
	// Forecast predicts the resource requirements for the given workload.
	// It uses historical metrics and known seasonality markers to generate a prediction.
	Forecast(ctx context.Context, workload corev1.ObjectReference) (*Prediction, error)
}

// defaultPredictor is a stub implementation for the forecasting engine.
type defaultPredictor struct{}

// NewPredictor creates a new hybrid forecasting predictor.
func NewPredictor() Predictor {
	return &defaultPredictor{}
}

func (p *defaultPredictor) Forecast(ctx context.Context, workload corev1.ObjectReference) (*Prediction, error) {
	// TODO: Integrate with Prometheus historical data and seasonality markers.
	// Currently returns a low-confidence empty prediction as a fallback.
	return &Prediction{
		Confidence: 0,
		ValidUntil: time.Now().Add(1 * time.Hour),
	}, nil
}
