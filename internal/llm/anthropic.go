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

// AnthropicProvider implements native Anthropic Messages API with ephemeral prompt caching
// on the Tier-0 cache-stable SystemPrefix.
type AnthropicProvider struct {
	id      string
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewAnthropicProvider(id, baseURL, apiKey string) *AnthropicProvider {
	base := strings.TrimRight(baseURL, "/")
	if base == "" {
		base = "https://api.anthropic.com"
	}
	return &AnthropicProvider{
		id:      id,
		baseURL: base,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 180 * time.Second},
	}
}

func (p *AnthropicProvider) ID() string   { return p.id }
func (p *AnthropicProvider) Kind() string { return "anthropic" }

func (p *AnthropicProvider) TestConnection(ctx context.Context, model string) error {
	if strings.TrimSpace(p.apiKey) == "" {
		return fmt.Errorf("Anthropic API key is empty")
	}
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}
	ch, err := p.StreamChat(ctx, ChatRequest{
		Model:     model,
		Messages:  []Message{{Role: "user", Content: "Hi"}},
		MaxTokens: 8,
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

func (p *AnthropicProvider) StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	payload := p.buildPayload(req)
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/messages", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		_ = resp.Body.Close()
		return nil, fmt.Errorf("anthropic HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(errBytes)))
	}

	out := make(chan StreamEvent, 32)
	go func() {
		defer close(out)
		defer resp.Body.Close()

		type activeTool struct {
			id   string
			name string
			json strings.Builder
		}
		toolsByIdx := map[int]*activeTool{}
		var totalUsage Usage

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" || data == "[DONE]" {
				continue
			}
			var ev struct {
				Type  string `json:"type"`
				Index int    `json:"index"`
				Error *struct {
					Message string `json:"message"`
				} `json:"error"`
				Message *struct {
					Usage struct {
						InputTokens              int `json:"input_tokens"`
						CacheReadInputTokens     int `json:"cache_read_input_tokens"`
						CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
					} `json:"usage"`
				} `json:"message"`
				ContentBlock *struct {
					Type string `json:"type"`
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"content_block"`
				Delta *struct {
					Type        string `json:"type"`
					Text        string `json:"text"`
					Thinking    string `json:"thinking"`
					PartialJSON string `json:"partial_json"`
				} `json:"delta"`
				Usage *struct {
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			}
			if json.Unmarshal([]byte(data), &ev) != nil {
				continue
			}
			switch ev.Type {
			case "error":
				if ev.Error != nil {
					out <- StreamEvent{Type: "error", Error: ev.Error.Message}
				}
			case "message_start":
				if ev.Message != nil {
					totalUsage.InputTokens += ev.Message.Usage.InputTokens + ev.Message.Usage.CacheCreationInputTokens
					totalUsage.CachedTokens += ev.Message.Usage.CacheReadInputTokens
				}
			case "content_block_start":
				if ev.ContentBlock != nil && ev.ContentBlock.Type == "tool_use" {
					toolsByIdx[ev.Index] = &activeTool{id: ev.ContentBlock.ID, name: ev.ContentBlock.Name}
				}
			case "content_block_delta":
				if ev.Delta != nil {
					switch ev.Delta.Type {
					case "text_delta":
						out <- StreamEvent{Type: "delta", Delta: ev.Delta.Text}
					case "thinking_delta":
						out <- StreamEvent{Type: "thinking", ThinkingDelta: ev.Delta.Thinking}
					case "input_json_delta":
						if at, ok := toolsByIdx[ev.Index]; ok {
							at.json.WriteString(ev.Delta.PartialJSON)
						}
					}
				}
			case "content_block_stop":
				if at, ok := toolsByIdx[ev.Index]; ok {
					raw := strings.TrimSpace(at.json.String())
					if raw == "" {
						raw = "{}"
					}
					out <- StreamEvent{
						Type: "tool_call",
						ToolCall: &ToolCall{
							ID:        at.id,
							Name:      at.name,
							Arguments: json.RawMessage(raw),
						},
					}
					delete(toolsByIdx, ev.Index)
				}
			case "message_delta":
				if ev.Usage != nil {
					totalUsage.OutputTokens = ev.Usage.OutputTokens
				}
			}
		}

		if totalUsage.InputTokens > 0 || totalUsage.OutputTokens > 0 || totalUsage.CachedTokens > 0 {
			out <- StreamEvent{Type: "usage", Usage: &totalUsage}
		}
		if err := scanner.Err(); err != nil && ctx.Err() == nil {
			out <- StreamEvent{Type: "error", Error: err.Error()}
			return
		}
		out <- StreamEvent{Type: "done"}
	}()
	return out, nil
}

func (p *AnthropicProvider) buildPayload(req ChatRequest) map[string]any {
	var sysBlocks []map[string]any
	if strings.TrimSpace(req.SystemPrefix) != "" {
		b := map[string]any{
			"type": "text",
			"text": req.SystemPrefix,
		}
		if req.EnableCache {
			b["cache_control"] = map[string]any{"type": "ephemeral"}
		}
		sysBlocks = append(sysBlocks, b)
	}
	if strings.TrimSpace(req.DynamicContext) != "" {
		sysBlocks = append(sysBlocks, map[string]any{
			"type": "text",
			"text": req.DynamicContext,
		})
	}

	var msgs []map[string]any
	for _, m := range req.Messages {
		switch m.Role {
		case "user":
			msgs = append(msgs, map[string]any{
				"role":    "user",
				"content": m.Content,
			})
		case "assistant":
			var blocks []map[string]any
			if m.Content != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": m.Content})
			}
			for _, tc := range m.ToolCalls {
				var inputObj any
				_ = json.Unmarshal(tc.Arguments, &inputObj)
				if inputObj == nil {
					inputObj = map[string]any{}
				}
				blocks = append(blocks, map[string]any{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Name,
					"input": inputObj,
				})
			}
			if len(blocks) > 0 {
				msgs = append(msgs, map[string]any{"role": "assistant", "content": blocks})
			}
		case "tool":
			// Anthropic expects tool_result blocks inside a user turn
			block := map[string]any{
				"type":        "tool_result",
				"tool_use_id": m.ToolCallID,
				"content":     m.Content,
			}
			if len(msgs) > 0 && msgs[len(msgs)-1]["role"] == "user" {
				if arr, ok := msgs[len(msgs)-1]["content"].([]map[string]any); ok {
					msgs[len(msgs)-1]["content"] = append(arr, block)
					continue
				}
			}
			msgs = append(msgs, map[string]any{
				"role":    "user",
				"content": []map[string]any{block},
			})
		}
	}

	maxTok := req.MaxTokens
	if maxTok <= 0 {
		maxTok = 4096
	}
	payload := map[string]any{
		"model":      req.Model,
		"max_tokens": maxTok,
		"stream":     true,
		"messages":   msgs,
	}
	if len(sysBlocks) > 0 {
		payload["system"] = sysBlocks
	}
	if req.Temperature > 0 {
		payload["temperature"] = req.Temperature
	}
	if len(req.Tools) > 0 {
		var tools []map[string]any
		for _, t := range req.Tools {
			tools = append(tools, map[string]any{
				"name":         t.Name,
				"description":  t.Description,
				"input_schema": t.Parameters,
			})
		}
		payload["tools"] = tools
	}
	return payload
}
