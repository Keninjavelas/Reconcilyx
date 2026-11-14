package carbon

import (
	"os"
	"strconv"
	"time"

	"github.com/reconcilyx/operator/pkg/security/credentials"
)

type CarbonConfig struct {
	Provider        string
	CacheTTL        time.Duration
	MountDir        string
	ElectricityMaps ElectricityMapsConfig
	WattTime        WattTimeConfig
	StaticFallback  StaticConfig
}

type ElectricityMapsConfig struct {
	APIKey string
	BaseURL string
}

type WattTimeConfig struct {
	APIKey   string
	Username string
	Password string
	BaseURL  string
}

type StaticConfig struct {
	FallbackIntensity float64
}

func CarbonConfigFromEnv() CarbonConfig {
	cfg := CarbonConfig{
		Provider: "static",
		CacheTTL: 1 * time.Hour,
	}
	if v := os.Getenv("RECONCILYX_CARBON_PROVIDER"); v != "" {
		cfg.Provider = v
	}
	if ttl := os.Getenv("RECONCILYX_CARBON_CACHE_TTL_HOURS"); ttl != "" {
		if h, err := strconv.Atoi(ttl); err == nil && h > 0 {
			cfg.CacheTTL = time.Duration(h) * time.Hour
		}
	}
	mountDir := os.Getenv("RECONCILYX_SECRETS_DIR")
	cfg.MountDir = mountDir
	cfg.ElectricityMaps.APIKey = credentials.LoadFromFileOrEnv(mountDir, "electricitymaps", "api_key", "RECONCILYX_EM_API_KEY", "ELECTRICITY_MAPS_API_KEY").Secret.Expose()
	cfg.ElectricityMaps.BaseURL = os.Getenv("RECONCILYX_EM_BASE_URL")
	cfg.WattTime.APIKey = credentials.LoadFromFileOrEnv(mountDir, "watttime", "api_key", "RECONCILYX_WT_API_KEY", "WATTTIME_API_KEY").Secret.Expose()
	cfg.WattTime.Username = os.Getenv("WATTTIME_USERNAME")
	cfg.WattTime.Password = os.Getenv("WATTTIME_PASSWORD")
	cfg.WattTime.BaseURL = os.Getenv("RECONCILYX_WT_BASE_URL")
	if v := os.Getenv("RECONCILYX_CARBON_STATIC_FALLBACK"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			cfg.StaticFallback.FallbackIntensity = f
		}
	}
	return cfg
}

func NewCarbonProvider(cfg CarbonConfig) Provider {
	var raw Provider
	switch cfg.Provider {
	case "electricitymaps", "em":
		if cfg.ElectricityMaps.APIKey == "" {
			raw = NewStaticProvider()
		} else {
			p := NewElectricityMapsProvider(cfg.ElectricityMaps.APIKey)
			if cfg.ElectricityMaps.BaseURL != "" {
				p.BaseURL = cfg.ElectricityMaps.BaseURL
			}
			raw = p
		}
	case "watttime", "wt":
		if cfg.WattTime.APIKey == "" && (cfg.WattTime.Username == "" || cfg.WattTime.Password == "") {
			raw = NewStaticProvider()
		} else if cfg.WattTime.APIKey != "" {
			p := NewWattTimeProvider(cfg.WattTime.APIKey)
			if cfg.WattTime.BaseURL != "" {
				p.BaseURL = cfg.WattTime.BaseURL
			}
			raw = p
		} else {
			p := NewWattTimeProviderWithCreds(cfg.WattTime.Username, cfg.WattTime.Password)
			if cfg.WattTime.BaseURL != "" {
				p.BaseURL = cfg.WattTime.BaseURL
			}
			raw = p
		}
	case "static", "":
		fallthrough
	default:
		sp := NewStaticProvider()
		if cfg.StaticFallback.FallbackIntensity > 0 {
			sp.fallback = cfg.StaticFallback.FallbackIntensity
		}
		raw = sp
	}
	return NewCachedCarbonProvider(raw, cfg.CacheTTL)
}
