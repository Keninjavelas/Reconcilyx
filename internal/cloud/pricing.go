package cloud

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type ProviderName string

const (
	ProviderAWS   ProviderName = "AWS"
	ProviderGCP   ProviderName = "GCP"
	ProviderAzure ProviderName = "Azure"
)

type InstanceType struct {
	Provider        ProviderName `json:"provider"`
	Region          string       `json:"region"`
	Name            string       `json:"name"`
	VCPU            float64      `json:"vcpu"`
	MemoryGB        float64      `json:"memoryGB"`
	OnDemandPrice   float64      `json:"onDemandPrice"`
	SpotPrice       float64      `json:"spotPrice"`
	CarbonIntensity float64      `json:"carbonIntensity"`
}

type PricingProvider interface {
	Name() ProviderName
	SpotPrice(ctx context.Context, instanceType, region string) (float64, error)
	OnDemandPrice(ctx context.Context, instanceType, region string) (float64, error)
	ListMatchingInstances(ctx context.Context, vCPU, memGB float64, region string) ([]InstanceType, error)
}

type priceKey struct {
	kind         string
	instanceType string
	region       string
}

type priceCacheEntry struct {
	value     float64
	timestamp time.Time
}

type listCacheEntry struct {
	instances []InstanceType
	timestamp time.Time
}

type CachedPricingProvider struct {
	provider     PricingProvider
	ttl          time.Duration
	mu           sync.RWMutex
	listCache    map[string]listCacheEntry
	priceCache   map[priceKey]priceCacheEntry
	priceErrOnce sync.Once
}

func NewCachedPricingProvider(provider PricingProvider, ttl time.Duration) *CachedPricingProvider {
	if ttl <= 0 {
		ttl = 1 * time.Hour
	}
	return &CachedPricingProvider{
		provider:   provider,
		ttl:        ttl,
		listCache:  make(map[string]listCacheEntry),
		priceCache: make(map[priceKey]priceCacheEntry),
	}
}

func (c *CachedPricingProvider) Name() ProviderName {
	return c.provider.Name()
}

func (c *CachedPricingProvider) getPrice(kind, instanceType, region string, loader func(context.Context, string, string) (float64, error), ctx context.Context) (float64, error) {
	key := priceKey{kind: kind, instanceType: instanceType, region: region}

	c.mu.RLock()
	entry, ok := c.priceCache[key]
	c.mu.RUnlock()

	if ok && time.Since(entry.timestamp) < c.ttl {
		return entry.value, nil
	}

	v, err := loader(ctx, instanceType, region)
	if err != nil {
		return 0, err
	}

	c.mu.Lock()
	c.priceCache[key] = priceCacheEntry{value: v, timestamp: time.Now()}
	c.mu.Unlock()
	return v, nil
}

func (c *CachedPricingProvider) SpotPrice(ctx context.Context, instanceType, region string) (float64, error) {
	return c.getPrice("spot", instanceType, region, c.provider.SpotPrice, ctx)
}

func (c *CachedPricingProvider) OnDemandPrice(ctx context.Context, instanceType, region string) (float64, error) {
	return c.getPrice("od", instanceType, region, c.provider.OnDemandPrice, ctx)
}

func (c *CachedPricingProvider) ListMatchingInstances(ctx context.Context, vCPU, memGB float64, region string) ([]InstanceType, error) {
	key := fmt.Sprintf("%.1f|%.1f|%s", vCPU, memGB, region)

	c.mu.RLock()
	entry, ok := c.listCache[key]
	c.mu.RUnlock()

	if ok && time.Since(entry.timestamp) < c.ttl {
		return entry.instances, nil
	}

	instances, err := c.provider.ListMatchingInstances(ctx, vCPU, memGB, region)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.listCache[key] = listCacheEntry{
		instances: instances,
		timestamp: time.Now(),
	}
	c.mu.Unlock()

	return instances, nil
}
