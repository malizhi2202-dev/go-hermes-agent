package extensions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"go-hermes-agent/internal/config"
	"go-hermes-agent/internal/tools"
)

type ToolSpec struct {
	Name           string            `yaml:"name" json:"name"`
	Description    string            `yaml:"description" json:"description"`
	Command        string            `yaml:"command" json:"command"`
	ArgsTemplate   []string          `yaml:"args_template" json:"args_template"`
	Env            map[string]string `yaml:"env" json:"env"`
	TimeoutSeconds int               `yaml:"timeout_seconds" json:"timeout_seconds"`
	InputKeys      []string          `yaml:"input_keys" json:"input_keys"`
}

type PluginManifest struct {
	Name        string   `yaml:"name" json:"name"`
	Version     string   `yaml:"version" json:"version"`
	Description string   `yaml:"description" json:"description"`
	Enabled     bool     `yaml:"enabled" json:"enabled"`
	Provides    []string `yaml:"provides" json:"provides"`
	Tool        ToolSpec `yaml:"tool" json:"tool"`
	Path        string   `json:"path"`
	Hash        string   `json:"hash"`
	StateSource string   `json:"state_source"`
}

type SkillDefinition struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Enabled     bool     `json:"enabled"`
	Path        string   `json:"path"`
	Platforms   []string `json:"platforms"`
	Tool        ToolSpec `json:"tool"`
	Hash        string   `json:"hash"`
	StateSource string   `json:"state_source"`
}

type SkillManifest struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Enabled     bool     `yaml:"enabled"`
	Platforms   []string `yaml:"platforms"`
	Tool        ToolSpec `yaml:"tool"`
}

type MCPTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type MCPServerStatus struct {
	Name        string    `json:"name"`
	Enabled     bool      `json:"enabled"`
	Command     string    `json:"command"`
	Args        []string  `json:"args"`
	Discovered  bool      `json:"discovered"`
	Tools       []MCPTool `json:"tools"`
	Error       string    `json:"error,omitempty"`
	RefreshedAt time.Time `json:"refreshed_at"`
}

type Summary struct {
	Plugins []PluginManifest  `json:"plugins"`
	Skills  []SkillDefinition `json:"skills"`
	MCP     []MCPServerStatus `json:"mcp"`
}

type Manager struct {
	cfg         config.Config
	audit       func(ctx context.Context, username, action, detail string) error
	plugins     []PluginManifest
	skills      []SkillDefinition
	mcpServers  []MCPServerStatus
	stateReader func(ctx context.Context) ([]ExtensionStateRecord, error)
	stateWriter func(ctx context.Context, kind, name string, enabled bool, hash string) error
	registered  []string
}

type ExtensionStateRecord struct {
	Kind    string
	Name    string
	Enabled bool
	Hash    string
}

func NewManager(
	cfg config.Config,
	audit func(ctx context.Context, username, action, detail string) error,
	stateReader func(ctx context.Context) ([]ExtensionStateRecord, error),
	stateWriter func(ctx context.Context, kind, name string, enabled bool, hash string) error,
) *Manager {
	return &Manager{cfg: cfg, audit: audit, stateReader: stateReader, stateWriter: stateWriter}
}

func (m *Manager) Discover(ctx context.Context) error {
	plugins, err := discoverPlugins(m.cfg.Extensions.PluginsDir)
	if err != nil {
		return err
	}
	skills, err := discoverSkills(m.cfg.Extensions.SkillsDirs)
	if err != nil {
		return err
	}
	mcpServers := discoverMCPServers(ctx, m.cfg.MCPServers)
	if err := m.applyStoredStates(ctx, &plugins, &skills); err != nil {
		return err
	}
	m.plugins = plugins
	m.skills = skills
	m.mcpServers = mcpServers
	return nil
}

func (m *Manager) Register(registry *tools.Registry) error {
	registry.Unregister(m.registered...)
	m.registered = nil
	for _, plugin := range m.plugins {
		if !plugin.Enabled || plugin.Tool.Command == "" {
			continue
		}
		if err := registerCommandTool(registry, "plugin", plugin.Name, plugin.Tool, m.audit); err != nil {
			return err
		}
		m.registered = append(m.registered, effectiveToolName("plugin", plugin.Name, plugin.Tool))
	}
	for _, skill := range m.skills {
		if !skill.Enabled || skill.Tool.Command == "" {
			continue
		}
		if err := registerCommandTool(registry, "skill", skill.Name, skill.Tool, m.audit); err != nil {
			return err
		}
		m.registered = append(m.registered, effectiveToolName("skill", skill.Name, skill.Tool))
	}
	for _, server := range m.mcpServers {
		if !server.Enabled {
			continue
		}
		for _, toolDef := range server.Tools {
			if err := registerMCPTool(registry, server.Name, m.cfg.MCPServers[server.Name], toolDef, m.audit); err != nil {
				return err
			}
			m.registered = append(m.registered, fmt.Sprintf("mcp.%s.%s", sanitizeName(server.Name), sanitizeName(toolDef.Name)))
		}
	}
	return nil
}

func (m *Manager) SetEnabled(ctx context.Context, username, kind, name string, enabled bool) error {
	kind = strings.TrimSpace(kind)
	name = strings.TrimSpace(name)
	if kind == "" || name == "" {
		return fmt.Errorf("kind and name are required")
	}
	hash := m.currentHash(kind, name)
	if m.stateWriter == nil {
		return fmt.Errorf("extension state storage is not configured")
	}
	if err := m.stateWriter(ctx, kind, name, enabled, hash); err != nil {
		return err
	}
	if m.audit != nil {
		detail := fmt.Sprintf("kind=%s name=%s enabled=%t hash=%s", kind, name, enabled, hash)
		_ = m.audit(ctx, username, "extension_state_changed", detail)
	}
	return m.Discover(ctx)
}

func (m *Manager) Summary() Summary {
	return Summary{
		Plugins: append([]PluginManifest(nil), m.plugins...),
		Skills:  append([]SkillDefinition(nil), m.skills...),
		MCP:     append([]MCPServerStatus(nil), m.mcpServers...),
	}
}

func discoverPlugins(root string) ([]PluginManifest, error) {
	if strings.TrimSpace(root) == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read plugins dir: %w", err)
	}
	plugins := make([]PluginManifest, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(root, entry.Name(), "plugin.yaml")
		raw, err := os.ReadFile(manifestPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read plugin manifest %s: %w", manifestPath, err)
		}
		var manifest PluginManifest
		if err := yaml.Unmarshal(raw, &manifest); err != nil {
			return nil, fmt.Errorf("parse plugin manifest %s: %w", manifestPath, err)
		}
		enabled, hasEnabled, err := readEnabledFlag(raw)
		if err != nil {
			return nil, fmt.Errorf("parse plugin enabled state %s: %w", manifestPath, err)
		}
		if manifest.Name == "" {
			manifest.Name = entry.Name()
		}
		if !hasEnabled {
			manifest.Enabled = true
		} else {
			manifest.Enabled = enabled
		}
		manifest.Path = filepath.Join(root, entry.Name())
		manifest.Hash = hashFiles(manifestPath)
		manifest.StateSource = "manifest"
		plugins = append(plugins, manifest)
	}
	sort.Slice(plugins, func(i, j int) bool { return plugins[i].Name < plugins[j].Name })
	return plugins, nil
}

func discoverSkills(roots []string) ([]SkillDefinition, error) {
	result := make([]SkillDefinition, 0)
	seen := make(map[string]struct{})
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read skills dir: %w", err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			skillDir := filepath.Join(root, entry.Name())
			skillMD := filepath.Join(skillDir, "SKILL.md")
			raw, err := os.ReadFile(skillMD)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, fmt.Errorf("read skill file %s: %w", skillMD, err)
			}
			meta, body := parseFrontmatter(string(raw))
			def := SkillDefinition{
				Name:        stringValue(meta["name"], entry.Name()),
				Description: strings.TrimSpace(firstNonEmpty(stringValue(meta["description"], ""), firstLine(body))),
				Enabled:     true,
				Path:        skillDir,
				Platforms:   stringSlice(meta["platforms"]),
			}
			manifestPath := filepath.Join(skillDir, "skill.yaml")
			if manifestRaw, err := os.ReadFile(manifestPath); err == nil {
				var manifest SkillManifest
				if err := yaml.Unmarshal(manifestRaw, &manifest); err != nil {
					return nil, fmt.Errorf("parse skill manifest %s: %w", manifestPath, err)
				}
				enabled, hasEnabled, err := readEnabledFlag(manifestRaw)
				if err != nil {
					return nil, fmt.Errorf("parse skill enabled state %s: %w", manifestPath, err)
				}
				if manifest.Name != "" {
					def.Name = manifest.Name
				}
				if manifest.Description != "" {
					def.Description = manifest.Description
				}
				if len(manifest.Platforms) > 0 {
					def.Platforms = manifest.Platforms
				}
				if manifest.Tool.Name != "" || manifest.Tool.Command != "" {
					def.Tool = manifest.Tool
				}
				if hasEnabled {
					def.Enabled = enabled
				}
				def.Hash = hashFiles(skillMD, manifestPath)
				def.StateSource = "manifest"
			} else {
				def.Hash = hashFiles(skillMD)
				def.StateSource = "frontmatter"
			}
			if _, ok := seen[def.Name]; ok {
				continue
			}
			seen[def.Name] = struct{}{}
			result = append(result, def)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func readEnabledFlag(raw []byte) (bool, bool, error) {
	var parsed map[string]any
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		return false, false, err
	}
	value, ok := parsed["enabled"]
	if !ok {
		return false, false, nil
	}
	enabled, ok := value.(bool)
	if !ok {
		return false, false, fmt.Errorf("enabled must be a boolean")
	}
	return enabled, true, nil
}

func (m *Manager) applyStoredStates(ctx context.Context, plugins *[]PluginManifest, skills *[]SkillDefinition) error {
	if m.stateReader == nil {
		return nil
	}
	states, err := m.stateReader(ctx)
	if err != nil {
		return err
	}
	index := make(map[string]ExtensionStateRecord, len(states))
	for _, state := range states {
		index[state.Kind+":"+state.Name] = state
	}
	for i := range *plugins {
		key := "plugin:" + (*plugins)[i].Name
		if state, ok := index[key]; ok {
			(*plugins)[i].Enabled = state.Enabled
			(*plugins)[i].StateSource = "database"
			if state.Hash != "" && state.Hash != (*plugins)[i].Hash {
				(*plugins)[i].StateSource = "database-mismatch"
			}
		}
	}
	for i := range *skills {
		key := "skill:" + (*skills)[i].Name
		if state, ok := index[key]; ok {
			(*skills)[i].Enabled = state.Enabled
			(*skills)[i].StateSource = "database"
			if state.Hash != "" && state.Hash != (*skills)[i].Hash {
				(*skills)[i].StateSource = "database-mismatch"
			}
		}
	}
	return nil
}

func (m *Manager) currentHash(kind, name string) string {
	switch kind {
	case "plugin":
		for _, plugin := range m.plugins {
			if plugin.Name == name {
				return plugin.Hash
			}
		}
	case "skill":
		for _, skill := range m.skills {
			if skill.Name == name {
				return skill.Hash
			}
		}
	}
	return ""
}

func hashFiles(paths ...string) string {
	digest := sha256.New()
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		_, _ = digest.Write([]byte(path))
		_, _ = digest.Write(raw)
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func discoverMCPServers(ctx context.Context, servers map[string]config.MCPServerConfig) []MCPServerStatus {
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]MCPServerStatus, 0, len(names))
	for _, name := range names {
		cfg := servers[name]
		status := MCPServerStatus{
			Name:        name,
			Enabled:     cfg.Enabled,
			Command:     cfg.Command,
			Args:        append([]string(nil), cfg.Args...),
			RefreshedAt: time.Now().UTC(),
		}
		if cfg.Enabled {
			tools, err := listMCPTools(ctx, name, cfg)
			if err != nil {
				status.Error = err.Error()
			} else {
				status.Discovered = true
				status.Tools = tools
			}
		}
		result = append(result, status)
	}
	return result
}

func parseFrontmatter(content string) (map[string]any, string) {
	if !strings.HasPrefix(content, "---\n") {
		return map[string]any{}, content
	}
	rest := strings.TrimPrefix(content, "---\n")
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		return map[string]any{}, content
	}
	frontmatter := rest[:idx]
	body := rest[idx+5:]
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(frontmatter), &parsed); err != nil {
		return map[string]any{}, content
	}
	return parsed, body
}

func firstLine(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func stringValue(value any, fallback string) string {
	if str, ok := value.(string); ok && strings.TrimSpace(str) != "" {
		return strings.TrimSpace(str)
	}
	return fallback
}

func stringSlice(value any) []string {
	switch typed := value.(type) {
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if str, ok := item.(string); ok && strings.TrimSpace(str) != "" {
				result = append(result, strings.TrimSpace(str))
			}
		}
		return result
	case []string:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if strings.TrimSpace(item) != "" {
				result = append(result, strings.TrimSpace(item))
			}
		}
		return result
	default:
		return nil
	}
}
