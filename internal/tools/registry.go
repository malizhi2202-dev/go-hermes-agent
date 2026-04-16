package tools

import (
	"context"
	"fmt"
	"sort"
)

// Handler executes one registered tool call.
type Handler func(ctx context.Context, input map[string]any) (map[string]any, error)

// Tool is one registered safe tool definition.
type Tool struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	InputKeys   []string `json:"input_keys"`
	Handler     Handler  `json:"-"`
}

// Registry holds the process-local tool allowlist.
type Registry struct {
	tools map[string]Tool
}

// New creates an empty tool registry.
func New() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds or replaces one tool definition.
func (r *Registry) Register(tool Tool) error {
	if tool.Name == "" {
		return fmt.Errorf("tool name is required")
	}
	if tool.Handler == nil {
		return fmt.Errorf("tool handler is required")
	}
	r.tools[tool.Name] = tool
	return nil
}

// Unregister removes tools by name.
func (r *Registry) Unregister(names ...string) {
	for _, name := range names {
		delete(r.tools, name)
	}
}

// List returns registered tools in stable name order.
func (r *Registry) List() []Tool {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]Tool, 0, len(names))
	for _, name := range names {
		result = append(result, r.tools[name])
	}
	return result
}

// Get returns a registered tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	tool, ok := r.tools[name]
	return tool, ok
}

// Execute dispatches a registered tool handler.
func (r *Registry) Execute(ctx context.Context, name string, input map[string]any) (map[string]any, error) {
	tool, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
	return tool.Handler(ctx, input)
}
