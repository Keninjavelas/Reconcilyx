package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type OllamaProvider struct {
	baseURL string
	model   string
	client  *http.Client
}

func NewOllamaProvider(config map[string]string) (Provider, error) {
	baseURL := config["url"]
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	model := config["model"]
	if model == "" {
		model = "llama3.2:3b"
	}
	return &OllamaProvider{
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (o *OllamaProvider) Name() string {
	return "ollama"
}

func (o *OllamaProvider) SupportsFunctionCalling() bool {
	return true
}

func (o *OllamaProvider) Chat(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error) {
	type reqBody struct {
		Model    string    `json:"model"`
		Messages []Message `json:"messages"`
		Stream   bool      `json:"stream"`
		Tools    []Tool    `json:"tools,omitempty"`
	}

	payload := reqBody{
		Model:    o.model,
		Messages: messages,
		Stream:   false,
		Tools:    tools,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/api/chat", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		// Return friendly message if local Ollama server is offline
		return &ChatResponse{
			Content: fmt.Sprintf("Ollama local LLM server unreachable at %s (%v). Using deterministic intent router.", o.baseURL, err),
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &ChatResponse{
			Content: fmt.Sprintf("Ollama returned HTTP status %s. Falling back to local intent parser.", resp.Status),
		}, nil
	}

	var res struct {
		Message struct {
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls"`
		} `json:"message"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	return &ChatResponse{
		Content:   res.Message.Content,
		ToolCalls: res.Message.ToolCalls,
	}, nil
}
