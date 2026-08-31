package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type OpenAIProvider struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

func NewOpenAIProvider(config map[string]string) (Provider, error) {
	apiKey := config["apiKey"]
	baseURL := config["url"]
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	model := config["model"]
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &OpenAIProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (o *OpenAIProvider) Name() string {
	return "openai"
}

func (o *OpenAIProvider) SupportsFunctionCalling() bool {
	return true
}

func (o *OpenAIProvider) Chat(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error) {
	if o.apiKey == "" {
		return &ChatResponse{
			Content: "OpenAI API key not configured. Please set OPENAI_API_KEY or switch provider to Ollama.",
		}, nil
	}

	type reqBody struct {
		Model    string    `json:"model"`
		Messages []Message `json:"messages"`
		Tools    []Tool    `json:"tools,omitempty"`
	}

	payload := reqBody{
		Model:    o.model,
		Messages: messages,
		Tools:    tools,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &ChatResponse{
			Content: fmt.Sprintf("OpenAI API returned HTTP status %s", resp.Status),
		}, nil
	}

	var res struct {
		Choices []struct {
			Message struct {
				Content   string     `json:"content"`
				ToolCalls []ToolCall `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	if len(res.Choices) == 0 {
		return &ChatResponse{Content: "No response received from OpenAI."}, nil
	}

	return &ChatResponse{
		Content:   res.Choices[0].Message.Content,
		ToolCalls: res.Choices[0].Message.ToolCalls,
	}, nil
}
