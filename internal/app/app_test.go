package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"go-hermes-agent/internal/config"
	"go-hermes-agent/internal/multiagent"
	"go-hermes-agent/internal/store"
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
	foundHistoryTool := false
	for _, step := range results[0].Trace {
		if step.Tool == "session.history" {
			foundHistoryTool = true
			break
		}
	}
	if len(results[0].Trace) < 3 || !foundHistoryTool {
		t.Fatalf("expected snapshot + tool + final trace, got %#v", results[0].Trace)
	}
}

func TestResumeBasisRecoversHistoryAndToolStates(t *testing.T) {
	traceRows := []store.MultiAgentTraceRecord{
		{Iteration: 1, Type: "snapshot", SnapshotJSON: `{"current_prompt":"continue delegated task","history_len":2,"next_iteration":3,"runtime":"llm-toolcalls","tool_risks":["tool timeout"],"history":[{"role":"assistant","content":"Inspecting prior work."},{"role":"tool","name":"session.search","content":"{\"items\":[1]}"}]}`},
		{Iteration: 1, Tool: "session.search", OutputJSON: `{"items":[1]}`},
		{Iteration: 2, Tool: "memory.read", OutputJSON: `{"memory":"alpha"}`},
		{Iteration: 3, Tool: "session.history", Error: "timeout", InputJSON: `{"limit":"2"}`},
	}
	messages := []store.Message{
		{Role: "user", Content: "ignored"},
		{Role: "assistant", Content: "Thinking about the delegated task."},
		{Role: "tool", Content: `session.search result: {"items":[1]}`},
		{Role: "assistant", Content: "I should read memory next."},
		{Role: "tool", Content: `memory.read result: {"memory":"alpha"}`},
	}

	basis := deriveResumeBasis(traceRows, messages)
	if basis.LastIteration != 3 {
		t.Fatalf("expected last iteration 3, got %#v", basis)
	}
	if basis.RecoveredHistoryMessage != 4 {
		t.Fatalf("expected recovered history message count, got %#v", basis)
	}
	if basis.LastSnapshotPrompt != "continue delegated task" || basis.LastSnapshotHistoryLen != 2 {
		t.Fatalf("expected recovered snapshot prompt/history len, got %#v", basis)
	}
	if basis.LastSnapshotNextIteration != 3 || basis.LastSnapshotRuntime != "llm-toolcalls" {
		t.Fatalf("expected recovered next iteration/runtime, got %#v", basis)
	}
	if len(basis.LastSnapshotToolRisks) != 1 || basis.LastSnapshotToolRisks[0] != "tool timeout" {
		t.Fatalf("expected recovered tool risks, got %#v", basis.LastSnapshotToolRisks)
	}
	if len(basis.LastSnapshotHistory) != 2 || basis.LastSnapshotHistory[1].Name != "session.search" {
		t.Fatalf("expected exact snapshot history, got %#v", basis.LastSnapshotHistory)
	}
	if len(basis.RecoveredToolStates) != 2 || basis.RecoveredToolStates[0].Tool != "session.search" || basis.RecoveredToolStates[1].Tool != "memory.read" {
		t.Fatalf("unexpected recovered tool states: %#v", basis.RecoveredToolStates)
	}

	seed := buildResumeSeedHistory(messages, basis)
	if len(seed) != 4 {
		t.Fatalf("expected recovered seed history, got %#v", seed)
	}
	if seed[0].Role != "assistant" || seed[1].Role != "tool" || seed[1].Name != "session.search" || !strings.Contains(seed[1].Content, `"items":[1]`) {
		t.Fatalf("unexpected parsed tool history: %#v", seed)
	}

	loopSeed := buildResumeLoopSeed(basis)
	if loopSeed.CurrentPrompt != "continue delegated task" || loopSeed.NextIteration != 3 || loopSeed.Runtime != "llm-toolcalls" {
		t.Fatalf("unexpected loop seed: %#v", loopSeed)
	}
	if len(loopSeed.ToolRisks) != 1 || loopSeed.ToolRisks[0] != "tool timeout" {
		t.Fatalf("unexpected loop seed risks: %#v", loopSeed.ToolRisks)
	}
}

func TestRunMultiAgentPlanCanUseExecProfileTool(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
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
									"id":   "call_exec_profile_1",
									"type": "function",
									"function": map[string]any{
										"name":      "system.exec_profile",
										"arguments": `{"profile":"system-health","approved":true}`,
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
							"content": "Execution profile completed.",
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
	cfg.Execution.Enabled = true
	cfg.Execution.AllowedCommands = []string{"echo", "date"}
	application, err := New(cfg)
	if err != nil {
		t.Fatalf("init app: %v", err)
	}
	defer application.Close()

	plan, err := application.BuildMultiAgentPlan(context.Background(), "admin", "inspect", []multiagent.Task{
		{ID: "exec", Goal: "run safe system health execution profile", AllowedTools: []string{"system.exec_profile"}},
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
	foundSnapshot := false
	foundVerifiedTool := false
	for _, step := range results[0].Trace {
		if step.Type == "snapshot" && step.Snapshot != nil {
			foundSnapshot = true
		}
		if step.Tool == "system.exec_profile" && step.Verifier != "" && step.VerificationClass == "ok" {
			foundVerifiedTool = true
		}
	}
	if !foundSnapshot || !foundVerifiedTool {
		t.Fatalf("expected snapshot and verified tool trace, got %#v", results[0].Trace)
	}
}
