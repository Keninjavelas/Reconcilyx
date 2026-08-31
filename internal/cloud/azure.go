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
	azureRetailPricesURL = "https://prices.azure.com/api/retail/prices"
)

type AzurePricingProvider struct {
	HTTPClient     *http.Client
	BaseURL        string
	subscriptionId string
	fallback       *staticAzureCatalog
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
		fallback:   newStaticAzureCatalog(),
	}
}

func NewLiveAzurePricingProvider(subscriptionId string) *AzurePricingProvider {
	p := NewAzurePricingProvider()
	p.subscriptionId = subscriptionId
	return p
}

func (az *AzurePricingProvider) Name() ProviderName { return ProviderAzure }

func carbonForAzure(region string, cmap map[string]float64) float64 {
	if c, ok := cmap[region]; ok {
		return c
	}
	return 290.0
}

func (az *AzurePricingProvider) fetchOnDemandLive(ctx context.Context, instanceType, region string) (float64, error) {
	if az.HTTPClient == nil || az.BaseURL == "" {
		return 0, fmt.Errorf("live Azure pricing disabled (no client/baseURL)")
	}
	u, _ := url.Parse(az.BaseURL)
	q := u.Query()
	filter := fmt.Sprintf("armRegionName eq '%s' and serviceName eq 'Virtual Machines' and armSkuName eq '%s' and priceType eq 'Consumption'", region, instanceType)
	q.Set("$filter", filter)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, err
	}
	if az.subscriptionId != "" {
		req.Header.Set("X-Subscription-Id", az.subscriptionId)
	}
	resp, err := az.HTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("azure retail prices api status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return 0, err
	}
	var doc struct {
		Items []struct {
			ArmSkuName      string  `json:"armSkuName"`
			ArmRegionName   string  `json:"armRegionName"`
			RetailPrice     float64 `json:"retailPrice"`
			IsPrimaryMeterRegion bool `json:"isPrimaryMeterRegion"`
			UnitOfMeasure   string  `json:"unitOfMeasure"`
		} `json:"Items"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return 0, err
	}
	best := 0.0
	for _, it := range doc.Items {
		if it.ArmSkuName != instanceType {
			continue
		}
		if it.ArmRegionName != region && it.ArmRegionName != "" {
			continue
		}
		if it.RetailPrice <= 0 {
			continue
		}
		if best == 0 || (it.IsPrimaryMeterRegion && best > 0) || it.RetailPrice < best {
			best = it.RetailPrice
		}
	}
	if best <= 0 {
		return 0, fmt.Errorf("azure %s/%s not found in retail prices", region, instanceType)
	}
	return best, nil
}

func (az *AzurePricingProvider) OnDemandPrice(ctx context.Context, instanceType, region string) (float64, error) {
	if az.HTTPClient != nil && az.BaseURL != "" {
		if v, err := az.fetchOnDemandLive(ctx, instanceType, region); err == nil && v > 0 {
			return v, nil
		}
	}
	if rate, ok := az.fallback.rates[instanceType]; ok {
		return rate, nil
	}
	return 0.085, nil
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
	if az.HTTPClient != nil && az.BaseURL != "" {
		for i := range catalog {
			live, err := az.fetchOnDemandLive(ctx, catalog[i].Name, region)
			if err == nil && live > 0 {
				catalog[i].OnDemandPrice = live
				catalog[i].SpotPrice = live * 0.32
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
