package execution

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"go-hermes-agent/internal/config"
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
	profiles        map[string]config.ExecutionProfile
}

// StepResult records the result of one execution-chain step.
type StepResult struct {
	Name    string `json:"name"`
	Command string `json:"command"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
}

// ProfileRunResult records the outcome of a controlled execution profile.
type ProfileRunResult struct {
	Profile          string       `json:"profile"`
	Approved         bool         `json:"approved"`
	CapabilityScoped bool         `json:"capability_scoped"`
	Steps            []StepResult `json:"steps"`
	Rollback         []StepResult `json:"rollback,omitempty"`
}

var execPlaceholderPattern = regexp.MustCompile(`^\{\{([a-zA-Z0-9_\-]+)\}\}$`)

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
		profiles:        cfg.Profiles,
	}
}

// Execute validates a command against policy and runs it without a shell.
func (e *Executor) Execute(ctx context.Context, command string, args []string) (string, error) {
	if !e.enabled {
		return "", fmt.Errorf("dynamic execution is disabled")
	}
	return e.executeValidated(ctx, command, args)
}

// ExecuteProfile runs a named controlled multi-step execution profile.
func (e *Executor) ExecuteProfile(ctx context.Context, profileName string, vars map[string]string, approved bool, capabilityToken string) (ProfileRunResult, error) {
	if !e.enabled {
		return ProfileRunResult{}, fmt.Errorf("dynamic execution is disabled")
	}
	profile, ok := e.profiles[strings.TrimSpace(profileName)]
	if !ok {
		return ProfileRunResult{}, fmt.Errorf("execution profile %q is not defined", profileName)
	}
	if profile.RequireApproval && !approved {
		return ProfileRunResult{}, fmt.Errorf("execution profile %q requires approval", profileName)
	}
	if token := strings.TrimSpace(profile.CapabilityToken); token != "" && token != strings.TrimSpace(capabilityToken) {
		return ProfileRunResult{}, fmt.Errorf("execution profile %q requires a valid capability token", profileName)
	}
	result := ProfileRunResult{
		Profile:          profileName,
		Approved:         approved,
		CapabilityScoped: strings.TrimSpace(profile.CapabilityToken) != "",
		Steps:            make([]StepResult, 0, len(profile.Steps)),
	}
	for index, step := range profile.Steps {
		args, err := renderExecutionArgs(step.ArgsTemplate, vars)
		if err != nil {
			return result, fmt.Errorf("profile %s step %d: %w", profileName, index, err)
		}
		output, execErr := e.executeValidated(ctx, step.Command, args)
		stepResult := StepResult{
			Name:    firstNonEmpty(step.Name, fmt.Sprintf("step-%d", index+1)),
			Command: step.Command,
			Output:  output,
		}
		if execErr != nil {
			stepResult.Error = execErr.Error()
			result.Steps = append(result.Steps, stepResult)
			if profile.RollbackProfile != "" {
				rollback, _ := e.executeRollbackProfile(ctx, profile.RollbackProfile, vars)
				result.Rollback = rollback
			}
			if !profile.ContinueOnError {
				return result, execErr
			}
			continue
		}
		result.Steps = append(result.Steps, stepResult)
	}
	return result, nil
}

func (e *Executor) executeValidated(ctx context.Context, command string, args []string) (string, error) {
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

func renderExecutionArgs(templates []string, vars map[string]string) ([]string, error) {
	args := make([]string, 0, len(templates))
	for _, tmpl := range templates {
		match := execPlaceholderPattern.FindStringSubmatch(strings.TrimSpace(tmpl))
		if len(match) == 2 {
			value := strings.TrimSpace(vars[match[1]])
			if value == "" {
				return nil, fmt.Errorf("missing profile variable %q", match[1])
			}
			args = append(args, value)
			continue
		}
		args = append(args, tmpl)
	}
	return args, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (e *Executor) executeRollbackProfile(ctx context.Context, profileName string, vars map[string]string) ([]StepResult, error) {
	profile, ok := e.profiles[strings.TrimSpace(profileName)]
	if !ok {
		return nil, fmt.Errorf("rollback profile %q is not defined", profileName)
	}
	results := make([]StepResult, 0, len(profile.Steps))
	for index, step := range profile.Steps {
		args, err := renderExecutionArgs(step.ArgsTemplate, vars)
		if err != nil {
			return results, err
		}
		output, execErr := e.executeValidated(ctx, step.Command, args)
		result := StepResult{
			Name:    firstNonEmpty(step.Name, fmt.Sprintf("rollback-%d", index+1)),
			Command: step.Command,
			Output:  output,
		}
		if execErr != nil {
			result.Error = execErr.Error()
			results = append(results, result)
			return results, execErr
		}
		results = append(results, result)
	}
	return results, nil
}
