package cloud

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	awsPricingEndpoint = "https://api.pricing.us-east-1.amazonaws.com"
	awsPricingService  = "pricing"
	awsPricingRegion   = "us-east-1"
)

type AWSPricingProvider struct {
	HTTPClient   *http.Client
	BaseURL      string
	Retry        RetryPolicy
	accessKeyID  string
	secretKey    string
	sessionToken string
	enableLive   bool
	fallback     *staticAWSCatalog
}

type staticAWSCatalog struct {
	rates  map[string]float64
	carbon map[string]float64
}

func newStaticAWSCatalog() *staticAWSCatalog {
	return &staticAWSCatalog{
		rates: map[string]float64{
			"c6i.large":  0.085,
			"c6i.xlarge": 0.170,
			"m6i.large":  0.096,
			"m6i.xlarge": 0.192,
			"t4g.medium": 0.0336,
			"t4g.large":  0.0672,
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
		BaseURL:    awsPricingEndpoint,
		enableLive: false,
		fallback:   newStaticAWSCatalog(),
	}
}

// NewLiveAWSPricingProvider enables the AWS Price List Query API. Live lookups
// require SigV4 credentials; callers without credentials should use the
// bundled static catalog.
func NewLiveAWSPricingProvider(accessKeyID, secretKey, sessionToken string) *AWSPricingProvider {
	p := NewAWSPricingProvider()
	p.enableLive = accessKeyID != "" && secretKey != ""
	p.accessKeyID = accessKeyID
	p.secretKey = secretKey
	p.sessionToken = sessionToken
	return p
}

func (a *AWSPricingProvider) Name() ProviderName { return ProviderAWS }

func (a *AWSPricingProvider) Live() bool { return a.enableLive }

func carbonForAWS(region string, cmap map[string]float64) float64 {
	if c, ok := cmap[region]; ok {
		return c
	}
	return 300.0
}

func (a *AWSPricingProvider) fetchOnDemandLive(ctx context.Context, instanceType, region string) (float64, error) {
	if !a.enableLive || a.HTTPClient == nil || a.BaseURL == "" {
		return 0, fmt.Errorf("live AWS pricing disabled (no credentials or client)")
	}
	payload, err := json.Marshal(map[string]any{
		"ServiceCode": "AmazonEC2",
		"Filters": []map[string]string{
			{"Type": "TERM_MATCH", "Field": "instanceType", "Value": instanceType},
			{"Type": "TERM_MATCH", "Field": "region", "Value": region},
			{"Type": "TERM_MATCH", "Field": "operatingSystem", "Value": "Linux"},
			{"Type": "TERM_MATCH", "Field": "tenancy", "Value": "Shared"},
			{"Type": "TERM_MATCH", "Field": "preInstalledSw", "Value": "NA"},
			{"Type": "TERM_MATCH", "Field": "capacitystatus", "Value": "Used"},
		},
		"MaxResults": 10,
	})
	if err != nil {
		return 0, err
	}

	resp, err := doWithRetry(ctx, a.HTTPClient, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.BaseURL, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-amz-json-1.1")
		req.Header.Set("X-Amz-Target", "AWSPriceListService.GetProducts")
		signAWSRequest(req, payload, a.accessKeyID, a.secretKey, a.sessionToken, awsPricingRegion, awsPricingService, time.Now().UTC())
		return req, nil
	}, a.Retry)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return 0, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("aws pricing api status %d: %s", resp.StatusCode, truncate(string(raw), 256))
	}

	var doc struct {
		PriceList []string `json:"PriceList"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return 0, err
	}
	for _, item := range doc.PriceList {
		var parsed struct {
			Product struct {
				Attributes struct {
					InstanceType string `json:"instanceType"`
				} `json:"attributes"`
			} `json:"product"`
			Terms struct {
				OnDemand map[string]map[string]struct {
					PriceDimensions map[string]struct {
						PricePerUnit map[string]string `json:"pricePerUnit"`
					} `json:"priceDimensions"`
				} `json:"OnDemand"`
			} `json:"terms"`
		}
		if err := json.Unmarshal([]byte(item), &parsed); err != nil {
			continue
		}
		if parsed.Product.Attributes.InstanceType != "" && parsed.Product.Attributes.InstanceType != instanceType {
			continue
		}
		for _, term := range parsed.Terms.OnDemand {
			for _, dim := range term {
				for _, price := range dim.PriceDimensions {
					if usd, ok := price.PricePerUnit["USD"]; ok {
						if v, err := strconvParseFloat64(usd); err == nil && v > 0 {
							return v, nil
						}
					}
				}
			}
		}
	}
	return 0, fmt.Errorf("instance %s not found in AWS price list for %s", instanceType, region)
}

func (a *AWSPricingProvider) OnDemandPrice(ctx context.Context, instanceType, region string) (float64, error) {
	if a.enableLive {
		if v, err := a.fetchOnDemandLive(ctx, instanceType, region); err == nil && v > 0 {
			return v, nil
		} else if err != nil {
			slog.Warn("Live AWS pricing unavailable; using bundled fallback",
				"instance", instanceType, "region", region, "error", err.Error())
		}
	}
	if rate, ok := a.fallback.rates[instanceType]; ok {
		return rate, nil
	}
	return 0, fmt.Errorf("no AWS price for %s in %s and no local fallback", instanceType, region)
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
	if a.enableLive {
		for i := range catalog {
			live, err := a.fetchOnDemandLive(ctx, catalog[i].Name, region)
			if err == nil && live > 0 {
				catalog[i].OnDemandPrice = live
				catalog[i].SpotPrice = live * 0.35
				continue
			}
			if err != nil {
				slog.Warn("Live AWS price refresh failed; keeping bundled price",
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

func signAWSRequest(req *http.Request, body []byte, accessKeyID, secretKey, sessionToken, region, service string, now time.Time) {
	amzDate := now.UTC().Format("20060102T150405Z")
	dateStamp := now.UTC().Format("20060102")

	req.Header.Set("X-Amz-Date", amzDate)
	if sessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", sessionToken)
	}

	host := req.URL.Host
	req.Host = host

	headers := map[string]string{
		"content-type": req.Header.Get("Content-Type"),
		"host":         host,
		"x-amz-date":   amzDate,
		"x-amz-target": req.Header.Get("X-Amz-Target"),
	}
	if sessionToken != "" {
		headers["x-amz-security-token"] = sessionToken
	}
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)

	var canonicalHeaders strings.Builder
	for _, name := range names {
		canonicalHeaders.WriteString(name)
		canonicalHeaders.WriteString(":")
		canonicalHeaders.WriteString(strings.TrimSpace(headers[name]))
		canonicalHeaders.WriteString("\n")
	}
	signedHeaders := strings.Join(names, ";")

	canonicalURI := req.URL.EscapedPath()
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	payloadHash := sha256Hex(body)
	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI,
		req.URL.Query().Encode(),
		canonicalHeaders.String(),
		signedHeaders,
		payloadHash,
	}, "\n")

	scope := dateStamp + "/" + region + "/" + service + "/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + sha256Hex([]byte(canonicalRequest))

	signingKey := hmacSHA256([]byte("AWS4"+secretKey), dateStamp)
	signingKey = hmacSHA256(signingKey, region)
	signingKey = hmacSHA256(signingKey, service)
	signingKey = hmacSHA256(signingKey, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(signingKey, stringToSign))

	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKeyID, scope, signedHeaders, signature,
	))
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
