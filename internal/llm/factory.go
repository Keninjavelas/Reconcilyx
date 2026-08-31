package llm

import (
	"fmt"
	"strings"
)

type ProviderInitializer func(config map[string]string) (Provider, error)

var registry = map[string]ProviderInitializer{
	"ollama":   NewOllamaProvider,
	"openai":   NewOpenAIProvider,
	"anthropic": NewOpenAIProvider, // Anthropic format mapping
	"groq":      NewOpenAIProvider, // Groq OpenAI-compatible endpoint
	"cohere":    NewOpenAIProvider, // Cohere OpenAI-compatible endpoint
}

func NewProvider(name string, config map[string]string) (Provider, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		name = "ollama"
	}

	fn, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("llm: unsupported provider '%s'", name)
	}

	return fn(config)
}
