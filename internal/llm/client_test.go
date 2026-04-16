package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"hermes-agent/go/internal/config"
)

func TestChatCompletionParsesNativeToolCalls(t *testing.T) {
	var seen struct {
		Tools []map[string]any `json:"tools"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&seen); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"finish_reason": "tool_calls",
					"message": map[string]any{
						"role":    "assistant",
						"content": "",
						"tool_calls": []map[string]any{
							{
								"id":   "call_1",
								"type": "function",
								"function": map[string]any{
									"name":      "session.search",
									"arguments": `{"query":"alpha","limit":"2"}`,
								},
							},
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	cfg := config.Default()
	cfg.CurrentModelProfile = ""
	cfg.LLM.BaseURL = server.URL
	cfg.LLM.Model = "test-model"
	cfg.LLM.APIKeyEnv = ""
	client := New(cfg)

	completion, err := client.ChatCompletion(context.Background(), []string{"system"}, nil, "find alpha", []ToolDefinition{
		{
			Type: "function",
			Function: ToolFunctionDefinition{
				Name:        "session.search",
				Description: "Search history",
				Parameters: map[string]any{
					"type": "object",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("chat completion: %v", err)
	}
	if len(seen.Tools) != 1 {
		t.Fatalf("expected tool definitions to be sent, got %#v", seen.Tools)
	}
	if completion.FinishReason != "tool_calls" {
		t.Fatalf("unexpected finish reason: %q", completion.FinishReason)
	}
	if len(completion.Message.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %#v", completion.Message.ToolCalls)
	}
	if completion.Message.ToolCalls[0].Function.Name != "session.search" {
		t.Fatalf("unexpected tool call: %#v", completion.Message.ToolCalls[0])
	}
}
