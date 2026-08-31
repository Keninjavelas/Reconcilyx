package cloud

import (
	"os"
	"strconv"
	"time"
)

type PricingConfig struct {
	CacheTTL       time.Duration
	AWSEnabled     bool
	AWSRegion      string
	AWSAccessKeyID string
	AWSSecretKey   string
	GCPEnabled     bool
	GCPRegion      string
	GCPAPIKey      string
	AzureEnabled   bool
	AzureRegion    string
	AzureSubId     string
}

func PricingConfigFromEnv() PricingConfig {
	cfg := PricingConfig{
		CacheTTL:     1 * time.Hour,
		AWSEnabled:   true,
		GCPEnabled:   true,
		AzureEnabled: true,
	}
	if ttl := os.Getenv("RECONCILYX_PRICING_CACHE_TTL_HOURS"); ttl != "" {
		if h, err := strconv.Atoi(ttl); err == nil && h > 0 {
			cfg.CacheTTL = time.Duration(h) * time.Hour
		}
	}
	if v := os.Getenv("RECONCILYX_PRICING_AWS_ENABLED"); v != "" {
		cfg.AWSEnabled = parseBoolEnv(v, true)
	}
	if v := os.Getenv("RECONCILYX_PRICING_AWS_REGION"); v != "" {
		cfg.AWSRegion = v
	}
	cfg.AWSAccessKeyID = os.Getenv("AWS_ACCESS_KEY_ID")
	cfg.AWSSecretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
	if cfg.AWSRegion == "" {
		cfg.AWSRegion = os.Getenv("AWS_REGION")
	}
	if v := os.Getenv("RECONCILYX_PRICING_GCP_ENABLED"); v != "" {
		cfg.GCPEnabled = parseBoolEnv(v, true)
	}
	if v := os.Getenv("RECONCILYX_PRICING_GCP_REGION"); v != "" {
		cfg.GCPRegion = v
	}
	cfg.GCPAPIKey = os.Getenv("RECONCILYX_PRICING_GCP_API_KEY")
	if cfg.GCPAPIKey == "" {
		cfg.GCPAPIKey = os.Getenv("GCP_BILLING_API_KEY")
	}
	if v := os.Getenv("RECONCILYX_PRICING_AZURE_ENABLED"); v != "" {
		cfg.AzureEnabled = parseBoolEnv(v, true)
	}
	if v := os.Getenv("RECONCILYX_PRICING_AZURE_REGION"); v != "" {
		cfg.AzureRegion = v
	}
	cfg.AzureSubId = os.Getenv("RECONCILYX_PRICING_AZURE_SUB_ID")
	if cfg.AzureSubId == "" {
		cfg.AzureSubId = os.Getenv("AZURE_SUBSCRIPTION_ID")
	}
	return cfg
}

func parseBoolEnv(v string, def bool) bool {
	switch v {
	case "1", "true", "TRUE", "yes", "YES", "on", "ON":
		return true
	case "0", "false", "FALSE", "no", "NO", "off", "OFF":
		return false
	}
	return def
}

func NewPricingProvider(name ProviderName, cfg PricingConfig) *CachedPricingProvider {
	var raw PricingProvider
	switch name {
	case ProviderAWS:
		if !cfg.AWSEnabled {
			return nil
		}
		if cfg.AWSAccessKeyID != "" && cfg.AWSSecretKey != "" {
			p := NewLiveAWSPricingProvider(cfg.AWSAccessKeyID)
			if cfg.AWSRegion != "" {
				_ = cfg.AWSRegion
			}
			raw = p
		} else {
			raw = NewAWSPricingProvider()
		}
	case ProviderGCP:
		if !cfg.GCPEnabled {
			return nil
		}
		if cfg.GCPAPIKey != "" {
			raw = NewLiveGCPPricingProvider(cfg.GCPAPIKey)
		} else {
			raw = NewGCPPricingProvider()
		}
	case ProviderAzure:
		if !cfg.AzureEnabled {
			return nil
		}
		if cfg.AzureSubId != "" {
			raw = NewLiveAzurePricingProvider(cfg.AzureSubId)
		} else {
			raw = NewAzurePricingProvider()
		}
	default:
		return nil
	}
	return NewCachedPricingProvider(raw, cfg.CacheTTL)
}

func AllPricingProviders(cfg PricingConfig) []*CachedPricingProvider {
	out := make([]*CachedPricingProvider, 0, 3)
	for _, n := range []ProviderName{ProviderAWS, ProviderGCP, ProviderAzure} {
		if p := NewPricingProvider(n, cfg); p != nil {
			out = append(out, p)
		}
	}
	return out
}
