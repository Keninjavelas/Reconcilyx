package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultPrometheusURL(t *testing.T) {
	t.Run("falls back to the in-cluster default", func(t *testing.T) {
		t.Setenv("RECONCILYX_PROMETHEUS_URL", "")
		t.Setenv("PROMETHEUS_URL", "")
		assert.Equal(t, "http://prometheus-operated.monitoring.svc.cluster.local:9090", DefaultPrometheusURL())
	})

	t.Run("honors the chart-provided env", func(t *testing.T) {
		t.Setenv("RECONCILYX_PROMETHEUS_URL", "http://prometheus.monitoring.svc:9090")
		t.Setenv("PROMETHEUS_URL", "")
		assert.Equal(t, "http://prometheus.monitoring.svc:9090", DefaultPrometheusURL())
	})

	t.Run("keeps the legacy env working", func(t *testing.T) {
		t.Setenv("RECONCILYX_PROMETHEUS_URL", "")
		t.Setenv("PROMETHEUS_URL", "http://legacy:9090")
		assert.Equal(t, "http://legacy:9090", DefaultPrometheusURL())
	})

	t.Run("prefers the chart env over the legacy one", func(t *testing.T) {
		t.Setenv("RECONCILYX_PROMETHEUS_URL", "http://chart:9090")
		t.Setenv("PROMETHEUS_URL", "http://legacy:9090")
		assert.Equal(t, "http://chart:9090", DefaultPrometheusURL())
	})
}
