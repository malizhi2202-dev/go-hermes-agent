package extensions

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"hermes-agent/go/internal/config"
	"hermes-agent/go/internal/tools"
)

func TestManagerDiscoversAndRegistersPluginAndSkillTools(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses unix executable permissions")
	}
	root := t.TempDir()
	pluginsDir := filepath.Join(root, "plugins")
	skillsDir := filepath.Join(root, "skills")
	if err := os.MkdirAll(filepath.Join(pluginsDir, "echoer"), 0o755); err != nil {
		t.Fatalf("mkdir plugin: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(skillsDir, "greeter"), 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	pluginScript := filepath.Join(root, "plugin-tool.sh")
	skillScript := filepath.Join(root, "skill-tool.sh")
	if err := os.WriteFile(pluginScript, []byte("#!/bin/sh\nprintf 'plugin:%s' \"$1\"\n"), 0o755); err != nil {
		t.Fatalf("write plugin script: %v", err)
	}
	if err := os.WriteFile(skillScript, []byte("#!/bin/sh\nprintf 'skill:%s' \"$1\"\n"), 0o755); err != nil {
		t.Fatalf("write skill script: %v", err)
	}
	pluginManifest := `
name: echoer
description: Echo plugin
enabled: true
tool:
  name: plugin.echoer
  description: Echo through plugin
  command: ` + pluginScript + `
  args_template:
    - "{{message}}"
  input_keys:
    - message
`
	if err := os.WriteFile(filepath.Join(pluginsDir, "echoer", "plugin.yaml"), []byte(pluginManifest), 0o644); err != nil {
		t.Fatalf("write plugin manifest: %v", err)
	}
	skillMarkdown := `---
name: greeter
description: Greeting skill
---

Say hello.
`
	if err := os.WriteFile(filepath.Join(skillsDir, "greeter", "SKILL.md"), []byte(skillMarkdown), 0o644); err != nil {
		t.Fatalf("write skill markdown: %v", err)
	}
	skillManifest := `
name: greeter
enabled: true
tool:
  name: skill.greeter
  description: Echo through skill
  command: ` + skillScript + `
  args_template:
    - "{{name}}"
  input_keys:
    - name
`
	if err := os.WriteFile(filepath.Join(skillsDir, "greeter", "skill.yaml"), []byte(skillManifest), 0o644); err != nil {
		t.Fatalf("write skill manifest: %v", err)
	}

	cfg := config.Default()
	cfg.Extensions.PluginsDir = pluginsDir
	cfg.Extensions.SkillsDirs = []string{skillsDir}
	manager := NewManager(cfg, nil, nil, nil, nil)
	if err := manager.Discover(context.Background()); err != nil {
		t.Fatalf("discover extensions: %v", err)
	}
	summary := manager.Summary()
	if len(summary.Plugins) != 1 || len(summary.Skills) != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}

	registry := tools.New()
	if err := manager.Register(registry); err != nil {
		t.Fatalf("register extensions: %v", err)
	}
	pluginResult, err := registry.Execute(context.Background(), "plugin.echoer", map[string]any{
		"username": "admin",
		"message":  "alpha",
	})
	if err != nil {
		t.Fatalf("execute plugin tool: %v", err)
	}
	if pluginResult["output"] != "plugin:alpha" {
		t.Fatalf("unexpected plugin output: %#v", pluginResult)
	}
	skillResult, err := registry.Execute(context.Background(), "skill.greeter", map[string]any{
		"username": "admin",
		"name":     "beta",
	})
	if err != nil {
		t.Fatalf("execute skill tool: %v", err)
	}
	if skillResult["output"] != "skill:beta" {
		t.Fatalf("unexpected skill output: %#v", skillResult)
	}
}

func TestCommandToolRejectsDangerousArgs(t *testing.T) {
	registry := tools.New()
	err := registerCommandTool(registry, "plugin", "echoer", ToolSpec{
		Name:         "plugin.echoer",
		Command:      "/bin/echo",
		ArgsTemplate: []string{"{{message}}"},
		InputKeys:    []string{"message"},
	}, nil)
	if err != nil {
		t.Fatalf("register command tool: %v", err)
	}
	_, err = registry.Execute(context.Background(), "plugin.echoer", map[string]any{
		"username": "admin",
		"message":  "ok; rm -rf /",
	})
	if err == nil || !strings.Contains(err.Error(), "forbidden shell metacharacters") {
		t.Fatalf("expected dangerous arg rejection, got %v", err)
	}
}

func TestMCPServerDiscoveryAndExecution(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses python3 helper")
	}
	root := t.TempDir()
	script := filepath.Join(root, "mcp_server.py")
	program := `import sys, json

def read_message():
    headers = {}
    while True:
        line = sys.stdin.buffer.readline()
        if not line:
            return None
        if line in (b"\r\n", b"\n"):
            break
        key, value = line.decode().split(":", 1)
        headers[key.strip().lower()] = value.strip()
    length = int(headers.get("content-length", "0"))
    if length <= 0:
        return None
    body = sys.stdin.buffer.read(length)
    return json.loads(body)

def write_message(payload):
    body = json.dumps(payload).encode()
    sys.stdout.buffer.write(f"Content-Length: {len(body)}\r\n\r\n".encode() + body)
    sys.stdout.buffer.flush()

while True:
    message = read_message()
    if message is None:
        break
    method = message.get("method")
    if method == "initialize":
        write_message({"jsonrpc":"2.0","id":message["id"],"result":{"capabilities":{"tools":{}}}})
    elif method == "notifications/initialized":
        continue
    elif method == "tools/list":
        write_message({"jsonrpc":"2.0","id":message["id"],"result":{"tools":[{"name":"echo","description":"Echo tool","inputSchema":{"type":"object","properties":{"text":{"type":"string"}}}}]}})
    elif method == "tools/call":
        params = message.get("params", {})
        args = params.get("arguments", {})
        write_message({"jsonrpc":"2.0","id":message["id"],"result":{"content":[{"type":"text","text":"mcp:"+args.get("text","")} ]}})
`
	if err := os.WriteFile(script, []byte(program), 0o644); err != nil {
		t.Fatalf("write mcp helper: %v", err)
	}
	cfg := config.Default()
	cfg.MCPServers = map[string]config.MCPServerConfig{
		"helper": {
			Enabled:        true,
			Command:        "python3",
			Args:           []string{script},
			TimeoutSeconds: 5,
		},
	}
	manager := NewManager(cfg, nil, nil, nil, nil)
	if err := manager.Discover(context.Background()); err != nil {
		t.Fatalf("discover extensions: %v", err)
	}
	summary := manager.Summary()
	if len(summary.MCP) != 1 || !summary.MCP[0].Discovered || len(summary.MCP[0].Tools) != 1 {
		t.Fatalf("unexpected mcp summary: %+v", summary.MCP)
	}
	registry := tools.New()
	if err := manager.Register(registry); err != nil {
		t.Fatalf("register mcp tools: %v", err)
	}
	result, err := registry.Execute(context.Background(), "mcp.helper.echo", map[string]any{
		"username": "admin",
		"text":     "gamma",
	})
	if err != nil {
		t.Fatalf("execute mcp tool: %v", err)
	}
	raw, ok := result["result"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected mcp result: %#v", result)
	}
	content, ok := raw["content"].([]any)
	if !ok || len(content) != 1 {
		t.Fatalf("unexpected mcp content: %#v", raw)
	}
	first, ok := content[0].(map[string]any)
	if !ok || first["text"] != "mcp:gamma" {
		t.Fatalf("unexpected mcp text: %#v", content[0])
	}
}

func TestMCPHTTPServerDiscoveryAndExecution(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		method, _ := req["method"].(string)
		response := map[string]any{
			"jsonrpc": "2.0",
			"id":      req["id"],
			"result":  map[string]any{},
		}
		switch method {
		case "initialize":
			response["result"] = map[string]any{"capabilities": map[string]any{"tools": map[string]any{}}}
		case "notifications/initialized":
			response["result"] = map[string]any{}
		case "tools/list":
			response["result"] = map[string]any{
				"tools": []map[string]any{
					{
						"name":        "echo",
						"description": "Echo tool",
						"inputSchema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"text": map[string]any{"type": "string"},
							},
						},
					},
				},
			}
		case "tools/call":
			params, _ := req["params"].(map[string]any)
			args, _ := params["arguments"].(map[string]any)
			response["result"] = map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "mcp:http:" + stringValue(args["text"], "")},
				},
			}
		default:
			response["error"] = map[string]any{"code": -32601, "message": "method not found"}
			delete(response, "result")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	cfg := config.Default()
	cfg.MCPServers = map[string]config.MCPServerConfig{
		"helper_http": {
			Enabled:        true,
			Transport:      "http",
			URL:            server.URL,
			TimeoutSeconds: 5,
		},
	}
	manager := NewManager(cfg, nil, nil, nil, nil)
	if err := manager.Discover(context.Background()); err != nil {
		t.Fatalf("discover extensions: %v", err)
	}
	summary := manager.Summary()
	if len(summary.MCP) != 1 || !summary.MCP[0].Discovered || len(summary.MCP[0].Tools) != 1 {
		t.Fatalf("unexpected mcp summary: %+v", summary.MCP)
	}
	if summary.MCP[0].Transport != "http" || summary.MCP[0].URL != server.URL {
		t.Fatalf("unexpected mcp transport summary: %+v", summary.MCP[0])
	}
	registry := tools.New()
	if err := manager.Register(registry); err != nil {
		t.Fatalf("register mcp tools: %v", err)
	}
	result, err := registry.Execute(context.Background(), "mcp.helper_http.echo", map[string]any{
		"username": "admin",
		"text":     "delta",
	})
	if err != nil {
		t.Fatalf("execute mcp tool: %v", err)
	}
	raw, ok := result["result"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected mcp result: %#v", result)
	}
	content, ok := raw["content"].([]any)
	if !ok || len(content) != 1 {
		t.Fatalf("unexpected mcp content: %#v", raw)
	}
	first, ok := content[0].(map[string]any)
	if !ok || first["text"] != "mcp:http:delta" {
		t.Fatalf("unexpected mcp text: %#v", content[0])
	}
}

func TestManagerAppliesStoredStateAndUnregistersDisabledTools(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses unix executable permissions")
	}
	root := t.TempDir()
	pluginsDir := filepath.Join(root, "plugins")
	if err := os.MkdirAll(filepath.Join(pluginsDir, "echoer"), 0o755); err != nil {
		t.Fatalf("mkdir plugin: %v", err)
	}
	script := filepath.Join(root, "plugin-tool.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'plugin:%s' \"$1\"\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	manifest := `
name: echoer
tool:
  name: plugin.echoer
  command: ` + script + `
  args_template:
    - "{{message}}"
  input_keys:
    - message
`
	manifestPath := filepath.Join(pluginsDir, "echoer", "plugin.yaml")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	var persisted []ExtensionStateRecord
	manager := NewManager(config.Config{
		Extensions: config.ExtensionConfig{PluginsDir: pluginsDir},
	}, nil, func(context.Context) ([]ExtensionStateRecord, error) {
		return persisted, nil
	}, func(_ context.Context, kind, name string, enabled bool, hash string) error {
		persisted = []ExtensionStateRecord{{Kind: kind, Name: name, Enabled: enabled, Hash: hash}}
		return nil
	}, nil)
	if err := manager.Discover(context.Background()); err != nil {
		t.Fatalf("discover: %v", err)
	}
	registry := tools.New()
	if err := manager.Register(registry); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := registry.Execute(context.Background(), "plugin.echoer", map[string]any{"username": "admin", "message": "alpha"}); err != nil {
		t.Fatalf("expected enabled tool: %v", err)
	}
	if err := manager.SetEnabled(context.Background(), "admin", "plugin", "echoer", false); err != nil {
		t.Fatalf("disable extension: %v", err)
	}
	if err := manager.Register(registry); err != nil {
		t.Fatalf("register after disable: %v", err)
	}
	if _, err := registry.Execute(context.Background(), "plugin.echoer", map[string]any{"username": "admin", "message": "alpha"}); err == nil {
		t.Fatal("expected disabled tool to be unregistered")
	}
}

func TestExtensionLifecycleValidateAndStateHooks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses unix executable permissions")
	}
	root := t.TempDir()
	pluginsDir := filepath.Join(root, "plugins")
	if err := os.MkdirAll(filepath.Join(pluginsDir, "echoer"), 0o755); err != nil {
		t.Fatalf("mkdir plugin: %v", err)
	}
	toolScript := filepath.Join(root, "plugin-tool.sh")
	validateScript := filepath.Join(root, "validate.sh")
	enableScript := filepath.Join(root, "enable.sh")
	disableScript := filepath.Join(root, "disable.sh")
	if err := os.WriteFile(toolScript, []byte("#!/bin/sh\nprintf 'plugin:%s' \"$1\"\n"), 0o755); err != nil {
		t.Fatalf("write tool script: %v", err)
	}
	if err := os.WriteFile(validateScript, []byte("#!/bin/sh\nprintf '%s' \"$1\" > \"$2\"\n"), 0o755); err != nil {
		t.Fatalf("write validate script: %v", err)
	}
	if err := os.WriteFile(enableScript, []byte("#!/bin/sh\nprintf 'enabled:%s' \"$1\" > \"$2\"\n"), 0o755); err != nil {
		t.Fatalf("write enable script: %v", err)
	}
	if err := os.WriteFile(disableScript, []byte("#!/bin/sh\nprintf 'disabled:%s' \"$1\" > \"$2\"\n"), 0o755); err != nil {
		t.Fatalf("write disable script: %v", err)
	}
	validateOut := filepath.Join(root, "validate.out")
	enableOut := filepath.Join(root, "enable.out")
	disableOut := filepath.Join(root, "disable.out")
	manifest := `
name: echoer
enabled: true
tool:
  name: plugin.echoer
  command: ` + toolScript + `
  args_template:
    - "{{message}}"
  input_keys:
    - message
lifecycle:
  validate:
    - name: manifest-check
      command: ` + validateScript + `
      args_template:
        - "{{extension_name}}"
        - ` + validateOut + `
  on_enable:
    - name: enable-hook
      command: ` + enableScript + `
      args_template:
        - "{{extension_name}}"
        - ` + enableOut + `
  on_disable:
    - name: disable-hook
      command: ` + disableScript + `
      args_template:
        - "{{extension_name}}"
        - ` + disableOut + `
`
	if err := os.WriteFile(filepath.Join(pluginsDir, "echoer", "plugin.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	var persisted []ExtensionStateRecord
	manager := NewManager(config.Config{
		Extensions: config.ExtensionConfig{PluginsDir: pluginsDir},
	}, nil, func(context.Context) ([]ExtensionStateRecord, error) {
		return persisted, nil
	}, func(_ context.Context, kind, name string, enabled bool, hash string) error {
		persisted = []ExtensionStateRecord{{Kind: kind, Name: name, Enabled: enabled, Hash: hash}}
		return nil
	}, nil)
	if err := manager.Discover(context.Background()); err != nil {
		t.Fatalf("discover: %v", err)
	}
	result, err := manager.Validate(context.Background(), "admin", "plugin", "echoer")
	if err != nil {
		t.Fatalf("validate extension: %v", err)
	}
	if result["results"] == nil {
		t.Fatalf("expected validation results: %#v", result)
	}
	raw, err := os.ReadFile(validateOut)
	if err != nil || strings.TrimSpace(string(raw)) != "echoer" {
		t.Fatalf("unexpected validate hook output: %q err=%v", string(raw), err)
	}
	if err := manager.SetEnabled(context.Background(), "admin", "plugin", "echoer", false); err != nil {
		t.Fatalf("disable extension: %v", err)
	}
	raw, err = os.ReadFile(disableOut)
	if err != nil || strings.TrimSpace(string(raw)) != "disabled:echoer" {
		t.Fatalf("unexpected disable hook output: %q err=%v", string(raw), err)
	}
	if err := manager.SetEnabled(context.Background(), "admin", "plugin", "echoer", true); err != nil {
		t.Fatalf("enable extension: %v", err)
	}
	raw, err = os.ReadFile(enableOut)
	if err != nil || strings.TrimSpace(string(raw)) != "enabled:echoer" {
		t.Fatalf("unexpected enable hook output: %q err=%v", string(raw), err)
	}
}
