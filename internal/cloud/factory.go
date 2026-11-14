package cloud

import (
	"os"
	"strconv"
	"time"

	"github.com/reconcilyx/operator/pkg/security/credentials"
)

type PricingConfig struct {
	CacheTTL                time.Duration
	MountDir                string
	AWSEnabled              bool
	AWSRoleARN              string
	AWSWebIdentityTokenFile string
	AWSAccessKeyID          string
	AWSSecretKey            string
	AWSSessionToken         string
	GCPEnabled              bool
	GCPWorkloadIdentity     bool
	GCPAPIKey               string
	AzureEnabled            bool
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
	cfg.MountDir = os.Getenv("RECONCILYX_SECRETS_DIR")
	if v := os.Getenv("RECONCILYX_PRICING_AWS_ENABLED"); v != "" {
		cfg.AWSEnabled = parseBoolEnv(v, true)
	}
	cfg.AWSRoleARN = os.Getenv("AWS_ROLE_ARN")
	cfg.AWSWebIdentityTokenFile = os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE")
	cfg.AWSAccessKeyID = credentials.LoadFromFileOrEnv(cfg.MountDir, "aws", "access_key_id", "AWS_ACCESS_KEY_ID").Secret.Expose()
	cfg.AWSSecretKey = credentials.LoadFromFileOrEnv(cfg.MountDir, "aws", "secret_access_key", "AWS_SECRET_ACCESS_KEY").Secret.Expose()
	cfg.AWSSessionToken = credentials.LoadFromFileOrEnv(cfg.MountDir, "aws", "session_token", "AWS_SESSION_TOKEN").Secret.Expose()
	if v := os.Getenv("RECONCILYX_PRICING_GCP_ENABLED"); v != "" {
		cfg.GCPEnabled = parseBoolEnv(v, true)
	}
	cfg.GCPWorkloadIdentity = parseBoolEnv(os.Getenv("RECONCILYX_GCP_WORKLOAD_IDENTITY"), false)
	cfg.GCPAPIKey = credentials.LoadFromFileOrEnv(cfg.MountDir, "gcp", "api_key", "RECONCILYX_PRICING_GCP_API_KEY", "GCP_BILLING_API_KEY").Secret.Expose()
	if v := os.Getenv("RECONCILYX_PRICING_AZURE_ENABLED"); v != "" {
		cfg.AzureEnabled = parseBoolEnv(v, true)
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
		if cfg.AWSRoleARN != "" && cfg.AWSWebIdentityTokenFile != "" {
			raw = NewWebIdentityAWSPricingProvider(cfg.AWSRoleARN, cfg.AWSWebIdentityTokenFile, "")
		} else if cfg.AWSAccessKeyID != "" && cfg.AWSSecretKey != "" {
			raw = NewLiveAWSPricingProvider(cfg.AWSAccessKeyID, cfg.AWSSecretKey, cfg.AWSSessionToken)
		} else {
			raw = NewAWSPricingProvider()
		}
	case ProviderGCP:
		if !cfg.GCPEnabled {
			return nil
		}
		if cfg.GCPWorkloadIdentity {
			raw = NewWorkloadIdentityGCPPricingProvider("")
		} else if cfg.GCPAPIKey != "" {
			raw = NewLiveGCPPricingProvider(cfg.GCPAPIKey)
		} else {
			raw = NewGCPPricingProvider()
		}
	case ProviderAzure:
		if !cfg.AzureEnabled {
			return nil
		}
		// The Azure Retail Prices API is anonymous; no subscription required.
		raw = NewLiveAzurePricingProvider()
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
