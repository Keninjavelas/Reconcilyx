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
	
	// usage(0.5) * 1.2 = 0.6 cores. Proposed should be 600m
	expectedProp := resource.MustParse("600m")
	proposed := finding.ProposedRequest.Cpu().MilliValue()
	assert.Equal(t, expectedProp.MilliValue(), proposed)
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
