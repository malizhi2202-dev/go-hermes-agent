package multiagent

import (
	"context"
	"testing"
)

func TestPlannerBuildsParallelPlanForReadOnlyTasks(t *testing.T) {
	planner := NewPlanner(DefaultPolicy())
	plan, err := planner.Build("inspect project", []Task{
		{ID: "a", Goal: "inspect gateway", AllowedTools: []string{"session.search"}},
		{ID: "b", Goal: "inspect models", AllowedTools: []string{"session.search"}},
	})
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if plan.Mode != TaskModeParallel {
		t.Fatalf("expected parallel mode, got %s", plan.Mode)
	}
}

func TestPlannerFallsBackToSequentialForWriteScopes(t *testing.T) {
	planner := NewPlanner(DefaultPolicy())
	plan, err := planner.Build("edit project", []Task{
		{ID: "a", Goal: "edit gateway", WriteScopes: []string{"internal/gateway"}},
		{ID: "b", Goal: "edit models", WriteScopes: []string{"internal/models"}},
	})
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if plan.Mode != TaskModeSequential {
		t.Fatalf("expected sequential mode, got %s", plan.Mode)
	}
}

func TestPolicyRejectsBlockedToolsAndSharedWriteScope(t *testing.T) {
	policy := DefaultPolicy()
	err := policy.Validate(Plan{
		Objective:     "bad",
		Mode:          TaskModeParallel,
		MaxConcurrent: 2,
		Tasks: []Task{
			{ID: "a", Goal: "task", AllowedTools: []string{"delegate_task"}},
		},
	})
	if err == nil {
		t.Fatal("expected blocked tool validation error")
	}

	err = policy.Validate(Plan{
		Objective:     "bad",
		Mode:          TaskModeSequential,
		MaxConcurrent: 1,
		Tasks: []Task{
			{ID: "a", Goal: "task", WriteScopes: []string{"internal/api"}},
			{ID: "b", Goal: "task", WriteScopes: []string{"internal/api"}},
		},
	})
	if err == nil {
		t.Fatal("expected shared write scope validation error")
	}
}

func TestAggregatorDeduplicatesRiskAndFiles(t *testing.T) {
	agg := NewAggregator().Aggregate([]Result{
		{TaskID: "a", Status: ResultCompleted, Summary: "done a", Risks: []string{"r1"}, FilesChanged: []string{"a.go"}},
		{TaskID: "b", Status: ResultFailed, Summary: "done b", Risks: []string{"r1", "r2"}, FilesChanged: []string{"a.go", "b.go"}},
	})
	if agg.Completed != 1 || agg.Failed != 1 {
		t.Fatalf("unexpected counts: %#v", agg)
	}
	if len(agg.Risks) != 2 || len(agg.FilesChanged) != 2 {
		t.Fatalf("expected deduped risks/files: %#v", agg)
	}
}

func TestOrchestratorRunsSequentialAndAggregates(t *testing.T) {
	orch := NewOrchestrator(DefaultPolicy())
	plan, err := orch.BuildPlan("do work", []Task{
		{ID: "a", Goal: "one", WriteScopes: []string{"internal/api"}},
		{ID: "b", Goal: "two", WriteScopes: []string{"internal/models"}},
	})
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	results, aggregate, err := orch.Run(context.Background(), plan, func(_ context.Context, task Task) Result {
		return Result{TaskID: task.ID, Status: ResultCompleted, Summary: "finished " + task.ID}
	})
	if err != nil {
		t.Fatalf("run orchestrator: %v", err)
	}
	if len(results) != 2 || aggregate.Completed != 2 {
		t.Fatalf("unexpected run result: %#v %#v", results, aggregate)
	}
}
