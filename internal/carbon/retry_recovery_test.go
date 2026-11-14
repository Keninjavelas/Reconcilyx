package carbon_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/reconcilyx/operator/internal/carbon"
	"github.com/reconcilyx/operator/pkg/httpretry"
	"github.com/reconcilyx/operator/pkg/pipelinemetrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestElectricityMaps_Retries429And503BeforeFallback(t *testing.T) {
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		switch n {
		case 1:
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
		case 2:
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			b, _ := json.Marshal(map[string]any{
				"carbonIntensity": 98.4,
				"zone":            "FR",
			})
			_, _ = w.Write(b)
		}
	}))
	defer ts.Close()

	baseClient := ts.Client()
	p := carbon.NewElectricityMapsProvider("em-retry-key")
	p.BaseURL = ts.URL
	p.HTTPClient = &http.Client{
		Transport: &httpretry.Transport{
			Base:        baseClient.Transport,
			Component:   "carbon_electricitymaps",
			BaseDelay:   time.Millisecond,
			MaxDelay:    5 * time.Millisecond,
			MaxAttempts: 3,
			Sleep:       func(context.Context, time.Duration) error { return nil },
		},
	}

	beforeRetries := testutil.ToFloat64(pipelinemetrics.RetriesTotal.WithLabelValues("carbon_electricitymaps", "429"))
	it, err := p.IntensityForRegion(context.Background(), "FR")
	require.NoError(t, err)
	assert.Equal(t, int32(3), calls.Load(), "should recover on 3rd attempt instead of falling back immediately")
	assert.Equal(t, "electricitymaps", it.Source)
	assert.InDelta(t, 98.4, it.Value, 0.001)
	assert.Equal(t, float64(0), testutil.ToFloat64(pipelinemetrics.FallbackActive.WithLabelValues("carbon", "electricitymaps")))
	assert.GreaterOrEqual(t, testutil.ToFloat64(pipelinemetrics.RetriesTotal.WithLabelValues("carbon_electricitymaps", "429"))-beforeRetries, float64(1))
}

func TestWattTime_ExhaustedRetriesSetsFallbackGauge(t *testing.T) {
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	baseClient := ts.Client()
	p := carbon.NewWattTimeProvider("wt-retry-key")
	p.BaseURL = ts.URL
	p.HTTPClient = &http.Client{
		Transport: &httpretry.Transport{
			Base:        baseClient.Transport,
			Component:   "carbon_watttime",
			BaseDelay:   time.Millisecond,
			MaxDelay:    5 * time.Millisecond,
			MaxAttempts: 3,
			Sleep:       func(context.Context, time.Duration) error { return nil },
		},
	}

	it, err := p.IntensityForRegion(context.Background(), "aws-us-east-1")
	require.NoError(t, err)
	assert.Equal(t, int32(3), calls.Load(), "should exhaust 3 attempts before static fallback")
	assert.Equal(t, "static", it.Source)
	assert.Equal(t, float64(1), testutil.ToFloat64(pipelinemetrics.FallbackActive.WithLabelValues("carbon", "watttime")))
}
