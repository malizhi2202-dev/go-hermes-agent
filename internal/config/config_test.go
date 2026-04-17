package config

import (
	"path/filepath"
	"testing"
)

func TestDefaultConfigValidate(t *testing.T) {
	cfg := Default()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}
}

func TestInvalidPasswordLengthFails(t *testing.T) {
	cfg := Default()
	cfg.Security.MinPasswordLength = 8
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for weak min password length")
	}
}

func TestEnabledMCPServerRequiresCommand(t *testing.T) {
	cfg := Default()
	cfg.MCPServers = map[string]MCPServerConfig{
		"broken": {
			Enabled:        true,
			TimeoutSeconds: 5,
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for missing mcp command")
	}
}

func TestExecutionProfileRequiresSteps(t *testing.T) {
	cfg := Default()
	cfg.Execution.Profiles = map[string]ExecutionProfile{
		"broken": {},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for empty execution profile")
	}
}

func TestInvalidContextConfigFails(t *testing.T) {
	cfg := Default()
	cfg.Context.HistoryWindowMessages = -1
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for negative history window")
	}

	cfg = Default()
	cfg.Context.MaxPromptChars = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for non-positive max prompt chars")
	}

	cfg = Default()
	cfg.Context.CompressThresholdMessages = -1
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for negative compression threshold")
	}

	cfg = Default()
	cfg.Context.SummaryMaxChars = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for non-positive summary max chars")
	}

	cfg = Default()
	cfg.Context.SummaryStrategy = "broken"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for invalid summary strategy")
	}
}

func TestUseModelProfileSwitchesResolvedLLM(t *testing.T) {
	cfg := Default()
	if err := cfg.UseModelProfile("ollama-qwen3"); err != nil {
		t.Fatalf("use model profile: %v", err)
	}
	resolved := cfg.ResolvedLLM()
	if resolved.Model != "qwen3:14b" {
		t.Fatalf("unexpected model after switch: %s", resolved.Model)
	}
	if !resolved.Local {
		t.Fatal("expected local profile")
	}
}

func TestSaveAndLoadPreservesCurrentModelProfile(t *testing.T) {
	cfg := Default()
	if err := cfg.UseModelProfile("lmstudio-local"); err != nil {
		t.Fatalf("use model profile: %v", err)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.CurrentModelProfile != "lmstudio-local" {
		t.Fatalf("unexpected current model profile: %s", loaded.CurrentModelProfile)
	}
	if loaded.ResolvedLLM().BaseURL != "http://127.0.0.1:1234/v1" {
		t.Fatalf("unexpected resolved base url: %s", loaded.ResolvedLLM().BaseURL)
	}
}

func TestInvalidPromptingConfigFails(t *testing.T) {
	cfg := Default()
	cfg.Prompting.CacheTTLMinutes = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for non-positive prompt cache ttl")
	}
}

func TestInvalidCronConfigFails(t *testing.T) {
	cfg := Default()
	cfg.Cron.TickSeconds = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for non-positive cron tick seconds")
	}
}
