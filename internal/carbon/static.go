package carbon

import "context"

var globalAverage = 450.0

type StaticProvider struct {
	overrides map[string]float64
	fallback  float64
}

func NewStaticProvider() *StaticProvider {
	return &StaticProvider{
		overrides: map[string]float64{
			"aws-us-east-1":       380,
			"aws-us-east-2":       420,
			"aws-us-west-1":       320,
			"aws-us-west-2":       300,
			"aws-eu-west-1":       280,
			"aws-eu-west-2":       260,
			"aws-eu-central-1":    340,
			"aws-eu-north-1":      180,
			"aws-ap-southeast-1":  500,
			"aws-ap-southeast-2":  440,
			"aws-ap-northeast-1":  460,
			"aws-sa-east-1":      540,
			"gcp-us-east1":        380,
			"gcp-us-east4":        400,
			"gcp-us-west1":        320,
			"gcp-us-west2":        320,
			"gcp-europe-west1":     280,
			"gcp-europe-west2":     260,
			"gcp-europe-west3":     340,
			"gcp-europe-north1":     180,
			"gcp-asia-east1":       500,
			"gcp-asia-southeast1":  500,
			"gcp-asia-northeast1": 460,
			"gcp-southamerica-east1": 540,
			"azure-eastus":        380,
			"azure-eastus2":       420,
			"azure-westus":        320,
			"azure-westus2":       300,
			"azure-westeurope":    280,
			"azure-uksouth":       260,
			"azure-germanywestcentral": 340,
			"azure-switzerlandnorth": 200,
			"azure-southeastasia": 500,
			"azure-eastasia":      500,
			"azure-japaneast":     460,
			"azure-brazilsouth":   540,
		},
		fallback: globalAverage,
	}
}

func (p *StaticProvider) SetFallback(fb float64) {
	if fb > 0 {
		p.fallback = fb
	}
}

func (p *StaticProvider) IntensityForRegion(_ context.Context, region string) (Intensity, error) {
	val, ok := p.overrides[region]
	if !ok {
		val = p.fallback
	}
	return Intensity{
		Value:  val,
		Source: "static",
		Region: region,
	}, nil
}
