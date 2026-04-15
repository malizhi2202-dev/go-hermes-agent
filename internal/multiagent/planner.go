package multiagent

import "strings"

type Planner struct {
	policy Policy
}

func NewPlanner(policy Policy) *Planner {
	return &Planner{policy: policy}
}

func (p *Planner) Build(objective string, tasks []Task) (Plan, error) {
	mode := TaskModeSequential
	if canRunParallel(tasks) {
		mode = TaskModeParallel
	}
	maxConcurrent := 1
	if mode == TaskModeParallel {
		maxConcurrent = min(max(1, len(tasks)), p.policy.MaxConcurrent)
	}
	plan := Plan{
		Objective:     strings.TrimSpace(objective),
		Mode:          mode,
		MaxConcurrent: maxConcurrent,
		Tasks:         tasks,
	}
	return plan, p.policy.Validate(plan)
}

func canRunParallel(tasks []Task) bool {
	if len(tasks) <= 1 {
		return false
	}
	for _, task := range tasks {
		if len(task.WriteScopes) > 0 {
			return false
		}
	}
	return true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
