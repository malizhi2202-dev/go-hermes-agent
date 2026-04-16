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

// ToolSpec describes one command-backed extension tool.
type ToolSpec struct {
	Name           string            `yaml:"name" json:"name"`
	Description    string            `yaml:"description" json:"description"`
	Command        string            `yaml:"command" json:"command"`
	ArgsTemplate   []string          `yaml:"args_template" json:"args_template"`
	Env            map[string]string `yaml:"env" json:"env"`
	TimeoutSeconds int               `yaml:"timeout_seconds" json:"timeout_seconds"`
	InputKeys      []string          `yaml:"input_keys" json:"input_keys"`
}

// HookSpec describes one controlled lifecycle hook command.
type HookSpec struct {
	Name           string            `yaml:"name" json:"name"`
	Command        string            `yaml:"command" json:"command"`
	ArgsTemplate   []string          `yaml:"args_template" json:"args_template"`
	Env            map[string]string `yaml:"env" json:"env"`
	TimeoutSeconds int               `yaml:"timeout_seconds" json:"timeout_seconds"`
}

// LifecycleSpec groups validate/enable/disable hooks for one extension.
type LifecycleSpec struct {
	Validate  []HookSpec `yaml:"validate" json:"validate"`
	OnEnable  []HookSpec `yaml:"on_enable" json:"on_enable"`
	OnDisable []HookSpec `yaml:"on_disable" json:"on_disable"`
}

// PluginManifest is the normalized plugin declaration.
type PluginManifest struct {
	Name        string        `yaml:"name" json:"name"`
	Version     string        `yaml:"version" json:"version"`
	Description string        `yaml:"description" json:"description"`
	Enabled     bool          `yaml:"enabled" json:"enabled"`
	Provides    []string      `yaml:"provides" json:"provides"`
	Tool        ToolSpec      `yaml:"tool" json:"tool"`
	Lifecycle   LifecycleSpec `yaml:"lifecycle" json:"lifecycle"`
	Path        string        `json:"path"`
	Hash        string        `json:"hash"`
	StateSource string        `json:"state_source"`
}

// SkillDefinition is the normalized discovered skill record.
type SkillDefinition struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Enabled     bool          `json:"enabled"`
	Path        string        `json:"path"`
	Platforms   []string      `json:"platforms"`
	Tool        ToolSpec      `json:"tool"`
	Lifecycle   LifecycleSpec `json:"lifecycle"`
	Hash        string        `json:"hash"`
	StateSource string        `json:"state_source"`
}

// SkillManifest is the YAML manifest shape for a skill.
type SkillManifest struct {
	Name        string        `yaml:"name"`
	Description string        `yaml:"description"`
	Enabled     bool          `yaml:"enabled"`
	Platforms   []string      `yaml:"platforms"`
	Tool        ToolSpec      `yaml:"tool"`
	Lifecycle   LifecycleSpec `yaml:"lifecycle"`
}

// MCPTool is one discovered MCP tool.
type MCPTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// MCPServerStatus reports discovery state for one MCP server.
type MCPServerStatus struct {
	Name        string    `json:"name"`
	Transport   string    `json:"transport"`
	URL         string    `json:"url,omitempty"`
	Enabled     bool      `json:"enabled"`
	Command     string    `json:"command"`
	Args        []string  `json:"args"`
	Discovered  bool      `json:"discovered"`
	Tools       []MCPTool `json:"tools"`
	Error       string    `json:"error,omitempty"`
	RefreshedAt time.Time `json:"refreshed_at"`
}

// Summary is the top-level extension discovery summary.
type Summary struct {
	Plugins []PluginManifest  `json:"plugins"`
	Skills  []SkillDefinition `json:"skills"`
	MCP     []MCPServerStatus `json:"mcp"`
}

// Manager discovers, persists, and registers extensions.
type Manager struct {
	cfg         config.Config
	audit       func(ctx context.Context, username, action, detail string) error
	plugins     []PluginManifest
	skills      []SkillDefinition
	mcpServers  []MCPServerStatus
	stateReader func(ctx context.Context) ([]ExtensionStateRecord, error)
	stateWriter func(ctx context.Context, kind, name string, enabled bool, hash string) error
	hookWriter  func(ctx context.Context, record ExtensionHookRecord) error
	registered  []string
}

// ExtensionStateRecord is the persisted enable/disable state model.
type ExtensionStateRecord struct {
	Kind    string
	Name    string
	Enabled bool
	Hash    string
}

// ExtensionHookRecord is the manager-level lifecycle hook result model.
type ExtensionHookRecord struct {
	Username string
	Kind     string
	Name     string
	Phase    string
	Hook     string
	Status   string
	Output   string
	Error    string
}

// NewManager creates an extension manager with optional state persistence hooks.
func NewManager(
	cfg config.Config,
	audit func(ctx context.Context, username, action, detail string) error,
	stateReader func(ctx context.Context) ([]ExtensionStateRecord, error),
	stateWriter func(ctx context.Context, kind, name string, enabled bool, hash string) error,
	hookWriter func(ctx context.Context, record ExtensionHookRecord) error,
) *Manager {
	return &Manager{cfg: cfg, audit: audit, stateReader: stateReader, stateWriter: stateWriter, hookWriter: hookWriter}
}

// Discover scans plugins, skills, and MCP servers and refreshes in-memory state.
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

// Register refreshes all enabled extension tools in the registry.
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

// SetEnabled persists extension state and refreshes discovery.
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
	if enabled {
		if _, err := m.Validate(ctx, username, kind, name); err != nil {
			return err
		}
		if err := m.runLifecycleHooks(ctx, username, kind, name, "on_enable"); err != nil {
			return err
		}
	} else {
		if err := m.runLifecycleHooks(ctx, username, kind, name, "on_disable"); err != nil {
			return err
		}
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

// Validate executes the controlled validation hooks for one extension.
func (m *Manager) Validate(ctx context.Context, username, kind, name string) (map[string]any, error) {
	hooks, input, err := m.lifecycleHooks(kind, name, "validate")
	if err != nil {
		return nil, err
	}
	results := make([]map[string]any, 0, len(hooks))
	for _, hook := range hooks {
		if m.audit != nil {
			_ = m.audit(ctx, username, "extension_validate_attempt", fmt.Sprintf("kind=%s name=%s hook=%s", kind, name, firstNonEmpty(hook.Name, hook.Command)))
		}
		output, err := executeCommandSpec(ctx, toolSpecFromHook(hook), input)
		result := map[string]any{
			"hook":   firstNonEmpty(hook.Name, hook.Command),
			"output": output,
		}
		if err != nil {
			result["error"] = err.Error()
			m.writeHookResult(ctx, ExtensionHookRecord{
				Username: username,
				Kind:     kind,
				Name:     name,
				Phase:    "validate",
				Hook:     firstNonEmpty(hook.Name, hook.Command),
				Status:   "failed",
				Output:   output,
				Error:    err.Error(),
			})
			if m.audit != nil {
				_ = m.audit(ctx, username, "extension_validate_denied", fmt.Sprintf("kind=%s name=%s err=%s", kind, name, err.Error()))
			}
			return map[string]any{"kind": kind, "name": name, "results": append(results, result)}, err
		}
		m.writeHookResult(ctx, ExtensionHookRecord{
			Username: username,
			Kind:     kind,
			Name:     name,
			Phase:    "validate",
			Hook:     firstNonEmpty(hook.Name, hook.Command),
			Status:   "success",
			Output:   output,
		})
		results = append(results, result)
	}
	if m.audit != nil {
		_ = m.audit(ctx, username, "extension_validate_success", fmt.Sprintf("kind=%s name=%s hooks=%d", kind, name, len(results)))
	}
	return map[string]any{"kind": kind, "name": name, "results": results}, nil
}

// Summary returns the current discovered extension state.
func (m *Manager) Summary() Summary {
	return Summary{
		Plugins: append([]PluginManifest(nil), m.plugins...),
		Skills:  append([]SkillDefinition(nil), m.skills...),
		MCP:     append([]MCPServerStatus(nil), m.mcpServers...),
	}
}

func (m *Manager) runLifecycleHooks(ctx context.Context, username, kind, name, phase string) error {
	hooks, input, err := m.lifecycleHooks(kind, name, phase)
	if err != nil {
		return err
	}
	for _, hook := range hooks {
		hookName := firstNonEmpty(hook.Name, hook.Command)
		if m.audit != nil {
			_ = m.audit(ctx, username, "extension_hook_attempt", fmt.Sprintf("kind=%s name=%s phase=%s hook=%s", kind, name, phase, hookName))
		}
		output, err := executeCommandSpec(ctx, toolSpecFromHook(hook), input)
		if err != nil {
			m.writeHookResult(ctx, ExtensionHookRecord{
				Username: username,
				Kind:     kind,
				Name:     name,
				Phase:    phase,
				Hook:     hookName,
				Status:   "failed",
				Output:   output,
				Error:    err.Error(),
			})
			if m.audit != nil {
				_ = m.audit(ctx, username, "extension_hook_denied", fmt.Sprintf("kind=%s name=%s phase=%s err=%s", kind, name, phase, err.Error()))
			}
			return err
		}
		m.writeHookResult(ctx, ExtensionHookRecord{
			Username: username,
			Kind:     kind,
			Name:     name,
			Phase:    phase,
			Hook:     hookName,
			Status:   "success",
			Output:   output,
		})
		if m.audit != nil {
			_ = m.audit(ctx, username, "extension_hook_success", fmt.Sprintf("kind=%s name=%s phase=%s hook=%s bytes=%d", kind, name, phase, hookName, len(output)))
		}
	}
	return nil
}

func (m *Manager) writeHookResult(ctx context.Context, record ExtensionHookRecord) {
	if m.hookWriter == nil {
		return
	}
	_ = m.hookWriter(ctx, record)
}

func (m *Manager) lifecycleHooks(kind, name, phase string) ([]HookSpec, map[string]any, error) {
	switch kind {
	case "plugin":
		for _, plugin := range m.plugins {
			if plugin.Name != name {
				continue
			}
			return hooksForPhase(plugin.Lifecycle, phase), extensionHookInput(kind, plugin.Name, plugin.Path, plugin.Hash, plugin.StateSource), nil
		}
	case "skill":
		for _, skill := range m.skills {
			if skill.Name != name {
				continue
			}
			return hooksForPhase(skill.Lifecycle, phase), extensionHookInput(kind, skill.Name, skill.Path, skill.Hash, skill.StateSource), nil
		}
	default:
		return nil, nil, fmt.Errorf("unsupported extension kind %q", kind)
	}
	return nil, nil, fmt.Errorf("extension %s/%s not found", kind, name)
}

func hooksForPhase(lifecycle LifecycleSpec, phase string) []HookSpec {
	switch phase {
	case "validate":
		return lifecycle.Validate
	case "on_enable":
		return lifecycle.OnEnable
	case "on_disable":
		return lifecycle.OnDisable
	default:
		return nil
	}
}

func extensionHookInput(kind, name, path, hash, stateSource string) map[string]any {
	return map[string]any{
		"extension_kind": kind,
		"extension_name": name,
		"extension_path": path,
		"extension_hash": hash,
		"state_source":   stateSource,
	}
}

func toolSpecFromHook(hook HookSpec) ToolSpec {
	return ToolSpec{
		Command:        hook.Command,
		ArgsTemplate:   hook.ArgsTemplate,
		Env:            hook.Env,
		TimeoutSeconds: hook.TimeoutSeconds,
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
			Transport:   normalizedMCPTransport(cfg),
			URL:         cfg.URL,
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
