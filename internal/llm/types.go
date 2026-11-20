package llm

import (
	"context"
	"encoding/json"
)

// Message represents a conversation turn across providers.
type Message struct {
	Role       string     `json:"role"` // system | user | assistant | tool
	Content    string     `json:"content,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolName   string     `json:"tool_name,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

// ToolDef defines a JSON-Schema tool available to the model.
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ToolCall represents an LLM function/tool invocation.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// Usage tracks token consumption and prompt-cache savings.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	CachedTokens int `json:"cached_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// ChatRequest encapsulates provider-agnostic generation inputs.
type ChatRequest struct {
	Model          string    `json:"model"`
	SystemPrefix   string    `json:"system_prefix"`   // Tier-0 cache-stable instructions
	DynamicContext string    `json:"dynamic_context"` // Tier 1-3 scene context
	Messages       []Message `json:"messages"`
	Tools          []ToolDef `json:"tools,omitempty"`
	Temperature    float64   `json:"temperature,omitempty"`
	MaxTokens      int       `json:"max_tokens,omitempty"`
	EnableCache    bool      `json:"enable_cache"`
}

// StreamEvent represents one streamed chunk or completion signal from an LLM provider.
type StreamEvent struct {
	Type          string     `json:"type"` // delta | thinking | tool_call | usage | done | error
	Delta         string     `json:"delta,omitempty"`
	ThinkingDelta string     `json:"thinking_delta,omitempty"`
	ToolCall      *ToolCall  `json:"tool_call,omitempty"`
	Usage         *Usage     `json:"usage,omitempty"`
	Error         string     `json:"error,omitempty"`
}

// Provider is the unified interface implemented by OpenAI, Anthropic, Gemini, OpenAI-Compat, and Mock.
type Provider interface {
	ID() string
	Kind() string
	StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error)
	TestConnection(ctx context.Context, model string) error
}
