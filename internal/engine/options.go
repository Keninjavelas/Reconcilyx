package engine

import (
	"github.com/reconcilyx/operator/internal/carbon"
	"github.com/reconcilyx/operator/internal/engine/rl"
)

type AnalyzerOption func(*defaultAnalyzer)

func WithCarbonProvider(p carbon.Provider) AnalyzerOption {
	return func(a *defaultAnalyzer) {
		a.carbonProvider = p
	}
}

func WithRLAgent(agent rl.Agent) AnalyzerOption {
	return func(a *defaultAnalyzer) {
		a.rlAgent = agent
	}
}

func WithWorkloadRegion(region string) AnalyzerOption {
	return func(a *defaultAnalyzer) {
		a.workloadRegion = region
	}
}

func WithCarbonWeight(w float64) AnalyzerOption {
	return func(a *defaultAnalyzer) {
		if w < 0 {
			w = 0
		}
		if w > 1.0 {
			w = 1.0
		}
		a.carbonWeight = w
	}
}

func RLAgentFrom(a Analyzer) (rl.Agent, bool) {
	impl, ok := a.(*defaultAnalyzer)
	if !ok {
		return nil, false
	}
	return impl.rlAgent, true
}

func RegionFrom(a Analyzer) string {
	impl, ok := a.(*defaultAnalyzer)
	if !ok {
		return ""
	}
	return impl.workloadRegion
}

func CarbonWeightFrom(a Analyzer) float64 {
	impl, ok := a.(*defaultAnalyzer)
	if !ok {
		return 0.0
	}
	return impl.carbonWeight
}

func DefaultRegionFrom(a Analyzer) string {
	if a == nil {
		return defaultRegion
	}
	return RegionFrom(a)
}

func DefaultCarbonWeightFrom(a Analyzer) float64 {
	if a == nil {
		return defaultCarbonWeight
	}
	return CarbonWeightFrom(a)
}

func DefaultCarbonProviderFrom(a Analyzer) carbon.Provider {
	if a == nil {
		return nil
	}
	impl, ok := a.(*defaultAnalyzer)
	if !ok {
		return nil
	}
	return impl.carbonProvider
}

