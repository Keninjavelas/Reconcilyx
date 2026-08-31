package carbon

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	wtDefaultBaseURL = "https://api.watttime.org/api/v3"
)

type WattTimeProvider struct {
	HTTPClient *http.Client
	BaseURL    string
	Username   string
	Password   string
	APIKey     string
	Fallback   Provider
}

func NewWattTimeProvider(apiKey string) *WattTimeProvider {
	return &WattTimeProvider{
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
		BaseURL:    wtDefaultBaseURL,
		APIKey:     apiKey,
		Fallback:   NewStaticProvider(),
	}
}

func NewWattTimeProviderWithCreds(username, password string) *WattTimeProvider {
	p := NewWattTimeProvider("")
	p.Username = username
	p.Password = password
	return p
}

func (w *WattTimeProvider) bearerToken() string {
	if w.APIKey != "" {
		return "Bearer " + w.APIKey
	}
	if w.Username != "" && w.Password != "" {
		raw := w.Username + ":" + w.Password
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(raw))
	}
	return ""
}

func (w *WattTimeProvider) IntensityForRegion(ctx context.Context, region string) (Intensity, error) {
	tok := w.bearerToken()
	if w.HTTPClient == nil || w.BaseURL == "" || tok == "" {
		if w.Fallback != nil {
			return w.Fallback.IntensityForRegion(ctx, region)
		}
		return Intensity{}, fmt.Errorf("watttime disabled (no credentials)")
	}
	u, _ := url.Parse(w.BaseURL + "/current")
	q := u.Query()
	q.Set("region", region)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return w.fallbackOrErr(ctx, region, err)
	}
	req.Header.Set("Authorization", tok)
	resp, err := w.HTTPClient.Do(req)
	if err != nil {
		return w.fallbackOrErr(ctx, region, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return w.fallbackOrErr(ctx, region, fmt.Errorf("watttime status %d", resp.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))
	if err != nil {
		return w.fallbackOrErr(ctx, region, err)
	}
	var doc struct {
		Value  float64 `json:"value"`
		Region string  `json:"region"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return w.fallbackOrErr(ctx, region, err)
	}
	if doc.Value <= 0 {
		return w.fallbackOrErr(ctx, region, fmt.Errorf("invalid intensity response"))
	}
	return Intensity{
		Value:  doc.Value,
		Source: "watttime",
		Region: doc.Region,
	}, nil
}

func (w *WattTimeProvider) fallbackOrErr(ctx context.Context, region string, err error) (Intensity, error) {
	if w.Fallback != nil {
		return w.Fallback.IntensityForRegion(ctx, region)
	}
	return Intensity{}, err
}
