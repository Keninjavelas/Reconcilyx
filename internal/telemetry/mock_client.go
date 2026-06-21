package telemetry

import (
	"context"
	"time"

	"github.com/prometheus/common/model"
)

// MockMetricsClient is a mock implementation of MetricsClient for unit testing.
type MockMetricsClient struct {
	QueryFunc       func(ctx context.Context, query string) (model.Value, error)
	QueryRangeFunc  func(ctx context.Context, query string, r time.Duration) (model.Value, error)
	HealthCheckFunc func(ctx context.Context) error
}

func (m *MockMetricsClient) Query(ctx context.Context, query string) (model.Value, error) {
	if m.QueryFunc != nil {
		return m.QueryFunc(ctx, query)
	}
	return nil, nil
}

func (m *MockMetricsClient) QueryRange(ctx context.Context, query string, r time.Duration) (model.Value, error) {
	if m.QueryRangeFunc != nil {
		return m.QueryRangeFunc(ctx, query, r)
	}
	return nil, nil
}

func (m *MockMetricsClient) HealthCheck(ctx context.Context) error {
	if m.HealthCheckFunc != nil {
		return m.HealthCheckFunc(ctx)
	}
	return nil
}
