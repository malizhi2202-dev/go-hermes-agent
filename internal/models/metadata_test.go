package models

import (
	"testing"

	"go-hermes-agent/internal/config"
)

func TestResolveMetadata(t *testing.T) {
	meta := ResolveMetadata(config.LLMConfig{Provider: "openai-compatible", Model: "gpt-4.1-mini", DisplayName: "GPT", Local: false})
	if meta.ContextWindow == 0 || !meta.SupportsPromptCache || !meta.SupportsTools {
		t.Fatalf("unexpected metadata: %#v", meta)
	}
	local := ResolveMetadata(config.LLMConfig{Provider: "openai-compatible", Model: "qwen3:14b", Local: true})
	if !local.Local || local.ContextWindow == 0 {
		t.Fatalf("unexpected local metadata: %#v", local)
	}
}
