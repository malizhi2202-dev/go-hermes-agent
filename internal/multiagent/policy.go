package multiagent

import (
	"fmt"
	"slices"
	"strings"
)

var blockedTools = []string{
	"delegate_task",
	"system.exec",
	"browser.navigate",
	"browser.click",
	"memory.write",
}

type Policy struct {
	MaxConcurrent int
}

func DefaultPolicy() Policy {
	return Policy{MaxConcurrent: 3}
}

func (p Policy) Validate(plan Plan) error {
	if strings.TrimSpace(plan.Objective) == "" {
		return fmt.Errorf("objective is required")
	}
	if len(plan.Tasks) == 0 {
		return fmt.Errorf("at least one task is required")
	}
	if plan.Mode != TaskModeSequential && plan.Mode != TaskModeParallel {
		return fmt.Errorf("invalid mode %q", plan.Mode)
	}
	maxConcurrent := p.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 3
	}
	if plan.MaxConcurrent <= 0 {
		plan.MaxConcurrent = 1
	}
	if plan.MaxConcurrent > maxConcurrent {
		return fmt.Errorf("max_concurrent exceeds policy limit")
	}
	seenIDs := map[string]struct{}{}
	writeOwners := map[string]string{}
	for _, task := range plan.Tasks {
		if strings.TrimSpace(task.ID) == "" {
			return fmt.Errorf("task id is required")
		}
		if _, ok := seenIDs[task.ID]; ok {
			return fmt.Errorf("duplicate task id %q", task.ID)
		}
		seenIDs[task.ID] = struct{}{}
		if strings.TrimSpace(task.Goal) == "" {
			return fmt.Errorf("task %q goal is required", task.ID)
		}
		for _, tool := range task.AllowedTools {
			if slices.Contains(blockedTools, tool) {
				return fmt.Errorf("task %q uses blocked tool %q", task.ID, tool)
			}
		}
		for _, scope := range task.WriteScopes {
			scope = strings.TrimSpace(scope)
			if scope == "" {
				continue
			}
			if owner, ok := writeOwners[scope]; ok && owner != task.ID {
				return fmt.Errorf("write scope %q is owned by both %q and %q", scope, owner, task.ID)
			}
			writeOwners[scope] = task.ID
		}
	}
	return nil
}
