package cloud

import (
	"context"
	"testing"
	"time"
)

func TestPricingProviders(t *testing.T) {
	ctx := context.Background()

	providers := []PricingProvider{
		NewAWSPricingProvider(),
		NewGCPPricingProvider(),
		NewAzurePricingProvider(),
	}

	for _, p := range providers {
		t.Run(string(p.Name()), func(t *testing.T) {
			instances, err := p.ListMatchingInstances(ctx, 2.0, 4.0, "us-east-1")
			if err != nil {
				t.Fatalf("ListMatchingInstances failed for %s: %v", p.Name(), err)
			}
			if len(instances) == 0 {
				t.Fatalf("Expected matched instances for %s, got 0", p.Name())
			}

			inst := instances[0]
			if inst.OnDemandPrice <= 0 || inst.SpotPrice <= 0 {
				t.Errorf("Invalid pricing for %s %s: onDemand=%f spot=%f", p.Name(), inst.Name, inst.OnDemandPrice, inst.SpotPrice)
			}
		})
	}
}

func TestCachedPricingProvider(t *testing.T) {
	ctx := context.Background()
	raw := NewAWSPricingProvider()
	cached := NewCachedPricingProvider(raw, 500*time.Millisecond)

	// First call should populate cache
	res1, err := cached.ListMatchingInstances(ctx, 2.0, 4.0, "us-west-2")
	if err != nil {
		t.Fatalf("First call failed: %v", err)
	}

	// Second call should return cached instance
	res2, err := cached.ListMatchingInstances(ctx, 2.0, 4.0, "us-west-2")
	if err != nil {
		t.Fatalf("Second call failed: %v", err)
	}

	if len(res1) != len(res2) {
		t.Errorf("Cache mismatch: expected len %d, got %d", len(res1), len(res2))
	}
}
