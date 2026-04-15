package extensions

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"go-hermes-agent/internal/tools"
)

var placeholderPattern = regexp.MustCompile(`^\{\{([a-zA-Z0-9_\-]+)\}\}$`)

func registerCommandTool(registry *tools.Registry, kind, name string, spec ToolSpec, audit func(context.Context, string, string, string) error) error {
	toolName := effectiveToolName(kind, name, spec)
	inputKeys := append([]string{"username"}, spec.InputKeys...)
	description := spec.Description
	if strings.TrimSpace(description) == "" {
		description = fmt.Sprintf("Execute the %s extension %s.", kind, name)
	}
	return registry.Register(tools.Tool{
		Name:        toolName,
		Description: description,
		InputKeys:   inputKeys,
		Handler: func(ctx context.Context, input map[string]any) (map[string]any, error) {
			username, _ := input["username"].(string)
			if audit != nil {
				_ = audit(ctx, username, kind+"_exec_attempt", fmt.Sprintf("name=%s tool=%s", name, toolName))
			}
			output, err := executeCommandSpec(ctx, spec, input)
			if err != nil {
				if audit != nil {
					_ = audit(ctx, username, kind+"_exec_denied", fmt.Sprintf("name=%s err=%s", name, err.Error()))
				}
				return nil, err
			}
			if audit != nil {
				_ = audit(ctx, username, kind+"_exec_success", fmt.Sprintf("name=%s bytes=%d", name, len(output)))
			}
			return map[string]any{"output": output}, nil
		},
	})
}

func effectiveToolName(kind, name string, spec ToolSpec) string {
	toolName := strings.TrimSpace(spec.Name)
	if toolName == "" {
		toolName = fmt.Sprintf("%s.%s", kind, sanitizeName(name))
	}
	return toolName
}

func executeCommandSpec(ctx context.Context, spec ToolSpec, input map[string]any) (string, error) {
	command := strings.TrimSpace(spec.Command)
	if command == "" {
		return "", fmt.Errorf("extension command is required")
	}
	args := make([]string, 0, len(spec.ArgsTemplate))
	for _, tmpl := range spec.ArgsTemplate {
		match := placeholderPattern.FindStringSubmatch(strings.TrimSpace(tmpl))
		if len(match) == 2 {
			value, err := inputValue(input, match[1])
			if err != nil {
				return "", err
			}
			args = append(args, value)
			continue
		}
		if err := validateArg(tmpl); err != nil {
			return "", err
		}
		args = append(args, tmpl)
	}
	timeout := spec.TimeoutSeconds
	if timeout <= 0 {
		timeout = 10
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, command, args...)
	if len(spec.Env) > 0 {
		env := make([]string, 0, len(spec.Env))
		for key, value := range spec.Env {
			env = append(env, key+"="+value)
		}
		cmd.Env = append(cmd.Environ(), env...)
	}
	output, err := cmd.CombinedOutput()
	if runCtx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("extension command timed out")
	}
	if err != nil {
		return "", fmt.Errorf("extension command failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func inputValue(input map[string]any, key string) (string, error) {
	value, ok := input[key]
	if !ok {
		return "", fmt.Errorf("missing input %q", key)
	}
	str, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("input %q must be a string", key)
	}
	str = strings.TrimSpace(str)
	if str == "" {
		return "", fmt.Errorf("input %q is required", key)
	}
	if err := validateArg(str); err != nil {
		return "", fmt.Errorf("input %q rejected: %w", key, err)
	}
	return str, nil
}

func validateArg(value string) error {
	if len(value) > 512 {
		return fmt.Errorf("arg too long")
	}
	if strings.ContainsAny(value, "\n\r;&|`$><") {
		return fmt.Errorf("arg contains forbidden shell metacharacters")
	}
	return nil
}

func sanitizeName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		default:
			builder.WriteRune('_')
		}
	}
	result := strings.Trim(builder.String(), "_")
	if result == "" {
		return "unnamed"
	}
	return result
}
