package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"time"
)

const (
	gcpBillingCatalogURL = "https://cloudbilling.googleapis.com/v1/services/6F81-5844-456A/skus"
)

type GCPPricingProvider struct {
	HTTPClient *http.Client
	BaseURL    string
	apiKey     string
	enableLive bool
	fallback   *staticGCPCatalog
}

type staticGCPCatalog struct {
	rates  map[string]float64
	carbon map[string]float64
}

func newStaticGCPCatalog() *staticGCPCatalog {
	return &staticGCPCatalog{
		rates: map[string]float64{
			"e2-standard-2": 0.067,
			"e2-standard-4": 0.134,
			"n2-standard-2": 0.082,
			"n2-standard-4": 0.164,
		},
		carbon: map[string]float64{
			"us-central1":  320.0,
			"us-west1":     90.0,
			"europe-west1": 130.0,
		},
	}
}

func NewGCPPricingProvider() *GCPPricingProvider {
	return &GCPPricingProvider{
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
		BaseURL:    gcpBillingCatalogURL,
		enableLive: false,
		fallback:   newStaticGCPCatalog(),
	}
}

func NewLiveGCPPricingProvider(apiKey string) *GCPPricingProvider {
	p := NewGCPPricingProvider()
	p.enableLive = true
	p.apiKey = apiKey
	return p
}

func (g *GCPPricingProvider) Name() ProviderName { return ProviderGCP }

func carbonForGCP(region string, cmap map[string]float64) float64 {
	if c, ok := cmap[region]; ok {
		return c
	}
	return 250.0
}

func (g *GCPPricingProvider) fetchOnDemandLive(ctx context.Context, instanceType, region string) (float64, error) {
	if g.HTTPClient == nil || g.BaseURL == "" || g.apiKey == "" {
		return 0, fmt.Errorf("live GCP pricing disabled (need HTTPClient, BaseURL, and API key)")
	}
	u, _ := url.Parse(g.BaseURL)
	q := u.Query()
	q.Set("key", g.apiKey)
	categoryFilter := fmt.Sprintf("resourceFamily = ComputeEngine AND region = %s", region)
	q.Set("filter", categoryFilter)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, err
	}
	resp, err := g.HTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("gcp billing api status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 20*1024*1024))
	if err != nil {
		return 0, err
	}
	var doc struct {
		Skus []struct {
			SkuId       string   `json:"skuId"`
			Description string   `json:"description"`
			Category    struct {
				ResourceFamily string `json:"resourceFamily"`
				UsageType      string `json:"usageType"`
			} `json:"category"`
			ServiceRegions []string `json:"serviceRegions"`
			PricingInfo    []struct {
				PricingExpression struct {
					DisplayQuantity float64 `json:"displayQuantity"`
					TieredRates     []struct {
						UnitPrice struct {
							CurrencyCode string  `json:"currencyCode"`
							Units        string  `json:"units"`
							Nanos        float64 `json:"nanos"`
						} `json:"unitPrice"`
					} `json:"tieredRates"`
				} `json:"pricingExpression"`
			} `json:"pricingInfo"`
		} `json:"skus"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return 0, err
	}
	for _, sku := range doc.Skus {
		if sku.Category.ResourceFamily != "Compute" || sku.Category.UsageType != "OnDemand" {
			continue
		}
		found := false
		for _, r := range sku.ServiceRegions {
			if r == region {
				found = true
				break
			}
		}
		if !found {
			continue
		}
		if !containsGCPInstance(sku.Description, instanceType) {
			continue
		}
		for _, pi := range sku.PricingInfo {
			for _, tr := range pi.PricingExpression.TieredRates {
				u1, _ := strconvParseFloat64(tr.UnitPrice.Units)
				price := u1 + tr.UnitPrice.Nanos/1e9
				if price > 0 {
					return price, nil
				}
			}
		}
	}
	return 0, fmt.Errorf("gcp sku for %s/%s not found", region, instanceType)
}

func containsGCPInstance(desc, instanceType string) bool {
	if desc == "" {
		return false
	}
	return indexOf(desc, instanceType) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func strconvParseFloat64(s string) (float64, error) {
	if s == "" {
		return 0, nil
	}
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}

func (g *GCPPricingProvider) OnDemandPrice(ctx context.Context, instanceType, region string) (float64, error) {
	if g.HTTPClient != nil && g.BaseURL != "" && g.apiKey != "" {
		if v, err := g.fetchOnDemandLive(ctx, instanceType, region); err == nil && v > 0 {
			return v, nil
		}
	}
	if rate, ok := g.fallback.rates[instanceType]; ok {
		return rate, nil
	}
	return 0.09, nil
}

func (g *GCPPricingProvider) SpotPrice(ctx context.Context, instanceType, region string) (float64, error) {
	od, err := g.OnDemandPrice(ctx, instanceType, region)
	if err != nil {
		return 0, err
	}
	return od * 0.30, nil
}

func (g *GCPPricingProvider) ListMatchingInstances(ctx context.Context, vCPU, memGB float64, region string) ([]InstanceType, error) {
	carbon := carbonForGCP(region, g.fallback.carbon)
	catalog := []InstanceType{
		{Provider: ProviderGCP, Region: region, Name: "e2-standard-2", VCPU: 2.0, MemoryGB: 8.0, OnDemandPrice: 0.0670, SpotPrice: 0.0201, CarbonIntensity: carbon},
		{Provider: ProviderGCP, Region: region, Name: "n2-standard-2", VCPU: 2.0, MemoryGB: 8.0, OnDemandPrice: 0.0820, SpotPrice: 0.0246, CarbonIntensity: carbon},
		{Provider: ProviderGCP, Region: region, Name: "e2-standard-4", VCPU: 4.0, MemoryGB: 16.0, OnDemandPrice: 0.1340, SpotPrice: 0.0402, CarbonIntensity: carbon},
		{Provider: ProviderGCP, Region: region, Name: "n2-standard-4", VCPU: 4.0, MemoryGB: 16.0, OnDemandPrice: 0.1640, SpotPrice: 0.0492, CarbonIntensity: carbon},
	}
	if g.HTTPClient != nil && g.BaseURL != "" && g.apiKey != "" {
		for i := range catalog {
			live, err := g.fetchOnDemandLive(ctx, catalog[i].Name, region)
			if err == nil && live > 0 {
				catalog[i].OnDemandPrice = live
				catalog[i].SpotPrice = live * 0.30
			}
		}
	}
	var matched []InstanceType
	for _, inst := range catalog {
		if inst.VCPU >= vCPU && inst.MemoryGB >= memGB {
			matched = append(matched, inst)
		}
	}
	if len(matched) == 0 {
		matched = append(matched, InstanceType{
			Provider:        ProviderGCP,
			Region:          region,
			Name:            fmt.Sprintf("custom-gcp-%.0fvcpu-%.0fgb", math.Ceil(vCPU), math.Ceil(memGB)),
			VCPU:            math.Ceil(vCPU),
			MemoryGB:        math.Ceil(memGB),
			OnDemandPrice:   0.033*vCPU + 0.004*memGB,
			SpotPrice:       (0.033*vCPU + 0.004*memGB) * 0.30,
			CarbonIntensity: carbon,
		})
	}
	return matched, nil
}
