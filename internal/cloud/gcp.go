package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	gcpBillingCatalogURL = "https://cloudbilling.googleapis.com/v1/services/6F81-5844-456A/skus"
)

type GCPPricingProvider struct {
	HTTPClient *http.Client
	BaseURL    string
	Retry      RetryPolicy
	apiKey     string
	enableLive bool
	fallback   *staticGCPCatalog
}

type gcpMachineSpec struct {
	family string
	vcpu   float64
	memGB  float64
}

// Live GCP list prices are published per vCPU-hour and per GB-hour, not per
// machine type. These specs let the client total the component SKUs for the
// predefined machine types it quotes.
var gcpLiveMachineSpecs = map[string]gcpMachineSpec{
	"e2-standard-2": {family: "e2", vcpu: 2, memGB: 8},
	"e2-standard-4": {family: "e2", vcpu: 4, memGB: 16},
	"n2-standard-2": {family: "n2", vcpu: 2, memGB: 8},
	"n2-standard-4": {family: "n2", vcpu: 4, memGB: 16},
}

type gcpBillingSKU struct {
	Description string `json:"description"`
	Category    struct {
		ResourceFamily string `json:"resourceFamily"`
		UsageType      string `json:"usageType"`
	} `json:"category"`
	ServiceRegions []string `json:"serviceRegions"`
	PricingInfo    []struct {
		PricingExpression struct {
			UsageUnit   string `json:"usageUnit"`
			TieredRates []struct {
				UnitPrice struct {
					CurrencyCode string  `json:"currencyCode"`
					Units        string  `json:"units"`
					Nanos        float64 `json:"nanos"`
				} `json:"unitPrice"`
			} `json:"tieredRates"`
		} `json:"pricingExpression"`
	} `json:"pricingInfo"`
}

type gcpBillingDoc struct {
	NextPageToken string          `json:"nextPageToken"`
	Skus          []gcpBillingSKU `json:"skus"`
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
	p.enableLive = apiKey != ""
	p.apiKey = apiKey
	return p
}

func (g *GCPPricingProvider) Name() ProviderName { return ProviderGCP }

func (g *GCPPricingProvider) Live() bool { return g.enableLive }

func carbonForGCP(region string, cmap map[string]float64) float64 {
	if c, ok := cmap[region]; ok {
		return c
	}
	return 250.0
}

func (g *GCPPricingProvider) fetchOnDemandLive(ctx context.Context, instanceType, region string) (float64, error) {
	if !g.enableLive || g.HTTPClient == nil || g.BaseURL == "" || g.apiKey == "" {
		return 0, fmt.Errorf("live GCP pricing disabled (need credentials, client, and base URL)")
	}
	spec, ok := gcpLiveMachineSpecs[instanceType]
	if !ok {
		return 0, fmt.Errorf("gcp live pricing does not know machine type %s", instanceType)
	}

	const maxPages = 20
	var pageToken string
	var corePrice, ramPrice float64
	for page := 0; page < maxPages; page++ {
		token := pageToken
		resp, err := doWithRetry(ctx, g.HTTPClient, func() (*http.Request, error) {
			u, err := url.Parse(g.BaseURL)
			if err != nil {
				return nil, err
			}
			q := u.Query()
			q.Set("key", g.apiKey)
			q.Set("pageSize", "5000")
			q.Set("currencyCode", "USD")
			if token != "" {
				q.Set("pageToken", token)
			}
			u.RawQuery = q.Encode()
			return http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
		}, g.Retry)
		if err != nil {
			return 0, err
		}

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 20*1024*1024))
		resp.Body.Close()
		if readErr != nil {
			return 0, readErr
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return 0, fmt.Errorf("gcp billing api status %d: %s", resp.StatusCode, truncate(string(body), 256))
		}

		var doc gcpBillingDoc
		if err := json.Unmarshal(body, &doc); err != nil {
			return 0, err
		}
		core, ram := gcpComponentPrices(doc.Skus, spec.family, region)
		if core > 0 {
			corePrice = core
		}
		if ram > 0 {
			ramPrice = ram
		}
		if corePrice > 0 && ramPrice > 0 {
			return spec.vcpu*corePrice + spec.memGB*ramPrice, nil
		}
		if doc.NextPageToken == "" || doc.NextPageToken == pageToken {
			break
		}
		pageToken = doc.NextPageToken
	}
	return 0, fmt.Errorf("gcp component prices for %s/%s not found", region, instanceType)
}

func gcpComponentPrices(skus []gcpBillingSKU, family, region string) (core, ram float64) {
	corePrefix := strings.ToUpper(family) + " INSTANCE CORE"
	ramPrefix := strings.ToUpper(family) + " INSTANCE RAM"
	for _, sku := range skus {
		if sku.Category.ResourceFamily != "Compute" || sku.Category.UsageType != "OnDemand" {
			continue
		}
		desc := strings.ToUpper(sku.Description)
		if strings.Contains(desc, "CUSTOM") || !gcpSKUInRegion(sku, region) {
			continue
		}
		price := gcpSKUHourlyPrice(sku)
		if price <= 0 {
			continue
		}
		if core == 0 && strings.HasPrefix(desc, corePrefix) {
			core = price
		}
		if ram == 0 && strings.HasPrefix(desc, ramPrefix) {
			ram = price
		}
	}
	return core, ram
}

func gcpSKUInRegion(sku gcpBillingSKU, region string) bool {
	for _, r := range sku.ServiceRegions {
		if r == region {
			return true
		}
	}
	return false
}

func gcpSKUHourlyPrice(sku gcpBillingSKU) float64 {
	for _, pi := range sku.PricingInfo {
		if pi.PricingExpression.UsageUnit != "" && !strings.EqualFold(pi.PricingExpression.UsageUnit, "h") {
			continue
		}
		for _, tr := range pi.PricingExpression.TieredRates {
			units, _ := strconvParseFloat64(tr.UnitPrice.Units)
			price := units + tr.UnitPrice.Nanos/1e9
			if price > 0 {
				return price
			}
		}
	}
	return 0
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
	if g.enableLive {
		if v, err := g.fetchOnDemandLive(ctx, instanceType, region); err == nil && v > 0 {
			return v, nil
		} else if err != nil {
			slog.Warn("Live GCP pricing unavailable; using bundled fallback",
				"instance", instanceType, "region", region, "error", err.Error())
		}
	}
	if rate, ok := g.fallback.rates[instanceType]; ok {
		return rate, nil
	}
	return 0, fmt.Errorf("no GCP price for %s in %s and no local fallback", instanceType, region)
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
	if g.enableLive {
		for i := range catalog {
			live, err := g.fetchOnDemandLive(ctx, catalog[i].Name, region)
			if err == nil && live > 0 {
				catalog[i].OnDemandPrice = live
				catalog[i].SpotPrice = live * 0.30
				continue
			}
			if err != nil {
				slog.Warn("Live GCP price refresh failed; keeping bundled price",
					"instance", catalog[i].Name, "region", region, "error", err.Error())
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
