package cloud_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/reconcilyx/operator/internal/cloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAWS_LiveOnDemand(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		doc := map[string]interface{}{
			"products": map[string]interface{}{
				"SKU-001": map[string]interface{}{
					"attributes": map[string]interface{}{
						"instanceType": "c6i.large",
					},
				},
			},
			"terms": map[string]interface{}{
				"OnDemand": map[string]interface{}{
					"SKU-001": map[string]interface{}{
						"SKU-001.JRTCKXETXF": map[string]interface{}{
							"priceDimensions": map[string]interface{}{
								"SKU-001.JRTCKXETXF.6YS6EN2CT7": map[string]interface{}{
									"pricePerUnit": map[string]string{
										"USD": "0.0855",
									},
								},
							},
						},
					},
				},
			},
		}
		b, _ := json.Marshal(doc)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	defer ts.Close()

	p := cloud.NewAWSPricingProvider()
	p.BaseURL = ts.URL + "/%s"
	p.HTTPClient = ts.Client()

	got, err := p.OnDemandPrice(context.Background(), "c6i.large", "us-east-1")
	require.NoError(t, err)
	assert.InDelta(t, 0.0855, got, 0.0001, "live AWS price should match server response")
}

func TestAWS_UnreachableFallsBackToStatic(t *testing.T) {
	p := cloud.NewAWSPricingProvider()
	p.BaseURL = "http://127.0.0.1:1/invalid/%s"
	p.HTTPClient = &http.Client{Timeout: 50 * time.Millisecond}

	got, err := p.OnDemandPrice(context.Background(), "t4g.large", "us-east-1")
	require.NoError(t, err, "fallback must not surface errors")
	assert.InDelta(t, 0.0672, got, 0.0001, "fallback should use static rate for known instance")
}

func TestAWS_ListInstancesWithLivePricing(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		instName := "c6i.large"
		if callCount == 3 {
			instName = "m6i.large"
		}
		price := fmt.Sprintf("%.4f", 0.08+float64(callCount)*0.001)
		doc := map[string]interface{}{
			"products": map[string]interface{}{
				"SKU": map[string]interface{}{
					"attributes": map[string]interface{}{
						"instanceType": instName,
					},
				},
			},
			"terms": map[string]interface{}{
				"OnDemand": map[string]interface{}{
					"SKU": map[string]interface{}{
						"t1": map[string]interface{}{
							"priceDimensions": map[string]interface{}{
								"d1": map[string]interface{}{
									"pricePerUnit": map[string]string{"USD": price},
								},
							},
						},
					},
				},
			},
		}
		b, _ := json.Marshal(doc)
		_, _ = w.Write(b)
	}))
	defer ts.Close()

	p := cloud.NewAWSPricingProvider()
	p.BaseURL = ts.URL + "/%s"
	p.HTTPClient = ts.Client()

	insts, err := p.ListMatchingInstances(context.Background(), 2.0, 4.0, "us-east-1")
	require.NoError(t, err)
	require.NotEmpty(t, insts)
	for _, inst := range insts {
		assert.Greater(t, inst.OnDemandPrice, 0.0)
		assert.Greater(t, inst.SpotPrice, 0.0)
		assert.Less(t, inst.SpotPrice, inst.OnDemandPrice)
	}
}

func TestAzure_LiveOnDemand(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		doc := map[string]interface{}{
			"Items": []interface{}{
				map[string]interface{}{
					"armSkuName":      "Standard_D2s_v5",
					"armRegionName":   "eastus",
					"retailPrice":     0.0972,
					"isPrimaryMeterRegion": true,
					"unitOfMeasure":   "1/Hour",
				},
			},
		}
		b, _ := json.Marshal(doc)
		_, _ = w.Write(b)
	}))
	defer ts.Close()

	p := cloud.NewAzurePricingProvider()
	p.BaseURL = ts.URL
	p.HTTPClient = ts.Client()

	got, err := p.OnDemandPrice(context.Background(), "Standard_D2s_v5", "eastus")
	require.NoError(t, err)
	assert.InDelta(t, 0.0972, got, 0.0001)
}

func TestAzure_HTTP500FallsBack(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	p := cloud.NewAzurePricingProvider()
	p.BaseURL = ts.URL
	p.HTTPClient = ts.Client()

	got, err := p.OnDemandPrice(context.Background(), "Standard_B2s", "eastus")
	require.NoError(t, err)
	assert.InDelta(t, 0.0416, got, 0.0001, "500 must trigger static fallback")
}

func TestGCP_LiveOnDemand(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		doc := map[string]interface{}{
			"skus": []interface{}{
				map[string]interface{}{
					"skuId":       "ABC",
					"description": "N2 Standard 2 VCPU running in us-central1 (e2-standard-2)",
					"category": map[string]interface{}{
						"resourceFamily": "Compute",
						"usageType":      "OnDemand",
					},
					"serviceRegions": []string{"us-central1", "us-east1"},
					"pricingInfo": []interface{}{
						map[string]interface{}{
							"pricingExpression": map[string]interface{}{
								"displayQuantity": 1,
								"tieredRates": []interface{}{
									map[string]interface{}{
										"unitPrice": map[string]interface{}{
											"currencyCode": "USD",
											"units":        "0",
											"nanos":        67_000_000,
										},
									},
								},
							},
						},
					},
				},
			},
		}
		b, _ := json.Marshal(doc)
		_, _ = w.Write(b)
	}))
	defer ts.Close()

	p := cloud.NewLiveGCPPricingProvider("test-key")
	p.BaseURL = ts.URL
	p.HTTPClient = ts.Client()

	got, err := p.OnDemandPrice(context.Background(), "e2-standard-2", "us-central1")
	require.NoError(t, err)
	assert.InDelta(t, 0.067, got, 0.0001)
}

func TestGCP_NoKeyUsesStatic(t *testing.T) {
	p := cloud.NewGCPPricingProvider()
	got, err := p.OnDemandPrice(context.Background(), "e2-standard-4", "us-central1")
	require.NoError(t, err)
	assert.InDelta(t, 0.134, got, 0.0001)
}

func TestCachedPricingProvider_CachesSpotAndOD(t *testing.T) {
	call := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		doc := map[string]interface{}{
			"Items": []interface{}{
				map[string]interface{}{
					"armSkuName":      "Standard_D4s_v5",
					"armRegionName":   "eastus",
					"retailPrice":     0.19 + float64(call)*0.001,
					"isPrimaryMeterRegion": true,
				},
			},
		}
		b, _ := json.Marshal(doc)
		_, _ = w.Write(b)
	}))
	defer ts.Close()

	raw := cloud.NewAzurePricingProvider()
	raw.BaseURL = ts.URL
	raw.HTTPClient = ts.Client()

	cached := cloud.NewCachedPricingProvider(raw, 5*time.Minute)
	ctx := context.Background()

	a1, err := cached.OnDemandPrice(ctx, "Standard_D4s_v5", "eastus")
	require.NoError(t, err)
	a2, err := cached.OnDemandPrice(ctx, "Standard_D4s_v5", "eastus")
	require.NoError(t, err)
	assert.Equal(t, a1, a2, "second call must be a cache hit")
	assert.Equal(t, 1, call, "only one HTTP call for two OnDemand reads")

	s1, err := cached.SpotPrice(ctx, "Standard_D4s_v5", "eastus")
	require.NoError(t, err)
	s2, err := cached.SpotPrice(ctx, "Standard_D4s_v5", "eastus")
	require.NoError(t, err)
	assert.Equal(t, s1, s2, "spot price must be cached too")
}

func TestFactory_DefaultEnvBuildsAllProviders(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("RECONCILYX_PRICING_GCP_API_KEY", "")
	t.Setenv("AZURE_SUBSCRIPTION_ID", "")

	cfg := cloud.PricingConfigFromEnv()
	providers := cloud.AllPricingProviders(cfg)
	require.Len(t, providers, 3)
	names := make(map[cloud.ProviderName]bool)
	for _, p := range providers {
		names[p.Name()] = true
	}
	assert.True(t, names[cloud.ProviderAWS])
	assert.True(t, names[cloud.ProviderGCP])
	assert.True(t, names[cloud.ProviderAzure])
}

func TestFactory_DisabledProviderOmitted(t *testing.T) {
	t.Setenv("RECONCILYX_PRICING_GCP_ENABLED", "false")
	cfg := cloud.PricingConfigFromEnv()
	providers := cloud.AllPricingProviders(cfg)
	found := false
	for _, p := range providers {
		if p.Name() == cloud.ProviderGCP {
			found = true
		}
	}
	assert.False(t, found, "GCP must not be present when disabled")
}
