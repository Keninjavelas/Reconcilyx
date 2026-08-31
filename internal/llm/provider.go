package llm

import (
	"context"
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type Message struct {
	Role       Role        `json:"role"`
	Content    string      `json:"content"`
	ToolCalls  []ToolCall  `json:"toolCalls,omitempty"`
	ToolCallID string      `json:"toolCallId,omitempty"`
}

type FunctionDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type Tool struct {
	Type     string             `json:"type"` // "function"
	Function FunctionDefinition `json:"function"`
}

type FunctionCall struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type ChatResponse struct {
	Content   string                 `json:"content"`
	ToolCalls []ToolCall             `json:"toolCalls,omitempty"`
	ChartData map[string]interface{} `json:"chartData,omitempty"`
}

type Provider interface {
	Name() string
	SupportsFunctionCalling() bool
	Chat(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error)
}
