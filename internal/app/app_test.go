package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"hermes-agent/go/internal/config"
	"hermes-agent/go/internal/multiagent"
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
	if len(results[0].Trace) == 0 {
		t.Fatalf("expected child trace, got %#v", results[0])
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
	if len(results[0].Trace) == 0 {
		t.Fatalf("expected trace in result, got %#v", results[0])
	}
	if len(results[0].Risks) == 0 || !strings.Contains(results[0].Risks[0], "unknown.tool") {
		t.Fatalf("expected invalid tool risk, got %#v", results[0].Risks)
	}
}

func TestRunMultiAgentPlanUsesNativeToolCallingWhenAvailable(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		callCount++
		switch callCount {
		case 1:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{
						"finish_reason": "tool_calls",
						"message": map[string]any{
							"role":    "assistant",
							"content": "",
							"tool_calls": []map[string]any{
								{
									"id":   "call_history_1",
									"type": "function",
									"function": map[string]any{
										"name":      "session.history",
										"arguments": `{"limit":"2"}`,
									},
								},
							},
						},
					},
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{
						"finish_reason": "stop",
						"message": map[string]any{
							"role":    "assistant",
							"content": "Collected history and finished the delegated task.",
						},
					},
				},
			})
		}
	}))
	defer server.Close()

	cfg := config.Default()
	cfg.DataDir = filepath.Join(t.TempDir(), "data")
	cfg.CurrentModelProfile = ""
	cfg.LLM.BaseURL = server.URL
	cfg.LLM.Model = "test-model"
	cfg.LLM.APIKeyEnv = ""
	application, err := New(cfg)
	if err != nil {
		t.Fatalf("init app: %v", err)
	}
	defer application.Close()

	seedSession, err := application.Store.CreateSession(context.Background(), "admin", "seed-model", "seed", "seed")
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if err := application.Store.AddMessage(context.Background(), seedSession, "user", "alpha seed"); err != nil {
		t.Fatalf("seed message: %v", err)
	}

	plan, err := application.BuildMultiAgentPlan(context.Background(), "admin", "inspect", []multiagent.Task{
		{ID: "history", Goal: "inspect prior work", AllowedTools: []string{"session.history"}},
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
	if !strings.Contains(results[0].Summary, "[runtime=llm-toolcalls]") {
		t.Fatalf("expected native tool-calling runtime, got %q", results[0].Summary)
	}
	if len(results[0].Trace) < 2 || results[0].Trace[0].Tool != "session.history" {
		t.Fatalf("expected tool trace followed by final trace, got %#v", results[0].Trace)
	}
}
