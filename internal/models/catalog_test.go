package models

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go-hermes-agent/internal/config"
)

func TestResolveProfileSupportsAliases(t *testing.T) {
	cfg := config.Default()
	resolved, ok := DefaultCatalog().ResolveProfile(cfg.ListModelProfiles(), "sonnet")
	if !ok {
		t.Fatal("expected alias resolution")
	}
	if resolved != "openrouter-claude-sonnet" {
		t.Fatalf("unexpected resolved profile: %s", resolved)
	}
}

func TestDiscoverLocalModelsFindsOllamaAndLMStudio(t *testing.T) {
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]any{
				{"name": "qwen3:14b"},
				{"name": "deepseek-r1:8b"},
			},
		})
	}))
	defer ollama.Close()

	lmstudio := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "local-gpt"},
			},
		})
	}))
	defer lmstudio.Close()

	profiles := map[string]config.LLMConfig{
		"ollama-qwen3": {
			BaseURL: ollama.URL + "/v1",
			Local:   true,
		},
		"lmstudio-local": {
			BaseURL: lmstudio.URL + "/v1",
			Local:   true,
		},
	}
	items, err := DiscoverLocalModels(context.Background(), profiles)
	if err != nil {
		t.Fatalf("discover local models: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 discovered models, got %d", len(items))
	}
}
