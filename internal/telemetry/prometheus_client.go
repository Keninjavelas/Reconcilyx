package telemetry

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

type prometheusClient struct {
	api v1.API
}

// NewPrometheusClient creates a new MetricsClient wrapping the official Prometheus API.
func NewPrometheusClient(address string) (MetricsClient, error) {
	client, err := api.NewClient(api.Config{
		Address: address,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create prometheus client: %w", err)
	}

	return &prometheusClient{
		api: v1.NewAPI(client),
	}, nil
}

func (c *prometheusClient) Query(ctx context.Context, query string) (model.Value, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var lastErr error

	// Simple retry loop with backoff: 500ms, 1s, 2s
	backoffs := []time.Duration{500 * time.Millisecond, 1 * time.Second, 2 * time.Second}

	for attempt := 0; attempt <= len(backoffs); attempt++ {
		res, warnings, err := c.api.Query(ctx, query, time.Now())
		if err == nil {
			if len(warnings) > 0 {
				// We can log warnings here in a real implementation
			}
			if res == nil {
				return nil, ErrNoData
			}
			return res, nil
		}

		lastErr = err

		if attempt < len(backoffs) {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("query timed out after %d attempts: %w", attempt+1, ctx.Err())
			case <-time.After(backoffs[attempt]):
				continue
			}
		}
	}

	return nil, fmt.Errorf("query failed after %d attempts: %w", len(backoffs)+1, lastErr)
}

func (c *prometheusClient) QueryRange(ctx context.Context, query string, r time.Duration) (model.Value, error) {
	now := time.Now()
	return c.QueryRangeWindow(ctx, query, now.Add(-r), now, r/10)
}

// QueryRangeWindow executes an explicit [start, end] PromQL range query at the given step.
func (c *prometheusClient) QueryRangeWindow(ctx context.Context, query string, start, end time.Time, step time.Duration) (model.Value, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if step <= 0 {
		step = 5 * time.Minute
	}
	ran := v1.Range{
		Start: start,
		End:   end,
		Step:  step,
	}

	res, _, err := c.api.QueryRange(ctx, query, ran)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, ErrNoData
	}
	return res, nil
}

func (c *prometheusClient) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	_, err := c.api.Buildinfo(ctx)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPrometheusUnavailable, err)
	}
	return nil
}
