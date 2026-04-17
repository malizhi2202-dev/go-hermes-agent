package llm

import (
	"context"
	"fmt"
	"strings"

	"go-hermes-agent/internal/config"
)

// AuxiliaryResolution describes how one side-task model was resolved.
type AuxiliaryResolution struct {
	Task    string           `json:"task"`
	Profile string           `json:"profile"`
	Source  string           `json:"source"`
	Config  config.LLMConfig `json:"config"`
}

// AuxiliaryRouter resolves auxiliary model usage for side tasks like summary or compression.
type AuxiliaryRouter struct {
	current func() config.Config
}

// NewAuxiliaryRouter creates a lightweight resolver over the current app config.
func NewAuxiliaryRouter(current func() config.Config) *AuxiliaryRouter {
	return &AuxiliaryRouter{current: current}
}

// Resolve picks the current auxiliary model config for one task.
func (r *AuxiliaryRouter) Resolve(task string) (AuxiliaryResolution, error) {
	cfg := r.current()
	if !cfg.Auxiliary.Enabled {
		return AuxiliaryResolution{Task: task, Profile: cfg.CurrentModelProfile, Source: "main", Config: cfg.ResolvedLLM()}, nil
	}
	selectedProfile := strings.TrimSpace(cfg.Auxiliary.Profile)
	source := "auxiliary.profile"
	switch strings.ToLower(strings.TrimSpace(task)) {
	case "summary":
		if strings.TrimSpace(cfg.Auxiliary.SummaryProfile) != "" {
			selectedProfile = strings.TrimSpace(cfg.Auxiliary.SummaryProfile)
			source = "auxiliary.summary_profile"
		}
	case "compression":
		if strings.TrimSpace(cfg.Auxiliary.CompressionProfile) != "" {
			selectedProfile = strings.TrimSpace(cfg.Auxiliary.CompressionProfile)
			source = "auxiliary.compression_profile"
		}
	}
	if selectedProfile == "" {
		return AuxiliaryResolution{Task: task, Profile: cfg.CurrentModelProfile, Source: "main", Config: cfg.ResolvedLLM()}, nil
	}
	profile, ok := cfg.ModelProfiles[selectedProfile]
	if !ok {
		return AuxiliaryResolution{}, fmt.Errorf("auxiliary profile %q is not defined", selectedProfile)
	}
	return AuxiliaryResolution{Task: task, Profile: selectedProfile, Source: source, Config: profile}, nil
}

// ClientFor creates a client for the resolved auxiliary task.
func (r *AuxiliaryRouter) ClientFor(task string) (*Client, AuxiliaryResolution, error) {
	resolution, err := r.Resolve(task)
	if err != nil {
		return nil, AuxiliaryResolution{}, err
	}
	return NewFromLLMConfig(resolution.Config), resolution, nil
}

// Chat runs one auxiliary side-task call using the resolved model.
func (r *AuxiliaryRouter) Chat(ctx context.Context, task string, systemBlocks []string, prompt string) (string, AuxiliaryResolution, error) {
	client, resolution, err := r.ClientFor(task)
	if err != nil {
		return "", AuxiliaryResolution{}, err
	}
	response, err := client.ChatWithContext(ctx, systemBlocks, prompt)
	return response, resolution, err
}
