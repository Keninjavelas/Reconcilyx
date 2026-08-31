package telemetry

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/common/model"
)

type cacheEntry struct {
	value  model.Value
	expiry time.Time
}

type retriableClient struct {
	inner    MetricsClient
	mu       sync.RWMutex
	cache    map[string]cacheEntry
	cacheTTL time.Duration
	retries  int
	baseWait time.Duration
	maxWait  time.Duration
}

func NewRetriableMetricsClient(inner MetricsClient) MetricsClient {
	return &retriableClient{
		inner:    inner,
		cache:    make(map[string]cacheEntry),
		cacheTTL: 5 * time.Minute,
		retries:  3,
		baseWait: 500 * time.Millisecond,
		maxWait:  8 * time.Second,
	}
}

func NewRetriableMetricsClientWithConfig(inner MetricsClient, cacheTTL time.Duration, retries int, baseWait, maxWait time.Duration) MetricsClient {
	return &retriableClient{
		inner:    inner,
		cache:    make(map[string]cacheEntry),
		cacheTTL: cacheTTL,
		retries:  retries,
		baseWait: baseWait,
		maxWait:  maxWait,
	}
}

func (c *retriableClient) cacheGet(key string) (model.Value, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.cache[key]
	if !ok || time.Now().After(e.expiry) {
		return nil, false
	}
	return e.value, true
}

func (c *retriableClient) cacheSet(key string, v model.Value) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[key] = cacheEntry{value: v, expiry: time.Now().Add(c.cacheTTL)}
}

func (c *retriableClient) PurgeExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for k, e := range c.cache {
		if now.After(e.expiry) {
			delete(c.cache, k)
		}
	}
}

func (c *retriableClient) retry(ctx context.Context, fn func() (model.Value, error)) (model.Value, error) {
	var lastErr error
	wait := c.baseWait
	for i := 0; i <= c.retries; i++ {
		v, err := fn()
		if err == nil {
			return v, nil
		}
		lastErr = err
		if i == c.retries {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
		wait = wait * 2
		if wait > c.maxWait {
			wait = c.maxWait
		}
	}
	return nil, fmt.Errorf("metrics client failed after %d retries: %w", c.retries, lastErr)
}

func (c *retriableClient) Query(ctx context.Context, query string) (model.Value, error) {
	cacheKey := "q:" + query
	if v, ok := c.cacheGet(cacheKey); ok {
		return v, nil
	}
	v, err := c.retry(ctx, func() (model.Value, error) {
		return c.inner.Query(ctx, query)
	})
	if err == nil {
		c.cacheSet(cacheKey, v)
	}
	return v, err
}

func (c *retriableClient) QueryRange(ctx context.Context, query string, r time.Duration) (model.Value, error) {
	cacheKey := "qr:" + query + ":" + r.String()
	if v, ok := c.cacheGet(cacheKey); ok {
		return v, nil
	}
	v, err := c.retry(ctx, func() (model.Value, error) {
		return c.inner.QueryRange(ctx, query, r)
	})
	if err == nil {
		c.cacheSet(cacheKey, v)
	}
	return v, err
}

func (c *retriableClient) HealthCheck(ctx context.Context) error {
	_, err := c.retry(ctx, func() (model.Value, error) {
		return nil, c.inner.HealthCheck(ctx)
	})
	if err != nil {
		return fmt.Errorf("health check retry: %w", err)
	}
	return nil
}
