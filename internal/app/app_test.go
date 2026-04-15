package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"go-hermes-agent/internal/config"
	"go-hermes-agent/internal/multiagent"
)

func TestRunMultiAgentPlanFallsBackToStubWithoutAPIKey(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = filepath.Join(t.TempDir(), "data")
	application, err := New(cfg)
	if err != nil {
		t.Fatalf("init app: %v", err)
	}
	defer application.Close()

	plan, err := application.BuildMultiAgentPlan(context.Background(), "admin", "inspect", []multiagent.Task{
		{ID: "gateway", Goal: "inspect gateway", AllowedTools: []string{"session.search"}},
	})
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	results, aggregate, err := application.RunMultiAgentPlan(context.Background(), "admin", plan)
	if err != nil {
		t.Fatalf("run plan: %v", err)
	}
	if aggregate.Completed != 1 || len(results) != 1 {
		t.Fatalf("unexpected aggregate/results: %#v %#v", aggregate, results)
	}
	if !strings.Contains(results[0].Summary, "[runtime=stub]") {
		t.Fatalf("expected stub runtime summary, got %q", results[0].Summary)
	}
	if results[0].ChildSessionID == 0 {
		t.Fatal("expected child session id to be recorded")
	}
	if aggregate.ParentSessionID == 0 {
		t.Fatal("expected parent session id to be recorded")
	}
	sessions, err := application.Store.ListSessionsPage(context.Background(), "admin", 10, 0)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) < 2 {
		t.Fatalf("expected parent + child sessions, got %#v", sessions)
	}
}

func TestRunMultiAgentPlanReportsUnknownAllowedTools(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = filepath.Join(t.TempDir(), "data")
	application, err := New(cfg)
	if err != nil {
		t.Fatalf("init app: %v", err)
	}
	defer application.Close()

	plan, err := application.BuildMultiAgentPlan(context.Background(), "admin", "inspect", []multiagent.Task{
		{ID: "gateway", Goal: "inspect gateway", AllowedTools: []string{"session.search", "unknown.tool"}, HistoryWindow: 2},
	})
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	results, _, err := application.RunMultiAgentPlan(context.Background(), "admin", plan)
	if err != nil {
		t.Fatalf("run plan: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %#v", results)
	}
	if len(results[0].Risks) == 0 || !strings.Contains(results[0].Risks[0], "unknown.tool") {
		t.Fatalf("expected invalid tool risk, got %#v", results[0].Risks)
	}
}
