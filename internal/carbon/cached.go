package carbon

import (
	"context"
	"sync"
	"time"
)

type CachedCarbonProvider struct {
	provider Provider
	ttl      time.Duration
	mu       sync.RWMutex
	cache    map[string]cacheEntry
}

type cacheEntry struct {
	intensity Intensity
	timestamp time.Time
	err       error
}

func NewCachedCarbonProvider(provider Provider, ttl time.Duration) *CachedCarbonProvider {
	if ttl <= 0 {
		ttl = 1 * time.Hour
	}
	return &CachedCarbonProvider{
		provider: provider,
		ttl:      ttl,
		cache:    make(map[string]cacheEntry),
	}
}

func (c *CachedCarbonProvider) IntensityForRegion(ctx context.Context, region string) (Intensity, error) {
	c.mu.RLock()
	entry, ok := c.cache[region]
	c.mu.RUnlock()

	if ok && time.Since(entry.timestamp) < c.ttl {
		return entry.intensity, entry.err
	}

	intensity, err := c.provider.IntensityForRegion(ctx, region)
	c.mu.Lock()
	c.cache[region] = cacheEntry{
		intensity: intensity,
		timestamp: time.Now(),
		err:       err,
	}
	c.mu.Unlock()
	return intensity, err
}
