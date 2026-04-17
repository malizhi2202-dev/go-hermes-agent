package models

import (
	"strings"

	"go-hermes-agent/internal/config"
)

// Metadata is the lightweight Go counterpart to Python's model_metadata + models_dev subset.
type Metadata struct {
	Provider            string `json:"provider"`
	Model               string `json:"model"`
	DisplayName         string `json:"display_name,omitempty"`
	ContextWindow       int    `json:"context_window"`
	MaxOutput           int    `json:"max_output"`
	SupportsTools       bool   `json:"supports_tools"`
	SupportsVision      bool   `json:"supports_vision"`
	SupportsReasoning   bool   `json:"supports_reasoning"`
	SupportsPromptCache bool   `json:"supports_prompt_cache"`
	Local               bool   `json:"local"`
	Source              string `json:"source"`
	Notes               string `json:"notes,omitempty"`
}

// ResolveMetadata returns lightweight provider-aware metadata for one model config.
func ResolveMetadata(cfg config.LLMConfig) Metadata {
	meta := Metadata{
		Provider:      cfg.Provider,
		Model:         cfg.Model,
		DisplayName:   cfg.DisplayName,
		ContextWindow: 128000,
		MaxOutput:     8192,
		SupportsTools: true,
		Local:         cfg.Local,
		Source:        "builtin-lightweight-registry",
	}
	model := strings.ToLower(strings.TrimSpace(cfg.Model))
	switch {
	case strings.Contains(model, "gpt-4.1"):
		meta.ContextWindow = 1047576
		meta.MaxOutput = 32768
		meta.SupportsVision = true
		meta.SupportsReasoning = true
		meta.SupportsPromptCache = true
		meta.Notes = "OpenAI GPT-4.1 family; strong general tool use and large context."
	case strings.Contains(model, "claude") || strings.Contains(model, "sonnet"):
		meta.ContextWindow = 200000
		meta.MaxOutput = 8192
		meta.SupportsVision = true
		meta.SupportsReasoning = true
		meta.SupportsPromptCache = true
		meta.Notes = "Anthropic Claude family; good long-context and prompt-caching fit."
	case strings.Contains(model, "qwen"):
		meta.ContextWindow = 131072
		meta.MaxOutput = 8192
		meta.SupportsVision = strings.Contains(model, "vl")
		meta.SupportsReasoning = true
		meta.Notes = "Qwen family; common local deployment target for lightweight setups."
	case strings.Contains(model, "deepseek"):
		meta.ContextWindow = 128000
		meta.MaxOutput = 8192
		meta.SupportsReasoning = true
		meta.Notes = "DeepSeek family; strong reasoning-oriented model line."
	case strings.Contains(model, "local-model"):
		meta.ContextWindow = 32768
		meta.MaxOutput = 4096
		meta.Notes = "Generic local OpenAI-compatible endpoint; values are conservative defaults."
	default:
		meta.Notes = "Fallback metadata for an OpenAI-compatible endpoint."
	}
	return meta
}

// ListProfileMetadata resolves metadata for every configured profile.
func ListProfileMetadata(profiles map[string]config.LLMConfig) map[string]Metadata {
	result := make(map[string]Metadata, len(profiles))
	for name, profile := range profiles {
		result[name] = ResolveMetadata(profile)
	}
	return result
}
