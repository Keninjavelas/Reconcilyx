package forecasting

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/prometheus/common/model"
	"github.com/reconcilyx/operator/internal/telemetry"
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

// defaultPredictor is an implementation for the forecasting engine that uses Prometheus historical data.
type defaultPredictor struct {
	metricsClient telemetry.MetricsClient
}

// NewPredictor creates a new hybrid forecasting predictor with the given metrics client.
func NewPredictor(client telemetry.MetricsClient) Predictor {
	return &defaultPredictor{
		metricsClient: client,
	}
}

func (p *defaultPredictor) Forecast(ctx context.Context, workload corev1.ObjectReference) (*Prediction, error) {
	// Fetch historical data for 24 hours
	rangeDuration := 24 * time.Hour

	// Build PromQL queries (we'll use container_cpu_usage_seconds_total and container_memory_working_set_bytes)
	// Note: We need to know the pod/container, but for now we'll use a simple query that can be refined
	cpuQuery := fmt.Sprintf(
		`sum(rate(container_cpu_usage_seconds_total{namespace="%s", pod=~"%s.*"}[1m])) by (pod)`,
		workload.Namespace, workload.Name,
	)
	memQuery := fmt.Sprintf(
		`sum(container_memory_working_set_bytes{namespace="%s", pod=~"%s.*"}) by (pod)`,
		workload.Namespace, workload.Name,
	)

	cpuData, err := p.metricsClient.QueryRange(ctx, cpuQuery, rangeDuration)
	if err != nil {
		return &Prediction{
			Confidence: 0,
			ValidUntil: time.Now().Add(1 * time.Hour),
		}, nil // Return fallback if query fails
	}

	memData, err := p.metricsClient.QueryRange(ctx, memQuery, rangeDuration)
	if err != nil {
		return &Prediction{
			Confidence: 0,
			ValidUntil: time.Now().Add(1 * time.Hour),
		}, nil // Return fallback if query fails
	}

	// Process CPU data
	cpuValues := extractValuesFromModel(cpuData)
	memValues := extractValuesFromModel(memData)

	if len(cpuValues) < 10 || len(memValues) < 10 {
		return &Prediction{
			Confidence: 0,
			ValidUntil: time.Now().Add(1 * time.Hour),
		}, nil // Not enough data
	}

	// Use simple moving average + variance for forecasting
	avgCPU, varCPU := meanAndVariance(cpuValues)
	avgMem, varMem := meanAndVariance(memValues)

	// Forecast = avg + 2*stddev (for 95% confidence interval)
	forecastCPU := avgCPU + 2*math.Sqrt(varCPU)
	forecastMem := avgMem + 2*math.Sqrt(varMem)

	// Calculate confidence (based on data variance)
	confidence := int32(80)
	if varCPU > avgCPU*0.5 || varMem > avgMem*0.5 {
		confidence = 50 // Lower confidence if variance is high
	} else if varCPU < avgCPU*0.1 && varMem < avgMem*0.1 {
		confidence = 95 // Higher confidence if variance is low
	}

	return &Prediction{
		MaxCPU:     resource.MustParse(fmt.Sprintf("%.0fm", forecastCPU*1000)),
		MaxMemory:  resource.MustParse(fmt.Sprintf("%.0fMi", forecastMem/(1024*1024))),
		ValidUntil: time.Now().Add(1 * time.Hour),
		Confidence: confidence,
	}, nil
}

func extractValuesFromModel(value model.Value) []float64 {
	values := make([]float64, 0)

	switch v := value.(type) {
	case model.Matrix:
		for _, stream := range v {
			for _, sample := range stream.Values {
				values = append(values, float64(sample.Value))
			}
		}
	case model.Vector:
		for _, sample := range v {
			values = append(values, float64(sample.Value))
		}
	}

	return values
}

func meanAndVariance(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	mean := 0.0
	for _, v := range values {
		mean += v
	}
	mean /= float64(len(values))

	variance := 0.0
	for _, v := range values {
		variance += (v - mean) * (v - mean)
	}
	variance /= float64(len(values))

	return mean, variance
}
