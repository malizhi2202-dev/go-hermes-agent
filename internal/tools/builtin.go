package tools

import (
	"context"
	"fmt"
	"strings"
)

type BuiltinDeps struct {
	AppName        string
	Model          string
	ExecEnabled    bool
	ListSessions   func(ctx context.Context, username string, limit int) (any, error)
	Search         func(ctx context.Context, username, query, role string, sessionID int64, fromTime, toTime string, limit int) (any, error)
	MemoryEnabled  bool
	ReadMemory     func(ctx context.Context, username string) (any, error)
	WriteMemory    func(ctx context.Context, username, target, action, content, match string) (any, error)
	ExecuteCommand func(ctx context.Context, command string, args []string) (string, error)
	ExecuteProfile func(ctx context.Context, profile string, vars map[string]string, approved bool, capabilityToken string) (any, error)
	WriteAudit     func(ctx context.Context, username, action, detail string) error
}

func RegisterBuiltins(registry *Registry, deps BuiltinDeps) error {
	if err := registry.Register(Tool{
		Name:        "system.health",
		Description: "Return basic service health information.",
		InputKeys:   []string{},
		Handler: func(_ context.Context, _ map[string]any) (map[string]any, error) {
			return map[string]any{
				"ok":                true,
				"app_name":          deps.AppName,
				"model":             deps.Model,
				"execution_enabled": deps.ExecEnabled,
			}, nil
		},
	}); err != nil {
		return err
	}
	if err := registry.Register(Tool{
		Name:        "session.history",
		Description: "List recent sessions for the current authenticated user.",
		InputKeys:   []string{"username", "limit"},
		Handler: func(ctx context.Context, input map[string]any) (map[string]any, error) {
			username, _ := input["username"].(string)
			limit := 10
			if raw, ok := input["limit"].(float64); ok && int(raw) > 0 {
				limit = int(raw)
			}
			sessions, err := deps.ListSessions(ctx, username, limit)
			if err != nil {
				return nil, err
			}
			return map[string]any{"sessions": sessions}, nil
		},
	}); err != nil {
		return err
	}
	if err := registry.Register(Tool{
		Name:        "session.search",
		Description: "Search the authenticated user's historical session messages by keyword, role, session, and time window.",
		InputKeys:   []string{"username", "query", "role", "session_id", "from_time", "to_time", "limit"},
		Handler: func(ctx context.Context, input map[string]any) (map[string]any, error) {
			username, _ := input["username"].(string)
			query, _ := input["query"].(string)
			if strings.TrimSpace(query) == "" {
				return nil, fmt.Errorf("query is required")
			}
			role, _ := input["role"].(string)
			fromTime, _ := input["from_time"].(string)
			toTime, _ := input["to_time"].(string)
			var sessionID int64
			switch raw := input["session_id"].(type) {
			case float64:
				sessionID = int64(raw)
			case int64:
				sessionID = raw
			}
			limit := 10
			if raw, ok := input["limit"].(float64); ok && int(raw) > 0 {
				limit = int(raw)
			}
			results, err := deps.Search(ctx, username, query, role, sessionID, fromTime, toTime, limit)
			if err != nil {
				return nil, err
			}
			return map[string]any{"results": results}, nil
		},
	}); err != nil {
		return err
	}
	if err := registry.Register(Tool{
		Name:        "memory.read",
		Description: "Read the authenticated user's stored memory and user preference notes.",
		InputKeys:   []string{"username"},
		Handler: func(ctx context.Context, input map[string]any) (map[string]any, error) {
			if !deps.MemoryEnabled {
				return nil, fmt.Errorf("memory is disabled")
			}
			username, _ := input["username"].(string)
			snapshot, err := deps.ReadMemory(ctx, username)
			if err != nil {
				return nil, err
			}
			return map[string]any{"memory": snapshot}, nil
		},
	}); err != nil {
		return err
	}
	if err := registry.Register(Tool{
		Name:        "memory.write",
		Description: "Write to the authenticated user's stored memory using add, replace, remove, or read.",
		InputKeys:   []string{"username", "target", "action", "content", "match"},
		Handler: func(ctx context.Context, input map[string]any) (map[string]any, error) {
			if !deps.MemoryEnabled {
				return nil, fmt.Errorf("memory is disabled")
			}
			username, _ := input["username"].(string)
			target, _ := input["target"].(string)
			action, _ := input["action"].(string)
			content, _ := input["content"].(string)
			match, _ := input["match"].(string)
			snapshot, err := deps.WriteMemory(ctx, username, target, action, content, match)
			if err != nil {
				return nil, err
			}
			return map[string]any{"memory": snapshot}, nil
		},
	}); err != nil {
		return err
	}
	if err := registry.Register(Tool{
		Name:        "system.exec",
		Description: "Execute a command only when execution is enabled and the command is on the allowlist.",
		InputKeys:   []string{"command", "args"},
		Handler: func(ctx context.Context, input map[string]any) (map[string]any, error) {
			command, _ := input["command"].(string)
			if strings.TrimSpace(command) == "" {
				return nil, fmt.Errorf("command is required")
			}
			var args []string
			if rawArgs, ok := input["args"].([]any); ok {
				for _, arg := range rawArgs {
					if str, ok := arg.(string); ok && str != "" {
						args = append(args, str)
					}
				}
			}
			username, _ := input["username"].(string)
			_ = deps.WriteAudit(ctx, username, "system_exec_attempt", fmt.Sprintf("command=%s args=%d", command, len(args)))
			output, err := deps.ExecuteCommand(ctx, command, args)
			if err != nil {
				_ = deps.WriteAudit(ctx, username, "system_exec_denied", err.Error())
				return nil, err
			}
			_ = deps.WriteAudit(ctx, username, "system_exec_success", fmt.Sprintf("command=%s output_bytes=%d", command, len(output)))
			return map[string]any{"output": output}, nil
		},
	}); err != nil {
		return err
	}
	if err := registry.Register(Tool{
		Name:        "system.exec_profile",
		Description: "Execute a named allowlisted multi-step execution profile when execution is enabled, optionally requiring approval or a capability token.",
		InputKeys:   []string{"profile", "vars", "approved", "capability_token"},
		Handler: func(ctx context.Context, input map[string]any) (map[string]any, error) {
			profile, _ := input["profile"].(string)
			if strings.TrimSpace(profile) == "" {
				return nil, fmt.Errorf("profile is required")
			}
			vars := map[string]string{}
			if rawVars, ok := input["vars"].(map[string]any); ok {
				for key, value := range rawVars {
					if str, ok := value.(string); ok {
						vars[key] = str
					}
				}
			}
			approved := false
			switch raw := input["approved"].(type) {
			case bool:
				approved = raw
			case string:
				approved = strings.EqualFold(strings.TrimSpace(raw), "true")
			}
			capabilityToken, _ := input["capability_token"].(string)
			username, _ := input["username"].(string)
			_ = deps.WriteAudit(ctx, username, "system_exec_profile_attempt", fmt.Sprintf("profile=%s vars=%d approved=%t capability_token=%t", profile, len(vars), approved, strings.TrimSpace(capabilityToken) != ""))
			results, err := deps.ExecuteProfile(ctx, profile, vars, approved, capabilityToken)
			if err != nil {
				_ = deps.WriteAudit(ctx, username, "system_exec_profile_denied", fmt.Sprintf("profile=%s error=%s", profile, err.Error()))
				return nil, err
			}
			resultMap, _ := results.(map[string]any)
			stepCount := 0
			rollbackCount := 0
			if resultMap != nil {
				if steps, ok := resultMap["steps"].([]any); ok {
					stepCount = len(steps)
				}
				if rollback, ok := resultMap["rollback"].([]any); ok {
					rollbackCount = len(rollback)
				}
			}
			_ = deps.WriteAudit(ctx, username, "system_exec_profile_success", fmt.Sprintf("profile=%s steps=%d rollback_steps=%d", profile, stepCount, rollbackCount))
			return map[string]any{"results": results}, nil
		},
	}); err != nil {
		return err
	}
	return nil
}
