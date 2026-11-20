package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

// GeminiProvider implements Google Gemini v1beta streaming with function declarations.
type GeminiProvider struct {
	id      string
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewGeminiProvider(id, baseURL, apiKey string) *GeminiProvider {
	base := strings.TrimRight(baseURL, "/")
	if base == "" {
		base = "https://generativelanguage.googleapis.com"
	}
	return &GeminiProvider{
		id:      id,
		baseURL: base,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 180 * time.Second},
	}
}

func (p *GeminiProvider) ID() string   { return p.id }
func (p *GeminiProvider) Kind() string { return "gemini" }

func (p *GeminiProvider) TestConnection(ctx context.Context, model string) error {
	if strings.TrimSpace(p.apiKey) == "" {
		return fmt.Errorf("Gemini API key is empty")
	}
	endpoint := fmt.Sprintf("%s/v1beta/models?key=%s", p.baseURL, url.QueryEscape(p.apiKey))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("gemini HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}

func (p *GeminiProvider) StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	modelName := strings.TrimPrefix(req.Model, "models/")
	if modelName == "" {
		modelName = "gemini-2.5-flash"
	}
	endpoint := fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?alt=sse&key=%s",
		p.baseURL, url.PathEscape(modelName), url.QueryEscape(p.apiKey))

	payload := p.buildPayload(req)
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		_ = resp.Body.Close()
		return nil, fmt.Errorf("gemini HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(errBytes)))
	}

	out := make(chan StreamEvent, 32)
	go func() {
		defer close(out)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" {
				continue
			}
			var chunk struct {
				Candidates []struct {
					Content struct {
						Parts []struct {
							Text         string `json:"text"`
							Thought      bool   `json:"thought"`
							FunctionCall *struct {
								Name string         `json:"name"`
								Args map[string]any `json:"args"`
							} `json:"functionCall"`
						} `json:"parts"`
					} `json:"content"`
				} `json:"candidates"`
				UsageMetadata *struct {
					PromptTokenCount        int `json:"promptTokenCount"`
					CachedContentTokenCount int `json:"cachedContentTokenCount"`
					CandidatesTokenCount    int `json:"candidatesTokenCount"`
				} `json:"usageMetadata"`
			}
			if json.Unmarshal([]byte(data), &chunk) != nil {
				continue
			}
			if chunk.UsageMetadata != nil {
				out <- StreamEvent{
					Type: "usage",
					Usage: &Usage{
						InputTokens:  chunk.UsageMetadata.PromptTokenCount,
						CachedTokens: chunk.UsageMetadata.CachedContentTokenCount,
						OutputTokens: chunk.UsageMetadata.CandidatesTokenCount,
					},
				}
			}
			for _, cand := range chunk.Candidates {
				for _, part := range cand.Content.Parts {
					if part.Text != "" {
						if part.Thought {
							out <- StreamEvent{Type: "thinking", ThinkingDelta: part.Text}
						} else {
							out <- StreamEvent{Type: "delta", Delta: part.Text}
						}
					}
					if part.FunctionCall != nil {
						argsJSON, _ := json.Marshal(part.FunctionCall.Args)
						out <- StreamEvent{
							Type: "tool_call",
							ToolCall: &ToolCall{
								ID:        "gem-" + uuid.NewString()[:8],
								Name:      part.FunctionCall.Name,
								Arguments: argsJSON,
							},
						}
					}
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

func (p *GeminiProvider) buildPayload(req ChatRequest) map[string]any {
	sysJoined := strings.TrimSpace(strings.Join([]string{req.SystemPrefix, req.DynamicContext}, "\n\n"))
	var contents []map[string]any
	for _, m := range req.Messages {
		switch m.Role {
		case "user":
			contents = append(contents, map[string]any{
				"role":  "user",
				"parts": []map[string]any{{"text": m.Content}},
			})
		case "assistant":
			var parts []map[string]any
			if m.Content != "" {
				parts = append(parts, map[string]any{"text": m.Content})
			}
			for _, tc := range m.ToolCalls {
				var args map[string]any
				_ = json.Unmarshal(tc.Arguments, &args)
				if args == nil {
					args = map[string]any{}
				}
				parts = append(parts, map[string]any{
					"functionCall": map[string]any{
						"name": tc.Name,
						"args": args,
					},
				})
			}
			if len(parts) > 0 {
				contents = append(contents, map[string]any{"role": "model", "parts": parts})
			}
		case "tool":
			var respObj map[string]any
			if json.Unmarshal([]byte(m.Content), &respObj) != nil {
				respObj = map[string]any{"result": m.Content}
			}
			contents = append(contents, map[string]any{
				"role": "function",
				"parts": []map[string]any{
					{
						"functionResponse": map[string]any{
							"name":     m.ToolName,
							"response": respObj,
						},
					},
				},
			})
		}
	}

	payload := map[string]any{
		"contents": contents,
	}
	if sysJoined != "" {
		payload["systemInstruction"] = map[string]any{
			"parts": []map[string]any{{"text": sysJoined}},
		}
	}
	genCfg := map[string]any{}
	if req.Temperature > 0 {
		genCfg["temperature"] = req.Temperature
	}
	if req.MaxTokens > 0 {
		genCfg["maxOutputTokens"] = req.MaxTokens
	}
	if len(genCfg) > 0 {
		payload["generationConfig"] = genCfg
	}
	if len(req.Tools) > 0 {
		var decls []map[string]any
		for _, t := range req.Tools {
			decls = append(decls, map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.Parameters,
			})
		}
		payload["tools"] = []map[string]any{{"functionDeclarations": decls}}
	}
	return payload
}
