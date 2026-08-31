package carbon

import "context"

type Intensity struct {
	Value  float64
	Source string
	Region string
}

type Provider interface {
	IntensityForRegion(ctx context.Context, region string) (Intensity, error)
}
