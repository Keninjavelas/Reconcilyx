package telemetry

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestTelemetryProvider_GetWorkloadMetrics(t *testing.T) {
	ref := corev1.ObjectReference{
		Namespace: "default",
		Name:      "test-workload",
	}

	t.Run("Multi-Container Success Path", func(t *testing.T) {
		mockClient := &MockMetricsClient{
			HealthCheckFunc: func(ctx context.Context) error { return nil },
			QueryFunc: func(ctx context.Context, query string) (model.Value, error) {
				var val1, val2 float64

				// Return two vectors for app and istio-proxy containers
				if strings.Contains(query, "resource_requests") && strings.Contains(query, `"cpu"`) {
					val1, val2 = 2.0, 0.5
				} else if strings.Contains(query, "resource_limits") && strings.Contains(query, `"cpu"`) {
					val1, val2 = 4.0, 1.0
				} else if strings.Contains(query, "container_cpu_usage_seconds_total") {
					val1, val2 = 1.5, 0.2
				} else if strings.Contains(query, "container_cpu_cfs_throttled_seconds_total") {
					val1, val2 = 0.1, 0.0
				} else if strings.Contains(query, "resource_requests") && strings.Contains(query, `"memory"`) {
					val1, val2 = 1024*1024*100, 1024*1024*50
				} else if strings.Contains(query, "resource_limits") && strings.Contains(query, `"memory"`) {
					val1, val2 = 1024*1024*200, 1024*1024*100
				} else if strings.Contains(query, "container_memory_working_set_bytes") {
					val1, val2 = 1024*1024*150, 1024*1024*60
				} else if strings.Contains(query, "OOMKilled") {
					val1, val2 = 2, 0
				} else if strings.Contains(query, "restarts_total") {
					val1, val2 = 5, 1
				} else if strings.Contains(query, "replicas_available") {
					// Replica query doesn't group by container, so return scalar representation
					return model.Vector{
						&model.Sample{Value: model.SampleValue(3)},
					}, nil
				}

				return model.Vector{
					&model.Sample{
						Metric: model.Metric{"container": "app"},
						Value:  model.SampleValue(val1),
					},
					&model.Sample{
						Metric: model.Metric{"container": "istio-proxy"},
						Value:  model.SampleValue(val2),
					},
				}, nil
			},
		}

		provider := NewTelemetryProvider(mockClient)
		metrics, err := provider.GetWorkloadMetrics(context.Background(), ref)

		assert.NoError(t, err)
		assert.NotNil(t, metrics)
		assert.Equal(t, int32(3), metrics.Replicas)

		assert.Len(t, metrics.Containers, 2)

		appMetrics := metrics.Containers["app"]
		assert.NotNil(t, appMetrics)
		assert.Equal(t, float64(2.0), appMetrics.CPU.RequestedCores)
		assert.Equal(t, float64(4.0), appMetrics.CPU.LimitedCores)
		assert.Equal(t, float64(1.5), appMetrics.CPU.UsageCores)
		assert.Equal(t, float64(104857600), appMetrics.Memory.RequestedBytes)
		assert.Equal(t, int32(5), appMetrics.Restarts)

		proxyMetrics := metrics.Containers["istio-proxy"]
		assert.NotNil(t, proxyMetrics)
		assert.Equal(t, float64(0.5), proxyMetrics.CPU.RequestedCores)
		assert.Equal(t, float64(52428800), proxyMetrics.Memory.RequestedBytes)
		assert.Equal(t, int32(1), proxyMetrics.Restarts)
	})

	t.Run("Health Check Failure", func(t *testing.T) {
		mockClient := &MockMetricsClient{
			HealthCheckFunc: func(ctx context.Context) error {
				return ErrPrometheusUnavailable
			},
		}

		provider := NewTelemetryProvider(mockClient)
		metrics, err := provider.GetWorkloadMetrics(context.Background(), ref)

		assert.ErrorIs(t, err, ErrPrometheusUnavailable)
		assert.Nil(t, metrics)
	})

	t.Run("Partial Telemetry Failure", func(t *testing.T) {
		mockClient := &MockMetricsClient{
			HealthCheckFunc: func(ctx context.Context) error { return nil },
			QueryFunc: func(ctx context.Context, query string) (model.Value, error) {
				if strings.Contains(query, `"cpu"`) || strings.Contains(query, "cpu_") {
					return nil, errors.New("timeout")
				}
				// Memory works
				return model.Vector{
					&model.Sample{
						Metric: model.Metric{"container": "app"},
						Value:  model.SampleValue(100),
					},
				}, nil
			},
		}

		provider := NewTelemetryProvider(mockClient)
		metrics, err := provider.GetWorkloadMetrics(context.Background(), ref)

		assert.ErrorIs(t, err, ErrPartialTelemetry)
		assert.NotNil(t, metrics)

		appMetrics := metrics.Containers["app"]
		assert.NotNil(t, appMetrics)
		assert.Equal(t, float64(100), appMetrics.Memory.RequestedBytes)
		assert.Equal(t, float64(0), appMetrics.CPU.RequestedCores) // CPU failed, so default 0
	})

	t.Run("Empty Metrics", func(t *testing.T) {
		mockClient := &MockMetricsClient{
			HealthCheckFunc: func(ctx context.Context) error { return nil },
			QueryFunc: func(ctx context.Context, query string) (model.Value, error) {
				return model.Vector{}, nil
			},
		}

		provider := NewTelemetryProvider(mockClient)
		metrics, err := provider.GetWorkloadMetrics(context.Background(), ref)

		// Doesn't strictly error if all queries successfully return empty vectors,
		// but the map will be empty.
		assert.NoError(t, err)
		assert.NotNil(t, metrics)
		assert.Len(t, metrics.Containers, 0)
	})

	t.Run("Merges JVM and GPU Metrics", func(t *testing.T) {
		appVector := func(v float64) model.Vector {
			return model.Vector{&model.Sample{Metric: model.Metric{"container": "app"}, Value: model.SampleValue(v)}}
		}
		mockClient := &MockMetricsClient{
			HealthCheckFunc: func(ctx context.Context) error { return nil },
			QueryFunc: func(ctx context.Context, query string) (model.Value, error) {
				switch {
				case strings.HasPrefix(strings.TrimSpace(query), "count(") && strings.Contains(query, "DCGM_FI_DEV_GPU_UTIL"):
					return appVector(1), nil
				case strings.Contains(query, "DCGM_FI_DEV_GPU_UTIL"):
					return appVector(64), nil
				case strings.Contains(query, "DCGM_FI_DEV_FB_USED"):
					return appVector(4096), nil
				case strings.Contains(query, "DCGM_FI_DEV_FB_TOTAL"):
					return appVector(8192), nil
				case strings.Contains(query, "DCGM_FI_DEV_GPU_TEMP"):
					return appVector(60), nil
				case strings.Contains(query, "jvm_memory_max_bytes"):
					return appVector(2 * 1024 * 1024 * 1024), nil
				case strings.Contains(query, "jvm_memory_bytes_committed"):
					return appVector(2 * 1024 * 1024 * 1024), nil
				case strings.Contains(query, "jvm_memory_bytes_used"):
					return appVector(1024 * 1024 * 1024), nil
				case strings.Contains(query, "replicas_available"):
					return model.Vector{&model.Sample{Value: 2}}, nil
				case strings.Contains(query, "resource_requests") && strings.Contains(query, `"cpu"`):
					return appVector(1), nil
				case strings.Contains(query, "resource_limits") && strings.Contains(query, `"cpu"`):
					return appVector(2), nil
				case strings.Contains(query, "resource_requests") && strings.Contains(query, `"memory"`):
					return appVector(1024 * 1024 * 1024), nil
				case strings.Contains(query, "resource_limits") && strings.Contains(query, `"memory"`):
					return appVector(2 * 1024 * 1024 * 1024), nil
				default:
					return appVector(0.5), nil
				}
			},
		}

		provider := NewTelemetryProvider(mockClient)
		metrics, err := provider.GetWorkloadMetrics(context.Background(), ref)
		assert.NoError(t, err)
		app := metrics.Containers["app"]
		assert.NotNil(t, app)
		assert.True(t, app.JVM.IsJVMWorkload)
		assert.Equal(t, float64(2*1024*1024*1024), app.JVM.XmxBytes)
		assert.Equal(t, int32(1), app.GPU.GPUCount)
		assert.Equal(t, float64(64), app.GPU.GPUUtilizationPct)
		assert.Equal(t, float64(4096*1024*1024), app.GPU.GPUMemoryUsedBytes)
	})
}
