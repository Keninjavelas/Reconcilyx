package carbon_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/reconcilyx/operator/internal/carbon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestElectricityMaps_LiveIntensity(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-em-key", r.Header.Get("auth-token"))
		assert.Equal(t, "FR", r.URL.Query().Get("zone"))
		doc := map[string]interface{}{
			"carbonIntensity": 72.5,
			"zone":            "FR",
		}
		b, _ := json.Marshal(doc)
		_, _ = w.Write(b)
	}))
	defer ts.Close()

	p := carbon.NewElectricityMapsProvider("test-em-key")
	p.BaseURL = ts.URL
	p.HTTPClient = ts.Client()

	it, err := p.IntensityForRegion(context.Background(), "FR")
	require.NoError(t, err)
	assert.InDelta(t, 72.5, it.Value, 0.001)
	assert.Equal(t, "electricitymaps", it.Source)
	assert.Equal(t, "FR", it.Region)
}

func TestElectricityMaps_NoKeyUsesStaticFallback(t *testing.T) {
	p := carbon.NewElectricityMapsProvider("")
	it, err := p.IntensityForRegion(context.Background(), "aws-us-east-1")
	require.NoError(t, err)
	assert.InDelta(t, 380, it.Value, 1)
	assert.Equal(t, "static", it.Source)
}

func TestElectricityMaps_HTTPErrorFallsBack(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer ts.Close()

	p := carbon.NewElectricityMapsProvider("test-em-key")
	p.BaseURL = ts.URL
	p.HTTPClient = ts.Client()

	it, err := p.IntensityForRegion(context.Background(), "DE")
	require.NoError(t, err, "fallback must not error")
	assert.Equal(t, "static", it.Source, "429 must trigger static fallback")
}

func TestWattTime_LiveIntensity(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.Header.Get("Authorization"), "Bearer ")
		assert.Equal(t, "CAISO_NORTH", r.URL.Query().Get("region"))
		doc := map[string]interface{}{
			"value":  428.1,
			"region": "CAISO_NORTH",
		}
		b, _ := json.Marshal(doc)
		_, _ = w.Write(b)
	}))
	defer ts.Close()

	p := carbon.NewWattTimeProvider("wt-key")
	p.BaseURL = ts.URL
	p.HTTPClient = ts.Client()

	it, err := p.IntensityForRegion(context.Background(), "CAISO_NORTH")
	require.NoError(t, err)
	assert.InDelta(t, 428.1, it.Value, 0.001)
	assert.Equal(t, "watttime", it.Source)
}

func TestWattTime_BasicAuthLive(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.Header.Get("Authorization"), "Basic ")
		doc := map[string]interface{}{"value": 150.0, "region": "PJM_NJ"}
		b, _ := json.Marshal(doc)
		_, _ = w.Write(b)
	}))
	defer ts.Close()

	p := carbon.NewWattTimeProviderWithCreds("user", "pass")
	p.BaseURL = ts.URL
	p.HTTPClient = ts.Client()

	it, err := p.IntensityForRegion(context.Background(), "PJM_NJ")
	require.NoError(t, err)
	assert.InDelta(t, 150.0, it.Value, 0.001)
}

func TestWattTime_NoCredsFallsBack(t *testing.T) {
	p := carbon.NewWattTimeProviderWithCreds("", "")
	it, err := p.IntensityForRegion(context.Background(), "gcp-europe-west1")
	require.NoError(t, err)
	assert.Equal(t, "static", it.Source)
}

func TestCachedCarbonProvider(t *testing.T) {
	call := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		doc := map[string]interface{}{
			"carbonIntensity": 100.0 + float64(call),
			"zone":            "NL",
		}
		b, _ := json.Marshal(doc)
		_, _ = w.Write(b)
	}))
	defer ts.Close()

	raw := carbon.NewElectricityMapsProvider("key")
	raw.BaseURL = ts.URL
	raw.HTTPClient = ts.Client()

	cached := carbon.NewCachedCarbonProvider(raw, 5*time.Minute)
	ctx := context.Background()
	it1, err := cached.IntensityForRegion(ctx, "NL")
	require.NoError(t, err)
	it2, err := cached.IntensityForRegion(ctx, "NL")
	require.NoError(t, err)
	assert.Equal(t, it1.Value, it2.Value)
	assert.Equal(t, 1, call, "second call must hit cache")
}

func TestFactory_StaticDefault(t *testing.T) {
	t.Setenv("RECONCILYX_CARBON_PROVIDER", "static")
	cfg := carbon.CarbonConfigFromEnv()
	p := carbon.NewCarbonProvider(cfg)
	it, err := p.IntensityForRegion(context.Background(), "azure-westeurope")
	require.NoError(t, err)
	assert.Equal(t, "static", it.Source)
	assert.InDelta(t, 280, it.Value, 5)
}

func TestFactory_ElectricityMapsFromEnv(t *testing.T) {
	t.Setenv("RECONCILYX_CARBON_PROVIDER", "electricitymaps")
	t.Setenv("ELECTRICITY_MAPS_API_KEY", "em-env-key")
	cfg := carbon.CarbonConfigFromEnv()
	assert.Equal(t, "electricitymaps", cfg.Provider)
	assert.Equal(t, "em-env-key", cfg.ElectricityMaps.APIKey)
}

func TestFactory_WattTimeFromEnv(t *testing.T) {
	t.Setenv("RECONCILYX_CARBON_PROVIDER", "watttime")
	t.Setenv("WATTTIME_API_KEY", "wt-env-key")
	cfg := carbon.CarbonConfigFromEnv()
	assert.Equal(t, "watttime", cfg.Provider)
	assert.Equal(t, "wt-env-key", cfg.WattTime.APIKey)
}
