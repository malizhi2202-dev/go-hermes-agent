package models

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"hermes-agent/go/internal/config"
)

// Catalog stores model profile aliases and discovery helpers.
type Catalog struct {
	Aliases map[string]string
}

// DiscoveredModel is one model found on a local or OpenAI-compatible endpoint.
type DiscoveredModel struct {
	Source      string           `json:"source"`
	ProfileName string           `json:"profile_name"`
	DisplayName string           `json:"display_name"`
	Model       string           `json:"model"`
	BaseURL     string           `json:"base_url"`
	Local       bool             `json:"local"`
	Config      config.LLMConfig `json:"config"`
}

// DefaultCatalog returns the built-in alias catalog.
func DefaultCatalog() Catalog {
	return Catalog{
		Aliases: map[string]string{
			"gpt":        "openai-gpt41-mini",
			"openai":     "openai-gpt41-mini",
			"claude":     "openrouter-claude-sonnet",
			"sonnet":     "openrouter-claude-sonnet",
			"qwen":       "ollama-qwen3",
			"ollama":     "ollama-qwen3",
			"lmstudio":   "lmstudio-local",
			"local":      "lmstudio-local",
			"local-qwen": "ollama-qwen3",
		},
	}
}

// ResolveProfile resolves a profile name from aliases, display names, or models.
func (c Catalog) ResolveProfile(profiles map[string]config.LLMConfig, raw string) (string, bool) {
	name := strings.TrimSpace(strings.ToLower(raw))
	if name == "" {
		return "", false
	}
	if _, ok := profiles[name]; ok {
		return name, true
	}
	if alias, ok := c.Aliases[name]; ok {
		if _, ok := profiles[alias]; ok {
			return alias, true
		}
	}
	names := make([]string, 0, len(profiles))
	for profileName, profile := range profiles {
		names = append(names, profileName)
		text := strings.ToLower(strings.TrimSpace(profile.DisplayName + " " + profile.Model))
		if strings.Contains(text, name) {
			return profileName, true
		}
	}
	sort.Strings(names)
	for _, profileName := range names {
		if strings.Contains(profileName, name) {
			return profileName, true
		}
	}
	return "", false
}

// DiscoverLocalModels scans local-compatible profiles and discovers available models.
func DiscoverLocalModels(ctx context.Context, profiles map[string]config.LLMConfig) ([]DiscoveredModel, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	discovered := make([]DiscoveredModel, 0)
	seen := make(map[string]struct{})
	for _, profile := range profiles {
		if !profile.Local && !isLocalBaseURL(profile.BaseURL) {
			continue
		}
		baseURL := strings.TrimSpace(profile.BaseURL)
		if baseURL == "" {
			continue
		}
		if _, err := url.Parse(baseURL); err != nil {
			continue
		}
		if items, err := discoverOllama(ctx, client, profile); err == nil {
			for _, item := range items {
				key := item.Source + ":" + item.Model + ":" + item.BaseURL
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				discovered = append(discovered, item)
			}
		}
		if items, err := discoverOpenAICompatibleModels(ctx, client, "lmstudio", profile); err == nil {
			for _, item := range items {
				key := item.Source + ":" + item.Model + ":" + item.BaseURL
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				discovered = append(discovered, item)
			}
		}
	}
	sort.Slice(discovered, func(i, j int) bool {
		if discovered[i].Source == discovered[j].Source {
			return discovered[i].Model < discovered[j].Model
		}
		return discovered[i].Source < discovered[j].Source
	})
	return discovered, nil
}

func discoverOllama(ctx context.Context, client *http.Client, profile config.LLMConfig) ([]DiscoveredModel, error) {
	root, err := normalizeBase(profile.BaseURL)
	if err != nil {
		return nil, err
	}
	root.Path = "/api/tags"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, root.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ollama discovery failed with status %s", resp.Status)
	}
	var payload struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	items := make([]DiscoveredModel, 0, len(payload.Models))
	for _, model := range payload.Models {
		if strings.TrimSpace(model.Name) == "" {
			continue
		}
		cfg := profile
		cfg.Model = model.Name
		cfg.DisplayName = "Ollama " + model.Name
		cfg.Local = true
		items = append(items, DiscoveredModel{
			Source:      "ollama",
			ProfileName: "ollama-" + sanitizeProfileName(model.Name),
			DisplayName: cfg.DisplayName,
			Model:       model.Name,
			BaseURL:     profile.BaseURL,
			Local:       true,
			Config:      cfg,
		})
	}
	return items, nil
}

func discoverOpenAICompatibleModels(ctx context.Context, client *http.Client, source string, profile config.LLMConfig) ([]DiscoveredModel, error) {
	root, err := normalizeBase(profile.BaseURL)
	if err != nil {
		return nil, err
	}
	root.Path = path.Join(strings.TrimRight(root.Path, "/"), "/models")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, root.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s discovery failed with status %s", source, resp.Status)
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	items := make([]DiscoveredModel, 0, len(payload.Data))
	for _, model := range payload.Data {
		if strings.TrimSpace(model.ID) == "" {
			continue
		}
		cfg := profile
		cfg.Model = model.ID
		cfg.DisplayName = strings.ToUpper(source[:1]) + source[1:] + " " + model.ID
		cfg.Local = true
		items = append(items, DiscoveredModel{
			Source:      source,
			ProfileName: source + "-" + sanitizeProfileName(model.ID),
			DisplayName: cfg.DisplayName,
			Model:       model.ID,
			BaseURL:     profile.BaseURL,
			Local:       true,
			Config:      cfg,
		})
	}
	return items, nil
}

func normalizeBase(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid base url %q", raw)
	}
	return u, nil
}

func sanitizeProfileName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	prevDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			prevDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		return "model"
	}
	return result
}

func isLocalBaseURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "127.0.0.1" || host == "localhost"
}
