package llm

import (
	"context"
	"testing"
)

func TestReconcilyxToolsCount(t *testing.T) {
	tools := GetReconcilyxTools()
	if len(tools) != 15 {
		t.Errorf("Expected 15 tool definitions, got %d", len(tools))
	}
}

func TestProviderFactory(t *testing.T) {
	providerNames := []string{"ollama", "openai", "anthropic", "groq", "cohere"}

	for _, name := range providerNames {
		p, err := NewProvider(name, map[string]string{"url": "http://localhost:11434"})
		if err != nil {
			t.Errorf("Failed to instantiate provider %s: %v", name, err)
		}
		if p == nil {
			t.Errorf("Provider %s is nil", name)
		}
	}
}

func TestHybridRouterFastPath(t *testing.T) {
	ctx := context.Background()
	provider, _ := NewProvider("ollama", nil)
	router := NewHybridRouter(provider)

	tests := []struct {
		query    string
		wantFunc string
	}{
		{"Why is my bill so high?", "getSavings"},
		{"Show me pending proposals", "getProposals"},
		{"What is my carbon footprint?", "getCarbon"},
		{"Where can I migrate to AWS or GCP?", "evaluateArbitrage"},
		{"Show me resource forecast trends", "forecastWorkload"},
	}

	for _, tt := range tests {
		res, err := router.Route(ctx, tt.query)
		if err != nil {
			t.Fatalf("Route failed for query '%s': %v", tt.query, err)
		}
		if res.FunctionName != tt.wantFunc {
			t.Errorf("Query '%s' mapped to function %s, want %s", tt.query, res.FunctionName, tt.wantFunc)
		}
		if !res.IsFastPath {
			t.Errorf("Expected fast-path match for query '%s'", tt.query)
		}
	}
}
