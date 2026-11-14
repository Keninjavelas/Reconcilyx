package rules

import (
	"testing"

	"github.com/reconcilyx/operator/api/v1alpha1"
	"github.com/reconcilyx/operator/internal/telemetry"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestCPUOverprovisionedRule(t *testing.T) {
	rule := &CPUOverprovisionedRule{}

	current := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("2000m"), // 2 cores
		},
	}

	metrics := &telemetry.ContainerMetrics{
		CPU: telemetry.CPUMetrics{
			UsageCores: 0.5, // 0.5 cores
		},
	}

	finding, err := rule.Evaluate("app", metrics, current)
	assert.NoError(t, err)
	assert.NotNil(t, finding)
	assert.Equal(t, v1alpha1.RecommendationRightSize, finding.Recommendation)
	
	// Asymmetric CPU policy: p95 usage (fallback 0.5) * 1.15 = 575m
	expectedProp := resource.MustParse("575m")
	proposed := finding.ProposedRequest.Cpu().MilliValue()
	assert.Equal(t, expectedProp.MilliValue(), proposed)
}

func TestAsymmetricSizingAndOOMProtection(t *testing.T) {
	t.Run("CPU uses p95 from historical slice + 15%", func(t *testing.T) {
		rule := &CPUOverprovisionedRule{}
		current := corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("4000m")},
		}
		hist := make([]float64, 20)
		for i := range hist {
			hist[i] = 0.2
		}
		hist[18] = 1.0 // nearest-rank p95 (rank 19 / index 18) = 1.0
		hist[19] = 1.5 // max peak (p100) ignored by p95 target
		metrics := &telemetry.ContainerMetrics{
			CPU: telemetry.CPUMetrics{UsageCores: 0.2, HistoricalCores: hist},
		}
		finding, err := rule.Evaluate("app", metrics, current)
		assert.NoError(t, err)
		assert.NotNil(t, finding)
		assert.Equal(t, int64(1150), finding.ProposedRequest.Cpu().MilliValue())
	})

	t.Run("Memory sets both Request and Limit to p99 + 25%", func(t *testing.T) {
		rule := &MemOverprovisionedRule{}
		current := corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")},
		}
		metrics := &telemetry.ContainerMetrics{
			Memory: telemetry.MemoryMetrics{
				WorkingSetBytes:    256 * 1024 * 1024,
				P99WorkingSetBytes: 512 * 1024 * 1024,
			},
		}
		finding, err := rule.Evaluate("app", metrics, current)
		assert.NoError(t, err)
		assert.NotNil(t, finding)
		assert.NotNil(t, finding.ProposedLimit)
		// 512Mi * 1.25 = 640Mi
		assert.Equal(t, int64(640*1024*1024), finding.ProposedRequest.Memory().Value())
		assert.Equal(t, int64(640*1024*1024), finding.ProposedLimit.Memory().Value())
	})

	t.Run("OOMKill refuses downsize and proposes upward +25%", func(t *testing.T) {
		cpuRule := &CPUOverprovisionedRule{}
		memRule := &MemOverprovisionedRule{}
		current := corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2000m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		}
		metrics := &telemetry.ContainerMetrics{
			CPU:    telemetry.CPUMetrics{UsageCores: 0.1},
			Memory: telemetry.MemoryMetrics{WorkingSetBytes: 256 * 1024 * 1024, OOMCount: 1},
		}
		cpuFinding, err := cpuRule.Evaluate("app", metrics, current)
		assert.NoError(t, err)
		assert.Nil(t, cpuFinding)

		memFinding, err := memRule.Evaluate("app", metrics, current)
		assert.NoError(t, err)
		assert.NotNil(t, memFinding)
		// 512Mi * 1.25 = 640Mi upward proposal
		assert.Equal(t, int64(640*1024*1024), memFinding.ProposedRequest.Memory().Value())
		assert.Equal(t, int64(640*1024*1024), memFinding.ProposedLimit.Memory().Value())
	})

	t.Run("High restart count blocks CPU/Mem downsize", func(t *testing.T) {
		memRule := &MemOverprovisionedRule{}
		current := corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("2Gi")},
		}
		metrics := &telemetry.ContainerMetrics{
			Memory:   telemetry.MemoryMetrics{WorkingSetBytes: 128 * 1024 * 1024},
			Restarts: 4,
		}
		finding, err := memRule.Evaluate("app", metrics, current)
		assert.NoError(t, err)
		assert.Nil(t, finding)
	})
}

func TestMemOverprovisionedRule(t *testing.T) {
	rule := &MemOverprovisionedRule{}

	current := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
	}

	metrics := &telemetry.ContainerMetrics{
		Memory: telemetry.MemoryMetrics{
			WorkingSetBytes: 512 * 1024 * 1024, // 512 MB
		},
	}

	finding, err := rule.Evaluate("app", metrics, current)
	assert.NoError(t, err)
	assert.NotNil(t, finding)
	assert.Equal(t, v1alpha1.RecommendationRightSize, finding.Recommendation)
}

func TestIdleWorkloadRule(t *testing.T) {
	rule := &IdleWorkloadRule{}

	metrics := &telemetry.ContainerMetrics{
		CPU: telemetry.CPUMetrics{UsageCores: 0.005},
		Memory: telemetry.MemoryMetrics{WorkingSetBytes: 10 * 1024 * 1024},
	}

	finding, err := rule.Evaluate("app", metrics, corev1.ResourceRequirements{})
	assert.NoError(t, err)
	assert.NotNil(t, finding)
	assert.Equal(t, v1alpha1.RecommendationScaleZero, finding.Recommendation)
}
