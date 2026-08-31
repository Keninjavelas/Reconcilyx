package carbon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	emDefaultBaseURL = "https://api.electricitymap.org/v3/carbon-intensity/latest"
)

type ElectricityMapsProvider struct {
	HTTPClient *http.Client
	BaseURL    string
	APIKey     string
	Fallback   Provider
}

func NewElectricityMapsProvider(apiKey string) *ElectricityMapsProvider {
	return &ElectricityMapsProvider{
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
		BaseURL:    emDefaultBaseURL,
		APIKey:     apiKey,
		Fallback:   NewStaticProvider(),
	}
}

func (e *ElectricityMapsProvider) IntensityForRegion(ctx context.Context, region string) (Intensity, error) {
	if e.HTTPClient == nil || e.BaseURL == "" || e.APIKey == "" {
		if e.Fallback != nil {
			return e.Fallback.IntensityForRegion(ctx, region)
		}
		return Intensity{}, fmt.Errorf("electricitymaps disabled")
	}
	u, _ := url.Parse(e.BaseURL)
	q := u.Query()
	q.Set("zone", region)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return e.fallbackOrErr(ctx, region, err)
	}
	req.Header.Set("auth-token", e.APIKey)
	resp, err := e.HTTPClient.Do(req)
	if err != nil {
		return e.fallbackOrErr(ctx, region, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return e.fallbackOrErr(ctx, region, fmt.Errorf("electricitymaps status %d", resp.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))
	if err != nil {
		return e.fallbackOrErr(ctx, region, err)
	}
	var doc struct {
		CarbonIntensity float64 `json:"carbonIntensity"`
		Zone            string  `json:"zone"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return e.fallbackOrErr(ctx, region, err)
	}
	if doc.CarbonIntensity <= 0 {
		return e.fallbackOrErr(ctx, region, fmt.Errorf("invalid intensity response"))
	}
	return Intensity{
		Value:  doc.CarbonIntensity,
		Source: "electricitymaps",
		Region: doc.Zone,
	}, nil
}

func (e *ElectricityMapsProvider) fallbackOrErr(ctx context.Context, region string, err error) (Intensity, error) {
	if e.Fallback != nil {
		return e.Fallback.IntensityForRegion(ctx, region)
	}
	return Intensity{}, err
}
