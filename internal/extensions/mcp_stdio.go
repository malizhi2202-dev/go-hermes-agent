package extensions

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"go-hermes-agent/internal/config"
	"go-hermes-agent/internal/tools"
)

type mcpRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type mcpResponse struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type mcpClient interface {
	initialize() error
	request(method string, params any) (json.RawMessage, error)
	close()
}

// listMCPTools discovers tools from one configured MCP server.
func listMCPTools(ctx context.Context, name string, cfg config.MCPServerConfig) ([]MCPTool, error) {
	client, err := newMCPClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer client.close()
	if err := client.initialize(); err != nil {
		return nil, err
	}
	raw, err := client.request("tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var payload struct {
		Tools []struct {
			Name        string         `json:"name"`
			Description string         `json:"description"`
			InputSchema map[string]any `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode mcp tools for %s: %w", name, err)
	}
	result := make([]MCPTool, 0, len(payload.Tools))
	for _, toolDef := range payload.Tools {
		if !toolAllowed(toolDef.Name, cfg.IncludeTools, cfg.ExcludeTools) {
			continue
		}
		result = append(result, MCPTool{
			Name:        toolDef.Name,
			Description: toolDef.Description,
			InputSchema: toolDef.InputSchema,
		})
	}
	return result, nil
}

// registerMCPTool registers one discovered MCP tool as a safe Go tool.
func registerMCPTool(registry *tools.Registry, serverName string, cfg config.MCPServerConfig, toolDef MCPTool, audit func(context.Context, string, string, string) error) error {
	toolName := fmt.Sprintf("mcp.%s.%s", sanitizeName(serverName), sanitizeName(toolDef.Name))
	inputKeys := mcpInputKeys(toolDef.InputSchema)
	inputKeys = append([]string{"username"}, inputKeys...)
	description := strings.TrimSpace(toolDef.Description)
	if description == "" {
		description = fmt.Sprintf("Call MCP tool %s on server %s.", toolDef.Name, serverName)
	}
	return registry.Register(tools.Tool{
		Name:        toolName,
		Description: description,
		InputKeys:   inputKeys,
		Handler: func(ctx context.Context, input map[string]any) (map[string]any, error) {
			username, _ := input["username"].(string)
			if audit != nil {
				_ = audit(ctx, username, "mcp_call_attempt", fmt.Sprintf("server=%s tool=%s", serverName, toolDef.Name))
			}
			client, err := newMCPClient(ctx, cfg)
			if err != nil {
				if audit != nil {
					_ = audit(ctx, username, "mcp_call_denied", err.Error())
				}
				return nil, err
			}
			defer client.close()
			if err := client.initialize(); err != nil {
				if audit != nil {
					_ = audit(ctx, username, "mcp_call_denied", err.Error())
				}
				return nil, err
			}
			params := map[string]any{
				"name":      toolDef.Name,
				"arguments": stripUsername(input),
			}
			raw, err := client.request("tools/call", params)
			if err != nil {
				if audit != nil {
					_ = audit(ctx, username, "mcp_call_denied", err.Error())
				}
				return nil, err
			}
			var payload map[string]any
			if err := json.Unmarshal(raw, &payload); err != nil {
				return nil, fmt.Errorf("decode mcp response: %w", err)
			}
			if audit != nil {
				_ = audit(ctx, username, "mcp_call_success", fmt.Sprintf("server=%s tool=%s", serverName, toolDef.Name))
			}
			return map[string]any{"result": payload}, nil
		},
	})
}

func newMCPClient(ctx context.Context, cfg config.MCPServerConfig) (mcpClient, error) {
	switch normalizedMCPTransport(cfg) {
	case "http":
		return newMCPHTTPClient(ctx, cfg)
	default:
		return newMCPStdioClient(ctx, cfg)
	}
}

func mcpInputKeys(schema map[string]any) []string {
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil
	}
	keys := make([]string, 0, len(properties))
	for key := range properties {
		keys = append(keys, key)
	}
	sortStrings(keys)
	return keys
}

func stripUsername(input map[string]any) map[string]any {
	result := make(map[string]any, len(input))
	for key, value := range input {
		if key == "username" {
			continue
		}
		result[key] = value
	}
	return result
}

type mcpStdioClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Reader
}

type mcpHTTPClient struct {
	client *http.Client
	url    string
}

func newMCPStdioClient(ctx context.Context, cfg config.MCPServerConfig) (*mcpStdioClient, error) {
	timeout := cfg.TimeoutSeconds
	if timeout <= 0 {
		timeout = 15
	}
	runCtx, _ := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	cmd := exec.CommandContext(runCtx, cfg.Command, cfg.Args...)
	if len(cfg.Env) > 0 {
		env := make([]string, 0, len(cfg.Env))
		for key, value := range cfg.Env {
			env = append(env, key+"="+value)
		}
		cmd.Env = append(cmd.Environ(), env...)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &mcpStdioClient{
		cmd:    cmd,
		stdin:  stdin,
		reader: bufio.NewReader(stdout),
	}, nil
}

func newMCPHTTPClient(ctx context.Context, cfg config.MCPServerConfig) (*mcpHTTPClient, error) {
	timeout := cfg.TimeoutSeconds
	if timeout <= 0 {
		timeout = 15
	}
	url := strings.TrimSpace(cfg.URL)
	if url == "" {
		return nil, fmt.Errorf("mcp http url is required")
	}
	return &mcpHTTPClient{
		client: &http.Client{Timeout: time.Duration(timeout) * time.Second},
		url:    url,
	}, nil
}

func (c *mcpStdioClient) close() {
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
		_, _ = c.cmd.Process.Wait()
	}
}

func (c *mcpStdioClient) initialize() error {
	if _, err := c.request("initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "hermes-go",
			"version": "0.1.0",
		},
	}); err != nil {
		return err
	}
	return c.notify("notifications/initialized", map[string]any{})
}

func (c *mcpHTTPClient) close() {}

func (c *mcpHTTPClient) initialize() error {
	if _, err := c.request("initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "hermes-go",
			"version": "0.1.0",
		},
	}); err != nil {
		return err
	}
	_, err := c.request("notifications/initialized", map[string]any{})
	return err
}

func (c *mcpStdioClient) notify(method string, params any) error {
	return c.send(mcpRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	})
}

func (c *mcpStdioClient) request(method string, params any) (json.RawMessage, error) {
	if err := c.send(mcpRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  method,
		Params:  params,
	}); err != nil {
		return nil, err
	}
	response, err := c.readResponse()
	if err != nil {
		return nil, err
	}
	if response.Error != nil {
		return nil, fmt.Errorf("mcp error %d: %s", response.Error.Code, response.Error.Message)
	}
	return response.Result, nil
}

func (c *mcpHTTPClient) request(method string, params any) (json.RawMessage, error) {
	body, err := json.Marshal(mcpRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("mcp http status %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return json.RawMessage(`{}`), nil
	}
	var response mcpResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, err
	}
	if response.Error != nil {
		return nil, fmt.Errorf("mcp error %d: %s", response.Error.Code, response.Error.Message)
	}
	return response.Result, nil
}

func (c *mcpStdioClient) send(payload mcpRequest) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	frame := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
	_, err = io.WriteString(c.stdin, frame)
	return err
}

func (c *mcpStdioClient) readResponse() (*mcpResponse, error) {
	length := 0
	for {
		line, err := c.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			value := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			length, err = strconv.Atoi(value)
			if err != nil {
				return nil, err
			}
		}
	}
	if length <= 0 {
		return nil, fmt.Errorf("invalid mcp content length")
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(c.reader, body); err != nil {
		return nil, err
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return nil, fmt.Errorf("empty mcp response")
	}
	var response mcpResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func toolAllowed(name string, include, exclude []string) bool {
	if len(include) > 0 {
		matched := false
		for _, item := range include {
			if item == name {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	for _, item := range exclude {
		if item == name {
			return false
		}
	}
	return true
}

func sortStrings(values []string) {
	for i := 0; i < len(values); i++ {
		for j := i + 1; j < len(values); j++ {
			if values[j] < values[i] {
				values[i], values[j] = values[j], values[i]
			}
		}
	}
}

func normalizedMCPTransport(cfg config.MCPServerConfig) string {
	transport := strings.ToLower(strings.TrimSpace(cfg.Transport))
	if transport == "" {
		return "stdio"
	}
	return transport
}
