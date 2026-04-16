package execution

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"hermes-agent/go/internal/config"
)

// Executor runs a tightly constrained allowlisted command.
type Executor struct {
	enabled         bool
	timeout         time.Duration
	allowedCommands map[string]struct{}
	maxArgs         int
	maxArgLength    int
	maxOutputBytes  int
	commandRules    map[string]config.CommandRule
}

// NewExecutor builds an executor from execution config.
func NewExecutor(cfg config.ExecutionConfig) *Executor {
	allowed := make(map[string]struct{}, len(cfg.AllowedCommands))
	for _, command := range cfg.AllowedCommands {
		command = strings.TrimSpace(command)
		if command != "" {
			allowed[command] = struct{}{}
		}
	}
	return &Executor{
		enabled:         cfg.Enabled,
		timeout:         time.Duration(cfg.TimeoutSeconds) * time.Second,
		allowedCommands: allowed,
		maxArgs:         cfg.MaxArgs,
		maxArgLength:    cfg.MaxArgLength,
		maxOutputBytes:  cfg.MaxOutputBytes,
		commandRules:    cfg.CommandRules,
	}
}

// Execute validates a command against policy and runs it without a shell.
func (e *Executor) Execute(ctx context.Context, command string, args []string) (string, error) {
	if !e.enabled {
		return "", fmt.Errorf("dynamic execution is disabled")
	}
	if _, ok := e.allowedCommands[command]; !ok {
		return "", fmt.Errorf("command %q is not in the allowlist", command)
	}
	maxArgs := e.maxArgs
	maxArgLength := e.maxArgLength
	maxOutputBytes := e.maxOutputBytes
	allowedPrefixes := []string(nil)
	deniedSubstrings := []string(nil)
	if rule, ok := e.commandRules[command]; ok {
		if rule.MaxArgs > 0 {
			maxArgs = rule.MaxArgs
		}
		if rule.MaxArgLength > 0 {
			maxArgLength = rule.MaxArgLength
		}
		if rule.MaxOutputBytes > 0 {
			maxOutputBytes = rule.MaxOutputBytes
		}
		allowedPrefixes = rule.AllowedArgPrefixes
		deniedSubstrings = rule.DeniedSubstrings
	}
	if len(args) > maxArgs {
		return "", fmt.Errorf("too many args: %d > %d", len(args), maxArgs)
	}
	for _, arg := range args {
		if len(arg) > maxArgLength {
			return "", fmt.Errorf("arg too long")
		}
		if strings.ContainsAny(arg, "\n\r;&|`$><") {
			return "", fmt.Errorf("arg contains forbidden shell metacharacters")
		}
		for _, denied := range deniedSubstrings {
			if denied != "" && strings.Contains(arg, denied) {
				return "", fmt.Errorf("arg contains denied substring")
			}
		}
		if len(allowedPrefixes) > 0 && strings.HasPrefix(arg, "-") {
			allowed := false
			for _, prefix := range allowedPrefixes {
				if strings.HasPrefix(arg, prefix) {
					allowed = true
					break
				}
			}
			if !allowed {
				return "", fmt.Errorf("arg prefix is not allowed")
			}
		}
	}
	runCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, command, args...)
	output, err := cmd.CombinedOutput()
	if runCtx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("command timed out")
	}
	if err != nil {
		return "", fmt.Errorf("command failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if len(output) > maxOutputBytes {
		output = output[:maxOutputBytes]
	}
	return strings.TrimSpace(string(output)), nil
}
