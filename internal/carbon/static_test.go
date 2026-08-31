package carbon_test

import (
	"context"
	"testing"

	"github.com/reconcilyx/operator/internal/carbon"
	"github.com/stretchr/testify/assert"
)

func TestStaticProvider_KnownRegion(t *testing.T) {
	p := carbon.NewStaticProvider()
	intensity, err := p.IntensityForRegion(context.Background(), "aws-us-east-1")
	assert.NoError(t, err)
	assert.Equal(t, 380.0, intensity.Value, "us-east-1 is 380 gCO2/kWh")
	assert.Equal(t, "static", intensity.Source)
	assert.Equal(t, "aws-us-east-1", intensity.Region)
}

func TestStaticProvider_KnownRegionGCP(t *testing.T) {
	p := carbon.NewStaticProvider()
	intensity, err := p.IntensityForRegion(context.Background(), "gcp-europe-north1")
	assert.NoError(t, err)
	assert.Equal(t, 180.0, intensity.Value, "Finland (europe-north1) is very low carbon")
}

func TestStaticProvider_KnownRegionAzure(t *testing.T) {
	p := carbon.NewStaticProvider()
	intensity, err := p.IntensityForRegion(context.Background(), "azure-southeastasia")
	assert.NoError(t, err)
	assert.Equal(t, 500.0, intensity.Value, "SE Asia is 500 gCO2/kWh")
}

func TestStaticProvider_UnknownRegionFallsBackToGlobalAverage(t *testing.T) {
	p := carbon.NewStaticProvider()
	intensity, err := p.IntensityForRegion(context.Background(), "aws-moon-base-1")
	assert.NoError(t, err)
	assert.Equal(t, 450.0, intensity.Value, "unknown regions fall back to global average 450")
	assert.Equal(t, "aws-moon-base-1", intensity.Region)
}

func TestStaticProvider_EmptyStringRegion(t *testing.T) {
	p := carbon.NewStaticProvider()
	intensity, err := p.IntensityForRegion(context.Background(), "")
	assert.NoError(t, err)
	assert.Equal(t, 450.0, intensity.Value, "empty string uses global average")
}

func TestStaticProvider_AllKnownRegionsAreConsistent(t *testing.T) {
	p := carbon.NewStaticProvider()
	regions := []string{
		"aws-us-east-1", "aws-us-west-2", "aws-eu-west-1", "aws-eu-north-1",
		"gcp-us-east1", "gcp-europe-west1", "gcp-asia-east1", "gcp-southamerica-east1",
		"azure-eastus", "azure-westeurope", "azure-japaneast", "azure-brazilsouth",
	}
	for _, r := range regions {
		intensity, err := p.IntensityForRegion(context.Background(), r)
		assert.NoError(t, err, "region %s should not error", r)
		assert.Greater(t, intensity.Value, 10.0, "%s: intensity must be positive", r)
		assert.Less(t, intensity.Value, 800.0, "%s: intensity sanity-check upper bound", r)
	}
}
