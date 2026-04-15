package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	AppName             string                     `yaml:"app_name"`
	ListenAddr          string                     `yaml:"listen_addr"`
	DataDir             string                     `yaml:"data_dir"`
	JWTIssuer           string                     `yaml:"jwt_issuer"`
	JWTExpiryMinutes    int                        `yaml:"jwt_expiry_minutes"`
	LLM                 LLMConfig                  `yaml:"llm"`
	CurrentModelProfile string                     `yaml:"current_model_profile"`
	ModelProfiles       map[string]LLMConfig       `yaml:"model_profiles"`
	Memory              MemoryConfig               `yaml:"memory"`
	Context             ContextConfig              `yaml:"context"`
	Security            SecurityConfig             `yaml:"security"`
	Server              ServerConfig               `yaml:"server"`
	Gateway             GatewayConfig              `yaml:"gateway"`
	Execution           ExecutionConfig            `yaml:"execution"`
	Extensions          ExtensionConfig            `yaml:"extensions"`
	MCPServers          map[string]MCPServerConfig `yaml:"mcp_servers"`
}

type LLMConfig struct {
	Provider       string `yaml:"provider"`
	Model          string `yaml:"model"`
	BaseURL        string `yaml:"base_url"`
	APIKeyEnv      string `yaml:"api_key_env"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
	DisplayName    string `yaml:"display_name"`
	Local          bool   `yaml:"local"`
}

type SecurityConfig struct {
	RequireAuth       bool `yaml:"require_auth"`
	AllowRegistration bool `yaml:"allow_registration"`
	MinPasswordLength int  `yaml:"min_password_length"`
	MaxLoginAttempts  int  `yaml:"max_login_attempts"`
	LoginWindowMinute int  `yaml:"login_window_minutes"`
}

type MemoryConfig struct {
	Enabled         bool `yaml:"enabled"`
	MemoryCharLimit int  `yaml:"memory_char_limit"`
	UserCharLimit   int  `yaml:"user_char_limit"`
	RecallLimit     int  `yaml:"recall_limit"`
}

type ContextConfig struct {
	HistoryWindowMessages     int    `yaml:"history_window_messages"`
	MaxPromptChars            int    `yaml:"max_prompt_chars"`
	CompressionEnabled        bool   `yaml:"compression_enabled"`
	CompressThresholdMessages int    `yaml:"compress_threshold_messages"`
	ProtectLastMessages       int    `yaml:"protect_last_messages"`
	SummaryMaxChars           int    `yaml:"summary_max_chars"`
	SummaryStrategy           string `yaml:"summary_strategy"`
}

type ServerConfig struct {
	ReadTimeoutSeconds  int `yaml:"read_timeout_seconds"`
	WriteTimeoutSeconds int `yaml:"write_timeout_seconds"`
	IdleTimeoutSeconds  int `yaml:"idle_timeout_seconds"`
}

type GatewayConfig struct {
	Enabled          bool                  `yaml:"enabled"`
	Token            string                `yaml:"token"`
	AllowedPlatforms []string              `yaml:"allowed_platforms"`
	Telegram         TelegramGatewayConfig `yaml:"telegram"`
}

type TelegramGatewayConfig struct {
	Enabled     bool   `yaml:"enabled"`
	BotTokenEnv string `yaml:"bot_token_env"`
	Secret      string `yaml:"secret"`
}

type ExecutionConfig struct {
	Enabled         bool                   `yaml:"enabled"`
	TimeoutSeconds  int                    `yaml:"timeout_seconds"`
	AllowedCommands []string               `yaml:"allowed_commands"`
	MaxArgs         int                    `yaml:"max_args"`
	MaxArgLength    int                    `yaml:"max_arg_length"`
	MaxOutputBytes  int                    `yaml:"max_output_bytes"`
	CommandRules    map[string]CommandRule `yaml:"command_rules"`
}

type CommandRule struct {
	MaxArgs            int      `yaml:"max_args"`
	MaxArgLength       int      `yaml:"max_arg_length"`
	MaxOutputBytes     int      `yaml:"max_output_bytes"`
	AllowedArgPrefixes []string `yaml:"allowed_arg_prefixes"`
	DeniedSubstrings   []string `yaml:"denied_substrings"`
}

type ExtensionConfig struct {
	PluginsDir string   `yaml:"plugins_dir"`
	SkillsDirs []string `yaml:"skills_dirs"`
}

type MCPServerConfig struct {
	Enabled        bool              `yaml:"enabled"`
	Command        string            `yaml:"command"`
	Args           []string          `yaml:"args"`
	Env            map[string]string `yaml:"env"`
	TimeoutSeconds int               `yaml:"timeout_seconds"`
	IncludeTools   []string          `yaml:"include_tools"`
	ExcludeTools   []string          `yaml:"exclude_tools"`
}

func Default() Config {
	return Config{
		AppName:          "hermes-go",
		ListenAddr:       "127.0.0.1:8080",
		DataDir:          "./data",
		JWTIssuer:        "hermes-go",
		JWTExpiryMinutes: 720,
		LLM: LLMConfig{
			Provider:       "openai-compatible",
			Model:          "gpt-4.1-mini",
			BaseURL:        "https://api.openai.com/v1",
			APIKeyEnv:      "OPENAI_API_KEY",
			TimeoutSeconds: 60,
			DisplayName:    "OpenAI GPT-4.1 Mini",
		},
		CurrentModelProfile: "openai-gpt41-mini",
		ModelProfiles: map[string]LLMConfig{
			"openai-gpt41-mini": {
				Provider:       "openai-compatible",
				Model:          "gpt-4.1-mini",
				BaseURL:        "https://api.openai.com/v1",
				APIKeyEnv:      "OPENAI_API_KEY",
				TimeoutSeconds: 60,
				DisplayName:    "OpenAI GPT-4.1 Mini",
			},
			"openrouter-claude-sonnet": {
				Provider:       "openai-compatible",
				Model:          "anthropic/claude-sonnet-4.6",
				BaseURL:        "https://openrouter.ai/api/v1",
				APIKeyEnv:      "OPENROUTER_API_KEY",
				TimeoutSeconds: 60,
				DisplayName:    "OpenRouter Claude Sonnet 4.6",
			},
			"ollama-qwen3": {
				Provider:       "openai-compatible",
				Model:          "qwen3:14b",
				BaseURL:        "http://127.0.0.1:11434/v1",
				APIKeyEnv:      "",
				TimeoutSeconds: 60,
				DisplayName:    "Ollama Qwen3 14B",
				Local:          true,
			},
			"lmstudio-local": {
				Provider:       "openai-compatible",
				Model:          "local-model",
				BaseURL:        "http://127.0.0.1:1234/v1",
				APIKeyEnv:      "",
				TimeoutSeconds: 60,
				DisplayName:    "LM Studio Local Model",
				Local:          true,
			},
		},
		Security: SecurityConfig{
			RequireAuth:       true,
			AllowRegistration: false,
			MinPasswordLength: 12,
			MaxLoginAttempts:  5,
			LoginWindowMinute: 15,
		},
		Memory: MemoryConfig{
			Enabled:         true,
			MemoryCharLimit: 2200,
			UserCharLimit:   1375,
			RecallLimit:     3,
		},
		Context: ContextConfig{
			HistoryWindowMessages:     8,
			MaxPromptChars:            24000,
			CompressionEnabled:        true,
			CompressThresholdMessages: 6,
			ProtectLastMessages:       4,
			SummaryMaxChars:           900,
			SummaryStrategy:           "rule",
		},
		Server: ServerConfig{
			ReadTimeoutSeconds:  15,
			WriteTimeoutSeconds: 60,
			IdleTimeoutSeconds:  60,
		},
		Gateway: GatewayConfig{
			Enabled:          true,
			Token:            "change-this-gateway-token",
			AllowedPlatforms: []string{"webhook"},
			Telegram: TelegramGatewayConfig{
				Enabled:     false,
				BotTokenEnv: "TELEGRAM_BOT_TOKEN",
				Secret:      "change-this-telegram-secret",
			},
		},
		Execution: ExecutionConfig{
			Enabled:         false,
			TimeoutSeconds:  10,
			AllowedCommands: []string{"echo", "date"},
			MaxArgs:         8,
			MaxArgLength:    256,
			MaxOutputBytes:  4096,
			CommandRules: map[string]CommandRule{
				"echo": {
					MaxArgs:        8,
					MaxArgLength:   256,
					MaxOutputBytes: 4096,
				},
				"date": {
					MaxArgs:            4,
					MaxArgLength:       64,
					MaxOutputBytes:     512,
					AllowedArgPrefixes: []string{"+", "--iso"},
				},
			},
		},
		Extensions: ExtensionConfig{
			PluginsDir: "./plugins",
			SkillsDirs: []string{"./skills"},
		},
		MCPServers: map[string]MCPServerConfig{},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	raw, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func (c Config) Validate() error {
	if c.ListenAddr == "" {
		return fmt.Errorf("listen_addr is required")
	}
	if c.DataDir == "" {
		return fmt.Errorf("data_dir is required")
	}
	if c.JWTIssuer == "" {
		return fmt.Errorf("jwt_issuer is required")
	}
	if c.JWTExpiryMinutes <= 0 {
		return fmt.Errorf("jwt_expiry_minutes must be > 0")
	}
	if c.LLM.Model == "" {
		return fmt.Errorf("llm.model is required")
	}
	if c.LLM.BaseURL == "" {
		return fmt.Errorf("llm.base_url is required")
	}
	if c.Security.MinPasswordLength < 10 {
		return fmt.Errorf("min_password_length must be >= 10")
	}
	if c.Security.MaxLoginAttempts <= 0 {
		return fmt.Errorf("max_login_attempts must be > 0")
	}
	if c.Security.LoginWindowMinute <= 0 {
		return fmt.Errorf("login_window_minutes must be > 0")
	}
	if c.Memory.MemoryCharLimit <= 0 {
		return fmt.Errorf("memory.memory_char_limit must be > 0")
	}
	if c.Memory.UserCharLimit <= 0 {
		return fmt.Errorf("memory.user_char_limit must be > 0")
	}
	if c.Memory.RecallLimit <= 0 {
		return fmt.Errorf("memory.recall_limit must be > 0")
	}
	if c.Context.HistoryWindowMessages < 0 {
		return fmt.Errorf("context.history_window_messages must be >= 0")
	}
	if c.Context.MaxPromptChars <= 0 {
		return fmt.Errorf("context.max_prompt_chars must be > 0")
	}
	if c.Context.CompressThresholdMessages < 0 {
		return fmt.Errorf("context.compress_threshold_messages must be >= 0")
	}
	if c.Context.ProtectLastMessages < 0 {
		return fmt.Errorf("context.protect_last_messages must be >= 0")
	}
	if c.Context.SummaryMaxChars <= 0 {
		return fmt.Errorf("context.summary_max_chars must be > 0")
	}
	if c.Context.SummaryStrategy == "" {
		c.Context.SummaryStrategy = "rule"
	}
	if c.Context.SummaryStrategy != "rule" && c.Context.SummaryStrategy != "llm" {
		return fmt.Errorf("context.summary_strategy must be one of: rule, llm")
	}
	if c.Execution.TimeoutSeconds <= 0 {
		return fmt.Errorf("execution.timeout_seconds must be > 0")
	}
	if c.Execution.MaxArgs <= 0 {
		return fmt.Errorf("execution.max_args must be > 0")
	}
	if c.Execution.MaxArgLength <= 0 {
		return fmt.Errorf("execution.max_arg_length must be > 0")
	}
	if c.Execution.MaxOutputBytes <= 0 {
		return fmt.Errorf("execution.max_output_bytes must be > 0")
	}
	for name, server := range c.MCPServers {
		if !server.Enabled {
			continue
		}
		if server.Command == "" {
			return fmt.Errorf("mcp_servers.%s.command is required", name)
		}
		if server.TimeoutSeconds <= 0 {
			return fmt.Errorf("mcp_servers.%s.timeout_seconds must be > 0", name)
		}
	}
	if c.CurrentModelProfile != "" {
		if _, ok := c.ModelProfiles[c.CurrentModelProfile]; !ok {
			return fmt.Errorf("current_model_profile %q is not defined", c.CurrentModelProfile)
		}
	}
	return nil
}

func (c Config) DBPath() string {
	return filepath.Join(c.DataDir, "hermes-go.db")
}

func (c Config) JWTSecretPath() string {
	return filepath.Join(c.DataDir, "jwt.secret")
}

func (c Config) Timeout() time.Duration {
	return time.Duration(c.LLM.TimeoutSeconds) * time.Second
}

func (c Config) JWTExpiry() time.Duration {
	return time.Duration(c.JWTExpiryMinutes) * time.Minute
}

func (c Config) ResolvedLLM() LLMConfig {
	if c.CurrentModelProfile == "" {
		return c.LLM
	}
	if profile, ok := c.ModelProfiles[c.CurrentModelProfile]; ok {
		return mergeLLMConfig(c.LLM, profile)
	}
	return c.LLM
}

func (c Config) ListModelProfiles() map[string]LLMConfig {
	result := make(map[string]LLMConfig, len(c.ModelProfiles))
	for name, profile := range c.ModelProfiles {
		result[name] = mergeLLMConfig(c.LLM, profile)
	}
	return result
}

func (c *Config) UseModelProfile(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("model profile is required")
	}
	profile, ok := c.ModelProfiles[name]
	if !ok {
		return fmt.Errorf("unknown model profile %q", name)
	}
	c.CurrentModelProfile = name
	c.LLM = mergeLLMConfig(c.LLM, profile)
	return nil
}

func (c *Config) UpsertModelProfile(name string, profile LLMConfig) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("model profile is required")
	}
	if c.ModelProfiles == nil {
		c.ModelProfiles = make(map[string]LLMConfig)
	}
	c.ModelProfiles[name] = profile
	return c.UseModelProfile(name)
}

func mergeLLMConfig(base, override LLMConfig) LLMConfig {
	result := base
	if override.Provider != "" {
		result.Provider = override.Provider
	}
	if override.Model != "" {
		result.Model = override.Model
	}
	if override.BaseURL != "" {
		result.BaseURL = override.BaseURL
	}
	result.APIKeyEnv = override.APIKeyEnv
	if override.TimeoutSeconds > 0 {
		result.TimeoutSeconds = override.TimeoutSeconds
	}
	if override.DisplayName != "" {
		result.DisplayName = override.DisplayName
	}
	result.Local = override.Local
	return result
}
