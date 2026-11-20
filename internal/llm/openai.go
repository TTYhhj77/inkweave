package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAICompatProvider supports OpenAI as well as OpenRouter, DeepSeek, Groq,
// Together, LM Studio, Ollama, and any OpenAI Chat Completions compatible URL.
type OpenAICompatProvider struct {
	id      string
	kind    string
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewOpenAICompatProvider(id, kind, baseURL, apiKey string) *OpenAICompatProvider {
	base := strings.TrimRight(baseURL, "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	return &OpenAICompatProvider{
		id:      id,
		kind:    kind,
		baseURL: base,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 180 * time.Second},
	}
}

func (p *OpenAICompatProvider) ID() string   { return p.id }
func (p *OpenAICompatProvider) Kind() string { return p.kind }

func (p *OpenAICompatProvider) TestConnection(ctx context.Context, model string) error {
	if p.kind == "openai" && strings.TrimSpace(p.apiKey) == "" {
		return fmt.Errorf("API key is empty")
	}
	// Lightweight probe using models list or tiny completion
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/models", nil)
	if err != nil {
		return err
	}
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	// Some compatible endpoints disable GET /models; fallback to 1-token chat check
	if model != "" {
		ch, err := p.StreamChat(ctx, ChatRequest{
			Model:     model,
			Messages:  []Message{{Role: "user", Content: "ping"}},
			MaxTokens: 4,
		})
		if err != nil {
			return err
		}
		for ev := range ch {
			if ev.Type == "error" {
				return fmt.Errorf("%s", ev.Error)
			}
		}
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}

func (p *OpenAICompatProvider) StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	payload := p.buildPayload(req)
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	var resp *http.Response
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		if p.apiKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
		}
		if strings.Contains(p.baseURL, "openrouter.ai") {
			httpReq.Header.Set("HTTP-Referer", "https://github.com/inkweave/inkweave")
			httpReq.Header.Set("X-Title", "Inkweave Novel Studio")
		}
		resp, lastErr = p.client.Do(httpReq)
		if lastErr == nil && resp.StatusCode != 429 && resp.StatusCode < 500 {
			break
		}
		if resp != nil {
			if attempt == 2 {
				errBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
				_ = resp.Body.Close()
				return nil, fmt.Errorf("openai-compat HTTP %d: %s", resp.StatusCode, string(errBytes))
			}
			_ = resp.Body.Close()
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 500 * time.Millisecond):
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		_ = resp.Body.Close()
		return nil, fmt.Errorf("openai-compat HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(errBytes)))
	}

	out := make(chan StreamEvent, 32)
	go func() {
		defer close(out)
		defer resp.Body.Close()

		type partialTool struct {
			id   string
			name string
			args strings.Builder
		}
		toolsByIdx := map[int]*partialTool{}

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				break
			}
			var chunk struct {
				Choices []struct {
					Delta struct {
						Content          string `json:"content"`
						ReasoningContent string `json:"reasoning_content"`
						ToolCalls        []struct {
							Index    int    `json:"index"`
							ID       string `json:"id"`
							Function struct {
								Name      string `json:"name"`
								Arguments string `json:"arguments"`
							} `json:"function"`
						} `json:"tool_calls"`
					} `json:"delta"`
					FinishReason *string `json:"finish_reason"`
				} `json:"choices"`
				Usage *struct {
					PromptTokens        int `json:"prompt_tokens"`
					CompletionTokens    int `json:"completion_tokens"`
					PromptTokensDetails *struct {
						CachedTokens int `json:"cached_tokens"`
					} `json:"prompt_tokens_details"`
				} `json:"usage"`
			}
			if json.Unmarshal([]byte(data), &chunk) != nil {
				continue
			}
			if chunk.Usage != nil {
				cached := 0
				if chunk.Usage.PromptTokensDetails != nil {
					cached = chunk.Usage.PromptTokensDetails.CachedTokens
				}
				out <- StreamEvent{
					Type: "usage",
					Usage: &Usage{
						InputTokens:  chunk.Usage.PromptTokens,
						CachedTokens: cached,
						OutputTokens: chunk.Usage.CompletionTokens,
					},
				}
			}
			for _, ch := range chunk.Choices {
				if ch.Delta.ReasoningContent != "" {
					out <- StreamEvent{Type: "thinking", ThinkingDelta: ch.Delta.ReasoningContent}
				}
				if ch.Delta.Content != "" {
					out <- StreamEvent{Type: "delta", Delta: ch.Delta.Content}
				}
				for _, tc := range ch.Delta.ToolCalls {
					pt, ok := toolsByIdx[tc.Index]
					if !ok {
						pt = &partialTool{}
						toolsByIdx[tc.Index] = pt
					}
					if tc.ID != "" {
						pt.id = tc.ID
					}
					if tc.Function.Name != "" {
						pt.name = tc.Function.Name
					}
					if tc.Function.Arguments != "" {
						pt.args.WriteString(tc.Function.Arguments)
					}
				}
			}
		}

		// Emit assembled tool calls sorted by index
		for i := 0; i < len(toolsByIdx); i++ {
			if pt, ok := toolsByIdx[i]; ok && pt.name != "" {
				rawArgs := strings.TrimSpace(pt.args.String())
				if rawArgs == "" {
					rawArgs = "{}"
				}
				out <- StreamEvent{
					Type: "tool_call",
					ToolCall: &ToolCall{
						ID:        pt.id,
						Name:      pt.name,
						Arguments: json.RawMessage(rawArgs),
					},
				}
			}
		}
		if err := scanner.Err(); err != nil && ctx.Err() == nil {
			out <- StreamEvent{Type: "error", Error: err.Error()}
			return
		}
		out <- StreamEvent{Type: "done"}
	}()
	return out, nil
}

func (p *OpenAICompatProvider) buildPayload(req ChatRequest) map[string]any {
	var msgs []map[string]any
	if strings.TrimSpace(req.SystemPrefix) != "" {
		msgs = append(msgs, map[string]any{
			"role":    "system",
			"content": req.SystemPrefix,
		})
	}
	if strings.TrimSpace(req.DynamicContext) != "" {
		msgs = append(msgs, map[string]any{
			"role":    "system",
			"content": req.DynamicContext,
		})
	}
	for _, m := range req.Messages {
		msg := map[string]any{"role": m.Role}
		if m.Content != "" || m.Role != "assistant" {
			msg["content"] = m.Content
		}
		if m.ToolCallID != "" {
			msg["tool_call_id"] = m.ToolCallID
		}
		if len(m.ToolCalls) > 0 {
			var tcs []map[string]any
			for _, tc := range m.ToolCalls {
				tcs = append(tcs, map[string]any{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]any{
						"name":      tc.Name,
						"arguments": string(tc.Arguments),
					},
				})
			}
			msg["tool_calls"] = tcs
		}
		msgs = append(msgs, msg)
	}

	payload := map[string]any{
		"model":    req.Model,
		"messages": msgs,
		"stream":   true,
	}
	// Avoid sending stream_options to local Ollama endpoints if unsupported, send for cloud providers
	if !strings.Contains(p.baseURL, "11434") {
		payload["stream_options"] = map[string]any{"include_usage": true}
	}
	if req.MaxTokens > 0 {
		payload["max_tokens"] = req.MaxTokens
	}
	if req.Temperature > 0 {
		payload["temperature"] = req.Temperature
	}
	if len(req.Tools) > 0 {
		var tools []map[string]any
		for _, t := range req.Tools {
			tools = append(tools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  t.Parameters,
				},
			})
		}
		payload["tools"] = tools
	}
	return payload
}
