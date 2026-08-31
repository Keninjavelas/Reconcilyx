package cloud

import (
	"bytes"
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
	awsPriceListURL = "https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonEC2/current/%s/index.json"
)

type AWSPricingProvider struct {
	HTTPClient *http.Client
	BaseURL    string
	apiKey     string
	enableLive bool
	fallback   *staticAWSCatalog
}

type staticAWSCatalog struct {
	rates   map[string]float64
	catalog []InstanceType
	carbon  map[string]float64
}

func newStaticAWSCatalog() *staticAWSCatalog {
	return &staticAWSCatalog{
		rates: map[string]float64{
			"c6i.large":   0.085,
			"c6i.xlarge":  0.170,
			"m6i.large":   0.096,
			"m6i.xlarge":  0.192,
			"t4g.medium":  0.0336,
			"t4g.large":   0.0672,
		},
		carbon: map[string]float64{
			"us-east-1":      385.0,
			"us-west-2":      110.0,
			"eu-west-1":      260.0,
			"ap-southeast-1": 420.0,
		},
	}
}

func NewAWSPricingProvider() *AWSPricingProvider {
	return &AWSPricingProvider{
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
		BaseURL:    awsPriceListURL,
		enableLive: false,
		fallback:   newStaticAWSCatalog(),
	}
}

func NewLiveAWSPricingProvider(apiKey string) *AWSPricingProvider {
	p := NewAWSPricingProvider()
	p.enableLive = true
	p.apiKey = apiKey
	return p
}

func (a *AWSPricingProvider) Name() ProviderName { return ProviderAWS }

func carbonForAWS(region string, cmap map[string]float64) float64 {
	if c, ok := cmap[region]; ok {
		return c
	}
	return 300.0
}

func (a *AWSPricingProvider) fetchOnDemandLive(ctx context.Context, instanceType, region string) (float64, error) {
	if a.HTTPClient == nil || a.BaseURL == "" {
		return 0, fmt.Errorf("live AWS pricing disabled (no client or base URL)")
	}
	endpoint := fmt.Sprintf(a.BaseURL, url.PathEscape(region))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	if a.apiKey != "" {
		req.Header.Set("X-Api-Key", a.apiKey)
	}
	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("pricing api status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 20*1024*1024))
	if err != nil {
		return 0, err
	}
	var doc struct {
		Products map[string]struct {
			Attributes struct {
				InstanceType string `json:"instanceType"`
			} `json:"attributes"`
		} `json:"products"`
		Terms struct {
			OnDemand map[string]map[string]struct {
				PriceDimensions map[string]struct {
					PricePerUnit map[string]string `json:"pricePerUnit"`
				} `json:"priceDimensions"`
			} `json:"OnDemand"`
		} `json:"terms"`
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&doc); err != nil {
		return 0, err
	}
	for sku, prod := range doc.Products {
		if prod.Attributes.InstanceType != instanceType {
			continue
		}
		terms, ok := doc.Terms.OnDemand[sku]
		if !ok {
			continue
		}
		for _, term := range terms {
			for _, dim := range term.PriceDimensions {
				if price, ok := dim.PricePerUnit["USD"]; ok {
					var v float64
					if _, err := fmt.Sscanf(price, "%f", &v); err == nil && v > 0 {
						return v, nil
					}
				}
			}
		}
	}
	return 0, fmt.Errorf("instance %s not found in AWS offer index", instanceType)
}

func (a *AWSPricingProvider) OnDemandPrice(ctx context.Context, instanceType, region string) (float64, error) {
	if a.HTTPClient != nil && a.BaseURL != "" {
		if v, err := a.fetchOnDemandLive(ctx, instanceType, region); err == nil && v > 0 {
			return v, nil
		}
	}
	if rate, ok := a.fallback.rates[instanceType]; ok {
		return rate, nil
	}
	return 0.10, nil
}

func (a *AWSPricingProvider) SpotPrice(ctx context.Context, instanceType, region string) (float64, error) {
	od, err := a.OnDemandPrice(ctx, instanceType, region)
	if err != nil {
		return 0, err
	}
	return od * 0.35, nil
}

func (a *AWSPricingProvider) ListMatchingInstances(ctx context.Context, vCPU, memGB float64, region string) ([]InstanceType, error) {
	carbon := carbonForAWS(region, a.fallback.carbon)
	catalog := []InstanceType{
		{Provider: ProviderAWS, Region: region, Name: "t4g.medium", VCPU: 2.0, MemoryGB: 4.0, OnDemandPrice: 0.0336, SpotPrice: 0.0118, CarbonIntensity: carbon},
		{Provider: ProviderAWS, Region: region, Name: "c6i.large", VCPU: 2.0, MemoryGB: 4.0, OnDemandPrice: 0.0850, SpotPrice: 0.0298, CarbonIntensity: carbon},
		{Provider: ProviderAWS, Region: region, Name: "m6i.large", VCPU: 2.0, MemoryGB: 8.0, OnDemandPrice: 0.0960, SpotPrice: 0.0336, CarbonIntensity: carbon},
		{Provider: ProviderAWS, Region: region, Name: "c6i.xlarge", VCPU: 4.0, MemoryGB: 8.0, OnDemandPrice: 0.1700, SpotPrice: 0.0595, CarbonIntensity: carbon},
		{Provider: ProviderAWS, Region: region, Name: "m6i.xlarge", VCPU: 4.0, MemoryGB: 16.0, OnDemandPrice: 0.1920, SpotPrice: 0.0672, CarbonIntensity: carbon},
	}
	if a.HTTPClient != nil && a.BaseURL != "" {
		for i := range catalog {
			live, err := a.fetchOnDemandLive(ctx, catalog[i].Name, region)
			if err == nil && live > 0 {
				catalog[i].OnDemandPrice = live
				catalog[i].SpotPrice = live * 0.35
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
			Provider:        ProviderAWS,
			Region:          region,
			Name:            fmt.Sprintf("custom-aws-%.0fvcpu-%.0fgb", math.Ceil(vCPU), math.Ceil(memGB)),
			VCPU:            math.Ceil(vCPU),
			MemoryGB:        math.Ceil(memGB),
			OnDemandPrice:   0.04*vCPU + 0.005*memGB,
			SpotPrice:       (0.04*vCPU + 0.005*memGB) * 0.35,
			CarbonIntensity: carbon,
		})
	}
	return matched, nil
}
