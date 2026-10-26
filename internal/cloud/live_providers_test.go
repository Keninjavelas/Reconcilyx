package cloud_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/reconcilyx/operator/internal/cloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeAWSPrice(t *testing.T, w http.ResponseWriter, instance, price string) {
	t.Helper()
	item := map[string]any{
		"product": map[string]any{
			"attributes": map[string]any{"instanceType": instance},
		},
		"terms": map[string]any{
			"OnDemand": map[string]any{
				"SKU": map[string]any{
					"TERM": map[string]any{
						"priceDimensions": map[string]any{
							"DIM": map[string]any{
								"pricePerUnit": map[string]string{"USD": price},
							},
						},
					},
				},
			},
		},
	}
	payload, err := json.Marshal(item)
	require.NoError(t, err)
	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
		"PriceList": []string{string(payload)},
	}))
}

func awsInstanceFromRequest(t *testing.T, r *http.Request) string {
	t.Helper()
	var req struct {
		ServiceCode string `json:"ServiceCode"`
		Filters     []struct {
			Field string `json:"Field"`
			Value string `json:"Value"`
		} `json:"Filters"`
	}
	require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
	assert.Equal(t, "AmazonEC2", req.ServiceCode)
	for _, f := range req.Filters {
		if f.Field == "instanceType" {
			return f.Value
		}
	}
	t.Fatalf("instanceType filter missing from AWS request")
	return ""
}

func TestAWS_LiveOnDemand(t *testing.T) {
	var gotAuth, gotTarget, gotContentType string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		gotAuth = r.Header.Get("Authorization")
		gotTarget = r.Header.Get("X-Amz-Target")
		gotContentType = r.Header.Get("Content-Type")
		instance := awsInstanceFromRequest(t, r)
		writeAWSPrice(t, w, instance, "0.0855")
	}))
	defer ts.Close()

	p := cloud.NewLiveAWSPricingProvider("AKIDEXAMPLE", "secret", "")
	p.BaseURL = ts.URL
	p.HTTPClient = ts.Client()

	got, err := p.OnDemandPrice(context.Background(), "c6i.large", "us-east-1")
	require.NoError(t, err)
	assert.InDelta(t, 0.0855, got, 0.0001)
	assert.True(t, strings.HasPrefix(gotAuth, "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/"), "request must be SigV4 signed: %q", gotAuth)
	assert.Equal(t, "AWSPriceListService.GetProducts", gotTarget)
	assert.Equal(t, "application/x-amz-json-1.1", gotContentType)
	assert.True(t, p.Live())
}

func TestAWS_UnreachableFallsBackToStatic(t *testing.T) {
	p := cloud.NewLiveAWSPricingProvider("AKIDEXAMPLE", "secret", "")
	p.BaseURL = "http://127.0.0.1:1/invalid"
	p.HTTPClient = &http.Client{Timeout: 50 * time.Millisecond}
	p.Retry = cloud.RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond}

	got, err := p.OnDemandPrice(context.Background(), "t4g.large", "us-east-1")
	require.NoError(t, err, "fallback must not surface errors")
	assert.InDelta(t, 0.0672, got, 0.0001, "fallback should use static rate for known instance")
}

func TestAWS_RetriesTransientFailures(t *testing.T) {
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		writeAWSPrice(t, w, "c6i.large", "0.0911")
	}))
	defer ts.Close()

	p := cloud.NewLiveAWSPricingProvider("AKIDEXAMPLE", "secret", "")
	p.BaseURL = ts.URL
	p.HTTPClient = ts.Client()
	p.Retry = cloud.RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond}

	got, err := p.OnDemandPrice(context.Background(), "c6i.large", "us-east-1")
	require.NoError(t, err)
	assert.InDelta(t, 0.0911, got, 0.0001)
	assert.Equal(t, 3, calls, "two transient failures must be retried")
}

func TestAWS_RetriesThrottling400(t *testing.T) {
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"__type":"ThrottlingException","message":"Rate exceeded"}`))
			return
		}
		writeAWSPrice(t, w, "c6i.large", "0.0922")
	}))
	defer ts.Close()

	p := cloud.NewLiveAWSPricingProvider("AKIDEXAMPLE", "secret", "")
	p.BaseURL = ts.URL
	p.HTTPClient = ts.Client()
	p.Retry = cloud.RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond}

	got, err := p.OnDemandPrice(context.Background(), "c6i.large", "us-east-1")
	require.NoError(t, err)
	assert.InDelta(t, 0.0922, got, 0.0001)
	assert.Equal(t, 2, calls, "AWS 400 ThrottlingException must be retried")
}

func TestAWS_ListInstancesWithLivePricing(t *testing.T) {
	livePrices := map[string]string{
		"t4g.medium": "0.0350",
		"c6i.large":  "0.0900",
		"m6i.large":  "0.1010",
		"c6i.xlarge": "0.1800",
		"m6i.xlarge": "0.2010",
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		instance := awsInstanceFromRequest(t, r)
		price, ok := livePrices[instance]
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		writeAWSPrice(t, w, instance, price)
	}))
	defer ts.Close()

	p := cloud.NewLiveAWSPricingProvider("AKIDEXAMPLE", "secret", "")
	p.BaseURL = ts.URL
	p.HTTPClient = ts.Client()

	insts, err := p.ListMatchingInstances(context.Background(), 2.0, 4.0, "us-east-1")
	require.NoError(t, err)
	require.NotEmpty(t, insts)
	byName := make(map[string]cloud.InstanceType)
	for _, inst := range insts {
		byName[inst.Name] = inst
		assert.Greater(t, inst.OnDemandPrice, 0.0)
		assert.Greater(t, inst.SpotPrice, 0.0)
		assert.Less(t, inst.SpotPrice, inst.OnDemandPrice)
	}
	assert.InDelta(t, 0.0900, byName["c6i.large"].OnDemandPrice, 0.0001)
	assert.InDelta(t, 0.0900*0.35, byName["c6i.large"].SpotPrice, 0.0001)
}

func TestAWS_UnknownInstanceWithoutFallbackErrors(t *testing.T) {
	p := cloud.NewAWSPricingProvider()
	_, err := p.OnDemandPrice(context.Background(), "x9.mega", "us-east-1")
	require.Error(t, err)
	assert.False(t, p.Live())
}

func TestAzure_LiveOnDemand(t *testing.T) {
	var gotFilter string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotFilter = r.URL.Query().Get("$filter")
		doc := map[string]any{
			"Items": []any{
				map[string]any{
					"armSkuName":           "Standard_D2s_v5",
					"armRegionName":        "eastus",
					"retailPrice":          0.0972,
					"isPrimaryMeterRegion": true,
					"unitOfMeasure":        "1 Hour",
				},
			},
		}
		_, _ = w.Write(mustJSON(t, doc))
	}))
	defer ts.Close()

	p := cloud.NewLiveAzurePricingProvider()
	p.BaseURL = ts.URL
	p.HTTPClient = ts.Client()

	got, err := p.OnDemandPrice(context.Background(), "Standard_D2s_v5", "eastus")
	require.NoError(t, err)
	assert.InDelta(t, 0.0972, got, 0.0001)
	assert.Contains(t, gotFilter, "armSkuName eq 'Standard_D2s_v5'")
	assert.Contains(t, gotFilter, "armRegionName eq 'eastus'")
	assert.True(t, p.Live())
}

func TestAzure_LiveOnDemandIgnoresSpotLowPriorityAndWindowsMeters(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		items := []any{
			map[string]any{"armSkuName": "Standard_F2s_v2", "armRegionName": "eastus", "skuName": "F2s v2 Low Priority", "meterName": "F2s v2 Low Priority", "productName": "Virtual Machines FSv2 Series", "retailPrice": 0.0169, "isPrimaryMeterRegion": true},
			map[string]any{"armSkuName": "Standard_F2s_v2", "armRegionName": "eastus", "skuName": "F2s v2 Spot", "meterName": "F2s v2 Spot", "productName": "Virtual Machines FSv2 Series", "retailPrice": 0.018612, "isPrimaryMeterRegion": true},
			map[string]any{"armSkuName": "Standard_F2s_v2", "armRegionName": "eastus", "skuName": "F2s v2", "meterName": "F2s v2", "productName": "Virtual Machines FSv2 Series Windows", "retailPrice": 0.177, "isPrimaryMeterRegion": true},
			map[string]any{"armSkuName": "Standard_F2s_v2", "armRegionName": "eastus", "skuName": "F2s v2", "meterName": "F2s v2", "productName": "Virtual Machines FSv2 Series", "retailPrice": 0.0846, "isPrimaryMeterRegion": true},
		}
		_, _ = w.Write(mustJSON(t, map[string]any{"Items": items}))
	}))
	defer ts.Close()

	p := cloud.NewLiveAzurePricingProvider()
	p.BaseURL = ts.URL
	p.HTTPClient = ts.Client()

	got, err := p.OnDemandPrice(context.Background(), "Standard_F2s_v2", "eastus")
	require.NoError(t, err)
	assert.InDelta(t, 0.0846, got, 0.0001, "Linux on-demand must win over Spot, Low Priority, and Windows meters")
}

func TestAzure_HTTP500FallsBack(t *testing.T) {
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	p := cloud.NewLiveAzurePricingProvider()
	p.BaseURL = ts.URL
	p.HTTPClient = ts.Client()
	p.Retry = cloud.RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond}

	got, err := p.OnDemandPrice(context.Background(), "Standard_B2s", "eastus")
	require.NoError(t, err)
	assert.InDelta(t, 0.0416, got, 0.0001, "500 must trigger static fallback")
	assert.Equal(t, 2, calls, "transient 500s must be retried before fallback")
}

func TestAzure_RetriesOn429(t *testing.T) {
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		doc := map[string]any{"Items": []any{map[string]any{
			"armSkuName": "Standard_D2s_v5", "armRegionName": "eastus", "retailPrice": 0.0972,
		}}}
		_, _ = w.Write(mustJSON(t, doc))
	}))
	defer ts.Close()

	p := cloud.NewLiveAzurePricingProvider()
	p.BaseURL = ts.URL
	p.HTTPClient = ts.Client()
	p.Retry = cloud.RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond}

	got, err := p.OnDemandPrice(context.Background(), "Standard_D2s_v5", "eastus")
	require.NoError(t, err)
	assert.InDelta(t, 0.0972, got, 0.0001)
	assert.Equal(t, 2, calls)
}

func TestGCP_LiveOnDemand(t *testing.T) {
	pages := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pages++
		q := r.URL.Query()
		assert.Equal(t, "test-key", q.Get("key"))
		assert.Empty(t, q.Get("filter"), "v1 Billing Catalog does not support filter")
		assert.Equal(t, "USD", q.Get("currencyCode"))

		coreSKU := map[string]any{
			"skuId":       "CORE",
			"description": "E2 Instance Core running in Americas",
			"category": map[string]any{
				"resourceFamily": "Compute",
				"usageType":      "OnDemand",
			},
			"serviceRegions": []string{"us-central1", "us-east1"},
			"pricingInfo": []any{map[string]any{
				"pricingExpression": map[string]any{
					"usageUnit": "h",
					"tieredRates": []any{map[string]any{
						"unitPrice": map[string]any{"currencyCode": "USD", "units": "0", "nanos": 21_500_000},
					}},
				},
			}},
		}
		ramSKU := map[string]any{
			"skuId":       "RAM",
			"description": "E2 Instance Ram running in Americas",
			"category": map[string]any{
				"resourceFamily": "Compute",
				"usageType":      "OnDemand",
			},
			"serviceRegions": []string{"us-central1", "us-east1"},
			"pricingInfo": []any{map[string]any{
				"pricingExpression": map[string]any{
					"usageUnit": "h",
					"tieredRates": []any{map[string]any{
						"unitPrice": map[string]any{"currencyCode": "USD", "units": "0", "nanos": 3_000_000},
					}},
				},
			}},
		}
		if pages == 1 {
			_, _ = w.Write(mustJSON(t, map[string]any{
				"skus":          []any{coreSKU},
				"nextPageToken": "page-2",
			}))
			return
		}
		_, _ = w.Write(mustJSON(t, map[string]any{"skus": []any{ramSKU}}))
	}))
	defer ts.Close()

	p := cloud.NewLiveGCPPricingProvider("test-key")
	p.BaseURL = ts.URL
	p.HTTPClient = ts.Client()

	got, err := p.OnDemandPrice(context.Background(), "e2-standard-2", "us-central1")
	require.NoError(t, err)
	// 2 vCPU * $0.0215 + 8 GB * $0.003 = $0.067
	assert.InDelta(t, 0.067, got, 0.0001)
	assert.Equal(t, 2, pages, "the client must follow nextPageToken")
	assert.True(t, p.Live())
}

func TestGCP_NoKeyUsesStatic(t *testing.T) {
	p := cloud.NewGCPPricingProvider()
	got, err := p.OnDemandPrice(context.Background(), "e2-standard-4", "us-central1")
	require.NoError(t, err)
	assert.InDelta(t, 0.134, got, 0.0001)
	assert.False(t, p.Live())
}

func TestStaticProvidersDoNotCallNetwork(t *testing.T) {
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	aws := cloud.NewAWSPricingProvider()
	aws.BaseURL = ts.URL
	aws.HTTPClient = ts.Client()
	_, err := aws.OnDemandPrice(context.Background(), "c6i.large", "us-east-1")
	require.NoError(t, err)

	gcp := cloud.NewGCPPricingProvider()
	gcp.BaseURL = ts.URL
	gcp.HTTPClient = ts.Client()
	_, err = gcp.OnDemandPrice(context.Background(), "e2-standard-2", "us-central1")
	require.NoError(t, err)

	azure := cloud.NewAzurePricingProvider()
	azure.BaseURL = ts.URL
	azure.HTTPClient = ts.Client()
	_, err = azure.OnDemandPrice(context.Background(), "Standard_B2s", "eastus")
	require.NoError(t, err)

	assert.Equal(t, 0, calls, "static providers must never hit the network")
}

func TestCachedPricingProvider_CachesSpotAndOD(t *testing.T) {
	call := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		doc := map[string]any{
			"Items": []any{
				map[string]any{
					"armSkuName":           "Standard_D4s_v5",
					"armRegionName":        "eastus",
					"retailPrice":          0.19 + float64(call)*0.001,
					"isPrimaryMeterRegion": true,
				},
			},
		}
		_, _ = w.Write(mustJSON(t, doc))
	}))
	defer ts.Close()

	raw := cloud.NewLiveAzurePricingProvider()
	raw.BaseURL = ts.URL
	raw.HTTPClient = ts.Client()

	cached := cloud.NewCachedPricingProvider(raw, 5*time.Minute)
	assert.True(t, cached.Live())
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
	assert.Equal(t, 2, call, "spot pricing performs one additional lookup and then caches")
}

func TestFactory_DefaultEnvBuildsAllProviders(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("RECONCILYX_PRICING_GCP_API_KEY", "")
	t.Setenv("GCP_BILLING_API_KEY", "")
	t.Setenv("RECONCILYX_PRICING_AWS_ENABLED", "true")
	t.Setenv("RECONCILYX_PRICING_GCP_ENABLED", "true")
	t.Setenv("RECONCILYX_PRICING_AZURE_ENABLED", "true")

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

func TestFactory_LiveModesFromCredentials(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIDEXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
	t.Setenv("AWS_SESSION_TOKEN", "token")
	t.Setenv("RECONCILYX_PRICING_GCP_API_KEY", "gcp-key")
	t.Setenv("RECONCILYX_PRICING_AWS_ENABLED", "true")
	t.Setenv("RECONCILYX_PRICING_GCP_ENABLED", "true")
	t.Setenv("RECONCILYX_PRICING_AZURE_ENABLED", "true")

	cfg := cloud.PricingConfigFromEnv()
	assert.Equal(t, "token", cfg.AWSSessionToken)
	for _, p := range cloud.AllPricingProviders(cfg) {
		assert.True(t, p.Live(), "%s should use live pricing with configured credentials", p.Name())
	}
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

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}
