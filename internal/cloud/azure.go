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

	"github.com/reconcilyx/operator/pkg/pipelinemetrics"
)

const (
	azureRetailPricesURL = "https://prices.azure.com/api/retail/prices"
)

// AzurePricingProvider reads public list prices from the Azure Retail Prices
// API. The API is anonymous, so no subscription or credentials are required.
type AzurePricingProvider struct {
	HTTPClient *http.Client
	BaseURL    string
	Retry      RetryPolicy
	enableLive bool
	fallback   *staticAzureCatalog
}

type staticAzureCatalog struct {
	rates  map[string]float64
	carbon map[string]float64
}

func newStaticAzureCatalog() *staticAzureCatalog {
	return &staticAzureCatalog{
		rates: map[string]float64{
			"Standard_B2s":    0.0416,
			"Standard_D2s_v5": 0.0960,
			"Standard_D4s_v5": 0.1920,
		},
		carbon: map[string]float64{
			"eastus":     380.0,
			"westus2":    115.0,
			"westeurope": 220.0,
		},
	}
}

func NewAzurePricingProvider() *AzurePricingProvider {
	return &AzurePricingProvider{
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
		BaseURL:    azureRetailPricesURL,
		enableLive: false,
		fallback:   newStaticAzureCatalog(),
	}
}

func NewLiveAzurePricingProvider() *AzurePricingProvider {
	p := NewAzurePricingProvider()
	p.enableLive = true
	return p
}

func (az *AzurePricingProvider) Name() ProviderName { return ProviderAzure }

func (az *AzurePricingProvider) Live() bool { return az.enableLive }

func carbonForAzure(region string, cmap map[string]float64) float64 {
	if c, ok := cmap[region]; ok {
		return c
	}
	return 290.0
}

func (az *AzurePricingProvider) fetchOnDemandLive(ctx context.Context, instanceType, region string) (float64, error) {
	if !az.enableLive || az.HTTPClient == nil || az.BaseURL == "" {
		return 0, fmt.Errorf("live Azure pricing disabled (no client or base URL)")
	}
	filter := fmt.Sprintf("armRegionName eq '%s' and serviceName eq 'Virtual Machines' and armSkuName eq '%s' and priceType eq 'Consumption'", region, instanceType)

	resp, err := doWithRetry(ctx, az.HTTPClient, func() (*http.Request, error) {
		u, err := url.Parse(az.BaseURL)
		if err != nil {
			return nil, err
		}
		q := u.Query()
		q.Set("$filter", filter)
		u.RawQuery = q.Encode()
		return http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	}, az.Retry)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return 0, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("azure retail prices api status %d: %s", resp.StatusCode, truncate(string(body), 256))
	}

	var doc struct {
		Items []struct {
			ArmSkuName           string  `json:"armSkuName"`
			ArmRegionName        string  `json:"armRegionName"`
			SkuName              string  `json:"skuName"`
			MeterName            string  `json:"meterName"`
			ProductName          string  `json:"productName"`
			PriceType            string  `json:"priceType"`
			ReservationTerm      string  `json:"reservationTerm"`
			RetailPrice          float64 `json:"retailPrice"`
			IsPrimaryMeterRegion bool    `json:"isPrimaryMeterRegion"`
			UnitOfMeasure        string  `json:"unitOfMeasure"`
		} `json:"Items"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return 0, err
	}

	var best float64
	var bestPrimary bool
	for _, it := range doc.Items {
		if it.ArmSkuName != "" && it.ArmSkuName != instanceType {
			continue
		}
		if it.ArmRegionName != "" && it.ArmRegionName != region {
			continue
		}
		if !isLinuxOnDemandAzureMeter(it.SkuName, it.MeterName, it.ProductName, it.PriceType, it.ReservationTerm) {
			continue
		}
		if it.RetailPrice <= 0 {
			continue
		}
		if best == 0 ||
			(it.IsPrimaryMeterRegion && !bestPrimary) ||
			(it.IsPrimaryMeterRegion == bestPrimary && it.RetailPrice < best) {
			best = it.RetailPrice
			bestPrimary = it.IsPrimaryMeterRegion
		}
	}
	if best <= 0 {
		return 0, fmt.Errorf("azure %s/%s not found in retail prices", region, instanceType)
	}
	return best, nil
}

// The Retail Prices API returns every meter for a SKU, including Spot, Low
// Priority, and Windows variants. Picking the cheapest would understate the
// Linux on-demand list price, so non-standard meters are excluded.
func isLinuxOnDemandAzureMeter(skuName, meterName, productName, priceType, reservationTerm string) bool {
	descriptor := strings.ToLower(strings.Join([]string{skuName, meterName, productName}, " "))
	if strings.Contains(descriptor, "spot") ||
		strings.Contains(descriptor, "low priority") ||
		strings.Contains(descriptor, "windows") {
		return false
	}
	if reservationTerm != "" {
		return false
	}
	return priceType == "" || strings.EqualFold(priceType, "Consumption")
}

func (az *AzurePricingProvider) OnDemandPrice(ctx context.Context, instanceType, region string) (float64, error) {
	if az.enableLive {
		if v, err := az.fetchOnDemandLive(ctx, instanceType, region); err == nil && v > 0 {
			pipelinemetrics.FallbackActive.WithLabelValues("cloud_pricing", "azure").Set(0)
			return v, nil
		} else if err != nil {
			pipelinemetrics.FallbackActive.WithLabelValues("cloud_pricing", "azure").Set(1)
			slog.Warn("Live Azure pricing unavailable; using bundled fallback",
				"instance", instanceType, "region", region, "error", err.Error())
		}
	}
	if rate, ok := az.fallback.rates[instanceType]; ok {
		if !az.enableLive {
			pipelinemetrics.FallbackActive.WithLabelValues("cloud_pricing", "azure").Set(1)
		}
		return rate, nil
	}
	return 0, fmt.Errorf("no Azure price for %s in %s and no local fallback", instanceType, region)
}

func (az *AzurePricingProvider) SpotPrice(ctx context.Context, instanceType, region string) (float64, error) {
	od, err := az.OnDemandPrice(ctx, instanceType, region)
	if err != nil {
		return 0, err
	}
	return od * 0.32, nil
}

func (az *AzurePricingProvider) ListMatchingInstances(ctx context.Context, vCPU, memGB float64, region string) ([]InstanceType, error) {
	carbon := carbonForAzure(region, az.fallback.carbon)
	catalog := []InstanceType{
		{Provider: ProviderAzure, Region: region, Name: "Standard_B2s", VCPU: 2.0, MemoryGB: 4.0, OnDemandPrice: 0.0416, SpotPrice: 0.0133, CarbonIntensity: carbon},
		{Provider: ProviderAzure, Region: region, Name: "Standard_D2s_v5", VCPU: 2.0, MemoryGB: 8.0, OnDemandPrice: 0.0960, SpotPrice: 0.0307, CarbonIntensity: carbon},
		{Provider: ProviderAzure, Region: region, Name: "Standard_D4s_v5", VCPU: 4.0, MemoryGB: 16.0, OnDemandPrice: 0.1920, SpotPrice: 0.0614, CarbonIntensity: carbon},
	}
	if az.enableLive {
		for i := range catalog {
			live, err := az.fetchOnDemandLive(ctx, catalog[i].Name, region)
			if err == nil && live > 0 {
				catalog[i].OnDemandPrice = live
				catalog[i].SpotPrice = live * 0.32
				continue
			}
			if err != nil {
				slog.Warn("Live Azure price refresh failed; keeping bundled price",
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
			Provider:        ProviderAzure,
			Region:          region,
			Name:            fmt.Sprintf("Standard_Custom_%.0f_%.0f", math.Ceil(vCPU), math.Ceil(memGB)),
			VCPU:            math.Ceil(vCPU),
			MemoryGB:        math.Ceil(memGB),
			OnDemandPrice:   0.038*vCPU + 0.0048*memGB,
			SpotPrice:       (0.038*vCPU + 0.0048*memGB) * 0.32,
			CarbonIntensity: carbon,
		})
	}
	return matched, nil
}
