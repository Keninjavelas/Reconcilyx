package carbon

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/reconcilyx/operator/pkg/httpretry"
	"github.com/reconcilyx/operator/pkg/pipelinemetrics"
	"github.com/reconcilyx/operator/pkg/security/credentials"
)

const (
	wtDefaultBaseURL = "https://api.watttime.org/api/v3"
)

type WattTimeProvider struct {
	HTTPClient   *http.Client
	BaseURL      string
	Username     string
	Password     string
	APIKey       string
	SecretSource func() credentials.SecretString
	Fallback     Provider
}

func NewWattTimeProvider(apiKey string) *WattTimeProvider {
	return &WattTimeProvider{
		HTTPClient: httpretry.WrapClient(&http.Client{Timeout: 15 * time.Second}, "carbon_watttime"),
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
	if w.SecretSource != nil {
		if sec := w.SecretSource(); !sec.IsEmpty() {
			return "Bearer " + sec.Expose()
		}
	}
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
	endpoint := strings.TrimRight(w.BaseURL, "/") + "/current"
	if strings.Contains(w.BaseURL, "watttime.org") && !strings.Contains(w.BaseURL, "/signal-index") {
		endpoint = strings.TrimRight(w.BaseURL, "/") + "/signal-index"
	}
	u, _ := url.Parse(endpoint)
	q := u.Query()
	q.Set("region", region)
	if strings.Contains(endpoint, "signal-index") {
		q.Set("signal_type", "co2_moer")
	}
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return w.fallbackOrErr(ctx, region, err)
	}
	req.Header.Set("Authorization", tok)
	resp, err := httpretry.WrapClient(w.HTTPClient, "carbon_watttime").Do(req)
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
		Data   []struct {
			Value float64 `json:"value"`
		} `json:"data"`
		Meta struct {
			Region string `json:"region"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return w.fallbackOrErr(ctx, region, err)
	}
	val := doc.Value
	reg := doc.Region
	if val <= 0 && len(doc.Data) > 0 {
		val = doc.Data[0].Value
	}
	if reg == "" {
		reg = doc.Meta.Region
	}
	if reg == "" {
		reg = region
	}
	if val <= 0 {
		return w.fallbackOrErr(ctx, region, fmt.Errorf("invalid intensity response"))
	}
	pipelinemetrics.FallbackActive.WithLabelValues("carbon", "watttime").Set(0)
	return Intensity{
		Value:  val,
		Source: "watttime",
		Region: reg,
	}, nil
}

func (w *WattTimeProvider) fallbackOrErr(ctx context.Context, region string, err error) (Intensity, error) {
	pipelinemetrics.FallbackActive.WithLabelValues("carbon", "watttime").Set(1)
	if w.Fallback != nil {
		return w.Fallback.IntensityForRegion(ctx, region)
	}
	return Intensity{}, err
}
