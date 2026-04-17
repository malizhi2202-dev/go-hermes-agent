package llm

import (
	"testing"

	"go-hermes-agent/internal/config"
)

func TestAuxiliaryRouterResolve(t *testing.T) {
	cfg := config.Default()
	cfg.Auxiliary.Enabled = true
	cfg.Auxiliary.Profile = "openrouter-claude-sonnet"
	cfg.Auxiliary.CompressionProfile = "ollama-qwen3"
	router := NewAuxiliaryRouter(func() config.Config { return cfg })

	compression, err := router.Resolve("compression")
	if err != nil {
		t.Fatalf("resolve compression auxiliary: %v", err)
	}
	if compression.Profile != "ollama-qwen3" {
		t.Fatalf("unexpected compression profile: %#v", compression)
	}

	summary, err := router.Resolve("summary")
	if err != nil {
		t.Fatalf("resolve summary auxiliary: %v", err)
	}
	if summary.Profile != "openrouter-claude-sonnet" {
		t.Fatalf("unexpected summary profile: %#v", summary)
	}
}
