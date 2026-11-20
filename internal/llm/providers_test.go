package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIStreamAndToolParsing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing bearer auth")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"สวัสดี\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"function\":{\"name\":\"read_chapter\",\"arguments\":\"{\\\"id\\\":\\\"ch-001\\\"}\"}}]}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":25,\"prompt_tokens_details\":{\"cached_tokens\":60}}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	p := NewOpenAICompatProvider("test-oai", "openai", srv.URL, "test-key")
	ch, err := p.StreamChat(context.Background(), ChatRequest{
		Model:    "gpt-4.1",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}

	var gotText string
	var gotTool *ToolCall
	var gotUsage *Usage
	for ev := range ch {
		switch ev.Type {
		case "delta":
			gotText += ev.Delta
		case "tool_call":
			gotTool = ev.ToolCall
		case "usage":
			gotUsage = ev.Usage
		case "error":
			t.Fatalf("unexpected error event: %s", ev.Error)
		}
	}
	if gotText != "สวัสดี" {
		t.Fatalf("gotText = %q", gotText)
	}
	if gotTool == nil || gotTool.Name != "read_chapter" {
		t.Fatalf("gotTool = %+v", gotTool)
	}
	if gotUsage == nil || gotUsage.CachedTokens != 60 {
		t.Fatalf("gotUsage = %+v", gotUsage)
	}
}

func TestAnthropicPromptCacheHeaderAndStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		sysArr, _ := req["system"].([]any)
		if len(sysArr) == 0 {
			t.Fatalf("expected system blocks")
		}
		firstBlock := sysArr[0].(map[string]any)
		if firstBlock["cache_control"] == nil {
			t.Fatalf("expected cache_control on Tier-0 SystemPrefix block")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":50,\"cache_read_input_tokens\":900}}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"หมอกบาง\"}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":14}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"message_stop\"}\n\n")
	}))
	defer srv.Close()

	p := NewAnthropicProvider("anth", srv.URL, "sk-ant")
	ch, err := p.StreamChat(context.Background(), ChatRequest{
		Model:        "claude-sonnet-4-20250514",
		SystemPrefix: "Story DNA",
		EnableCache:  true,
		Messages:     []Message{{Role: "user", Content: "Draft"}},
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	var text string
	var cached int
	for ev := range ch {
		if ev.Type == "delta" {
			text += ev.Delta
		}
		if ev.Type == "usage" && ev.Usage != nil {
			cached = ev.Usage.CachedTokens
		}
	}
	if text != "หมอกบาง" || cached != 900 {
		t.Fatalf("text=%q cached=%d", text, cached)
	}
}

func TestGeminiStreamParsing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "alt=sse") {
			t.Errorf("expected alt=sse query")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"แสงไฟท่าเรือ\"}]}}],\"usageMetadata\":{\"promptTokenCount\":220,\"cachedContentTokenCount\":100,\"candidatesTokenCount\":18}}\n\n")
	}))
	defer srv.Close()

	p := NewGeminiProvider("gem", srv.URL, "gem-key")
	ch, err := p.StreamChat(context.Background(), ChatRequest{
		Model:    "gemini-2.5-flash",
		Messages: []Message{{Role: "user", Content: "Write"}},
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	var got string
	var u *Usage
	for ev := range ch {
		if ev.Type == "delta" {
			got += ev.Delta
		}
		if ev.Type == "usage" {
			u = ev.Usage
		}
	}
	if got != "แสงไฟท่าเรือ" || u == nil || u.CachedTokens != 100 {
		t.Fatalf("got=%q usage=%+v", got, u)
	}
}
