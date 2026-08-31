package anomaly

import (
	"math"
)

// AnomalyType represents the type of detected anomaly
type AnomalyType string

const (
	AnomalySpike  AnomalyType = "spike"
	AnomalyDrop   AnomalyType = "drop"
	AnomalyNone   AnomalyType = "none"
)

// AnomalyResult contains details about a detected anomaly
type AnomalyResult struct {
	Type        AnomalyType
	Severity    float64 // 0-1, higher is more severe
	Description string
}

// Detector defines the interface for anomaly detection
type Detector interface {
	Detect(historicalData []float64, currentValue float64) (*AnomalyResult, error)
}

// ZScoreDetector uses Z-score for anomaly detection
type ZScoreDetector struct {
	Threshold float64 // Standard deviations from mean to consider as anomaly (default: 3)
}

// NewZScoreDetector creates a new Z-score anomaly detector
func NewZScoreDetector(threshold float64) *ZScoreDetector {
	if threshold <= 0 {
		threshold = 3.0
	}
	return &ZScoreDetector{Threshold: threshold}
}

// Detect finds anomalies using Z-score method
func (d *ZScoreDetector) Detect(historicalData []float64, currentValue float64) (*AnomalyResult, error) {
	if len(historicalData) < 3 {
		return &AnomalyResult{Type: AnomalyNone, Severity: 0, Description: "Not enough historical data"}, nil
	}

	mean, variance := calculateMeanAndVariance(historicalData)
	if variance == 0 {
		return &AnomalyResult{Type: AnomalyNone, Severity: 0, Description: "No variance in historical data"}, nil
	}

	stdDev := math.Sqrt(variance)
	zScore := (currentValue - mean) / stdDev
	absZScore := math.Abs(zScore)

	if absZScore < d.Threshold {
		return &AnomalyResult{Type: AnomalyNone, Severity: 0, Description: "No anomaly detected"}, nil
	}

	severity := math.Min(1.0, (absZScore-d.Threshold)/d.Threshold) // Normalize to 0-1
	var anomalyType AnomalyType
	var description string

	if zScore > 0 {
		anomalyType = AnomalySpike
		description = "Detected sudden spike in usage"
	} else {
		anomalyType = AnomalyDrop
		description = "Detected sudden drop in usage"
	}

	return &AnomalyResult{
		Type:        anomalyType,
		Severity:    severity,
		Description: description,
	}, nil
}

func calculateMeanAndVariance(data []float64) (float64, float64) {
	sum := 0.0
	for _, v := range data {
		sum += v
	}
	mean := sum / float64(len(data))

	sumSquaredDiff := 0.0
	for _, v := range data {
		diff := v - mean
		sumSquaredDiff += diff * diff
	}
	variance := sumSquaredDiff / float64(len(data))
	return mean, variance
}
