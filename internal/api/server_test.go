package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"hermes-agent/go/internal/app"
	"hermes-agent/go/internal/config"
)

func TestToolsEndpointRequiresAuthAndReturnsTools(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = filepath.Join(t.TempDir(), "data")
	application, err := app.New(cfg)
	if err != nil {
		t.Fatalf("init app: %v", err)
	}
	defer application.Close()

	if err := application.Auth.InitAdmin(context.Background(), "admin", "ChangeMe123!"); err != nil {
		t.Fatalf("init admin: %v", err)
	}
	token, err := application.Auth.Login(context.Background(), "admin", "ChangeMe123!")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	server := New(application)

	req := httptest.NewRequest(http.MethodGet, "/v1/tools", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var tools []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &tools); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(tools) == 0 {
		t.Fatal("expected non-empty tools response")
	}
}

func TestSearchEndpointReturnsHistoryMatches(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = filepath.Join(t.TempDir(), "data")
	application, err := app.New(cfg)
	if err != nil {
		t.Fatalf("init app: %v", err)
	}
	defer application.Close()

	if err := application.Auth.InitAdmin(context.Background(), "admin", "ChangeMe123!"); err != nil {
		t.Fatalf("init admin: %v", err)
	}
	token, err := application.Auth.Login(context.Background(), "admin", "ChangeMe123!")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	sessionID, err := application.Store.CreateSession(context.Background(), "admin", "test-model", "please remember alpha release", "stored")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := application.Store.AddMessage(context.Background(), sessionID, "user", "please remember alpha release"); err != nil {
		t.Fatalf("add message: %v", err)
	}
	if err := application.Store.AddMessage(context.Background(), sessionID, "assistant", "stored"); err != nil {
		t.Fatalf("add assistant message: %v", err)
	}

	server := New(application)
	req := httptest.NewRequest(http.MethodGet, "/v1/search?q=alpha&role=user&from="+time.Now().UTC().Add(-time.Hour).Format(time.RFC3339), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	var results []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected search results")
	}
}

func TestAuditEndpointReturnsRecords(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = filepath.Join(t.TempDir(), "data")
	application, err := app.New(cfg)
	if err != nil {
		t.Fatalf("init app: %v", err)
	}
	defer application.Close()

	if err := application.Auth.InitAdmin(context.Background(), "admin", "ChangeMe123!"); err != nil {
		t.Fatalf("init admin: %v", err)
	}
	token, err := application.Auth.Login(context.Background(), "admin", "ChangeMe123!")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if err := application.Store.WriteAudit(context.Background(), "admin", "system_exec_attempt", "command=echo args=1"); err != nil {
		t.Fatalf("write audit: %v", err)
	}

	server := New(application)
	req := httptest.NewRequest(http.MethodGet, "/v1/audit?action=system_exec_attempt", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	var records []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &records); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("expected audit records")
	}
}

func TestHistoryEndpointSupportsPagination(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = filepath.Join(t.TempDir(), "data")
	application, err := app.New(cfg)
	if err != nil {
		t.Fatalf("init app: %v", err)
	}
	defer application.Close()

	if err := application.Auth.InitAdmin(context.Background(), "admin", "ChangeMe123!"); err != nil {
		t.Fatalf("init admin: %v", err)
	}
	token, err := application.Auth.Login(context.Background(), "admin", "ChangeMe123!")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	for i := 0; i < 3; i++ {
		sessionID, err := application.Store.CreateSession(context.Background(), "admin", "test-model", "p", "r")
		if err != nil {
			t.Fatalf("create session: %v", err)
		}
		if err := application.Store.AddMessage(context.Background(), sessionID, "user", "hello"); err != nil {
			t.Fatalf("add message: %v", err)
		}
		if err := application.Store.AddMessage(context.Background(), sessionID, "assistant", "world"); err != nil {
			t.Fatalf("add message: %v", err)
		}
	}

	server := New(application)
	req := httptest.NewRequest(http.MethodGet, "/v1/history?limit=2&messages_limit=1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	var payload []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(payload))
	}
}

func TestExecutionAuditEndpointFiltersExecEvents(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = filepath.Join(t.TempDir(), "data")
	application, err := app.New(cfg)
	if err != nil {
		t.Fatalf("init app: %v", err)
	}
	defer application.Close()
	if err := application.Auth.InitAdmin(context.Background(), "admin", "ChangeMe123!"); err != nil {
		t.Fatalf("init admin: %v", err)
	}
	token, err := application.Auth.Login(context.Background(), "admin", "ChangeMe123!")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	_ = application.Store.WriteAudit(context.Background(), "admin", "system_exec_attempt", "command=echo")
	_ = application.Store.WriteAudit(context.Background(), "admin", "chat", "session recorded")

	server := New(application)
	req := httptest.NewRequest(http.MethodGet, "/v1/audit/execution", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	var records []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &records); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 execution audit record, got %d", len(records))
	}
}

func TestMultiAgentPlanAndRunEndpoints(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = filepath.Join(t.TempDir(), "data")
	application, err := app.New(cfg)
	if err != nil {
		t.Fatalf("init app: %v", err)
	}
	defer application.Close()
	if err := application.Auth.InitAdmin(context.Background(), "admin", "ChangeMe123!"); err != nil {
		t.Fatalf("init admin: %v", err)
	}
	token, err := application.Auth.Login(context.Background(), "admin", "ChangeMe123!")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	server := New(application)

	planBody := []byte(`{"objective":"inspect codebase","tasks":[{"id":"gateway","goal":"inspect gateway","allowed_tools":["session.search"]},{"id":"models","goal":"inspect models","allowed_tools":["session.search"]}]}`)
	planReq := httptest.NewRequest(http.MethodPost, "/v1/multiagent/plan", bytes.NewReader(planBody))
	planReq.Header.Set("Authorization", "Bearer "+token)
	planRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(planRec, planReq)
	if planRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", planRec.Code, planRec.Body.String())
	}

	var planPayload map[string]any
	if err := json.Unmarshal(planRec.Body.Bytes(), &planPayload); err != nil {
		t.Fatalf("decode plan response: %v", err)
	}
	if planPayload["mode"] != "parallel" {
		t.Fatalf("expected parallel mode, got %#v", planPayload["mode"])
	}

	runReq := httptest.NewRequest(http.MethodPost, "/v1/multiagent/run", bytes.NewReader(planRec.Body.Bytes()))
	runReq.Header.Set("Authorization", "Bearer "+token)
	runRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(runRec, runReq)
	if runRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", runRec.Code, runRec.Body.String())
	}
	var runPayload map[string]any
	if err := json.Unmarshal(runRec.Body.Bytes(), &runPayload); err != nil {
		t.Fatalf("decode run response: %v", err)
	}
	results, ok := runPayload["results"].([]any)
	if !ok || len(results) != 2 {
		t.Fatalf("unexpected results payload: %#v", runPayload["results"])
	}
	aggregate, ok := runPayload["aggregate"].(map[string]any)
	if !ok || aggregate["completed"] != float64(2) {
		t.Fatalf("unexpected aggregate payload: %#v", runPayload["aggregate"])
	}
	if aggregate["parent_session_id"] == nil {
		t.Fatalf("expected parent_session_id in aggregate: %#v", aggregate)
	}
	firstResult, ok := results[0].(map[string]any)
	if !ok || firstResult["child_session_id"] == nil {
		t.Fatalf("expected child_session_id in first result: %#v", results[0])
	}
	if firstResult["trace"] == nil {
		t.Fatalf("expected trace in first result: %#v", results[0])
	}

	parentSessionID := int64(aggregate["parent_session_id"].(float64))
	traceReq := httptest.NewRequest(http.MethodGet, "/v1/multiagent/traces?parent_session_id="+strconv.FormatInt(parentSessionID, 10), nil)
	traceReq.Header.Set("Authorization", "Bearer "+token)
	traceRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(traceRec, traceReq)
	if traceRec.Code != http.StatusOK {
		t.Fatalf("expected trace status 200, got %d body=%s", traceRec.Code, traceRec.Body.String())
	}
	var tracePayload []map[string]any
	if err := json.Unmarshal(traceRec.Body.Bytes(), &tracePayload); err != nil {
		t.Fatalf("decode trace response: %v", err)
	}
	if len(tracePayload) == 0 {
		t.Fatal("expected persisted multiagent traces")
	}

	childSessionID := int64(firstResult["child_session_id"].(float64))
	replayReq := httptest.NewRequest(http.MethodGet, "/v1/multiagent/replay?child_session_id="+strconv.FormatInt(childSessionID, 10), nil)
	replayReq.Header.Set("Authorization", "Bearer "+token)
	replayRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(replayRec, replayReq)
	if replayRec.Code != http.StatusOK {
		t.Fatalf("expected replay status 200, got %d body=%s", replayRec.Code, replayRec.Body.String())
	}
	var replayPayload map[string]any
	if err := json.Unmarshal(replayRec.Body.Bytes(), &replayPayload); err != nil {
		t.Fatalf("decode replay response: %v", err)
	}
	if replayPayload["recovery_hint"] == nil || replayPayload["trace"] == nil {
		t.Fatalf("expected replay payload with hint and trace, got %#v", replayPayload)
	}
	if replayPayload["resume_basis"] == nil {
		t.Fatalf("expected replay payload with resume_basis, got %#v", replayPayload)
	}

	summaryReq := httptest.NewRequest(http.MethodGet, "/v1/multiagent/traces/summary?parent_session_id="+strconv.FormatInt(parentSessionID, 10), nil)
	summaryReq.Header.Set("Authorization", "Bearer "+token)
	summaryRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(summaryRec, summaryReq)
	if summaryRec.Code != http.StatusOK {
		t.Fatalf("expected summary status 200, got %d body=%s", summaryRec.Code, summaryRec.Body.String())
	}
	var summaryPayload []map[string]any
	if err := json.Unmarshal(summaryRec.Body.Bytes(), &summaryPayload); err != nil {
		t.Fatalf("decode summary response: %v", err)
	}
	if len(summaryPayload) == 0 {
		t.Fatal("expected trace summary payload")
	}

	hotReq := httptest.NewRequest(http.MethodGet, "/v1/multiagent/traces/hotspots?parent_session_id="+strconv.FormatInt(parentSessionID, 10), nil)
	hotReq.Header.Set("Authorization", "Bearer "+token)
	hotRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(hotRec, hotReq)
	if hotRec.Code != http.StatusOK {
		t.Fatalf("expected hotspots status 200, got %d body=%s", hotRec.Code, hotRec.Body.String())
	}
	var hotPayload []map[string]any
	if err := json.Unmarshal(hotRec.Body.Bytes(), &hotPayload); err != nil {
		t.Fatalf("decode hotspots response: %v", err)
	}
	if len(hotPayload) == 0 {
		t.Fatal("expected hotspot payload")
	}

	timeReq := httptest.NewRequest(http.MethodGet, "/v1/multiagent/traces/summary?parent_session_id="+strconv.FormatInt(parentSessionID, 10)+"&from="+url.QueryEscape(time.Now().UTC().Add(-time.Hour).Format(time.RFC3339))+"&to="+url.QueryEscape(time.Now().UTC().Add(time.Hour).Format(time.RFC3339)), nil)
	timeReq.Header.Set("Authorization", "Bearer "+token)
	timeRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(timeRec, timeReq)
	if timeRec.Code != http.StatusOK {
		t.Fatalf("expected time-filtered summary status 200, got %d body=%s", timeRec.Code, timeRec.Body.String())
	}

	failReq := httptest.NewRequest(http.MethodGet, "/v1/multiagent/traces/failures?parent_session_id="+strconv.FormatInt(parentSessionID, 10), nil)
	failReq.Header.Set("Authorization", "Bearer "+token)
	failRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(failRec, failReq)
	if failRec.Code != http.StatusOK {
		t.Fatalf("expected failures status 200, got %d body=%s", failRec.Code, failRec.Body.String())
	}

	resumeBody := []byte(`{"child_session_id":` + strconv.FormatInt(childSessionID, 10) + `}`)
	resumeReq := httptest.NewRequest(http.MethodPost, "/v1/multiagent/resume", bytes.NewReader(resumeBody))
	resumeReq.Header.Set("Authorization", "Bearer "+token)
	resumeRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(resumeRec, resumeReq)
	if resumeRec.Code != http.StatusOK {
		t.Fatalf("expected resume status 200, got %d body=%s", resumeRec.Code, resumeRec.Body.String())
	}
	var resumePayload map[string]any
	if err := json.Unmarshal(resumeRec.Body.Bytes(), &resumePayload); err != nil {
		t.Fatalf("decode resume response: %v", err)
	}
	resultPayload, ok := resumePayload["result"].(map[string]any)
	if !ok || resultPayload["child_session_id"] == nil {
		t.Fatalf("expected resumed result payload, got %#v", resumePayload)
	}
	if resumePayload["resume_basis"] == nil {
		t.Fatalf("expected resume basis payload, got %#v", resumePayload)
	}
}

func TestExtensionsEndpointReturnsDiscoveredExtensions(t *testing.T) {
	cfg := config.Default()
	root := t.TempDir()
	cfg.DataDir = filepath.Join(root, "data")
	cfg.Extensions.PluginsDir = filepath.Join(root, "plugins")
	cfg.Extensions.SkillsDirs = []string{filepath.Join(root, "skills")}
	if err := os.MkdirAll(filepath.Join(cfg.Extensions.PluginsDir, "echoer"), 0o755); err != nil {
		t.Fatalf("mkdir plugin: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(cfg.Extensions.SkillsDirs[0], "greeter"), 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.Extensions.PluginsDir, "echoer", "plugin.yaml"), []byte("name: echoer\nenabled: true\n"), 0o644); err != nil {
		t.Fatalf("write plugin manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.Extensions.SkillsDirs[0], "greeter", "SKILL.md"), []byte("---\nname: greeter\ndescription: Greeting skill\n---\n"), 0o644); err != nil {
		t.Fatalf("write skill markdown: %v", err)
	}
	application, err := app.New(cfg)
	if err != nil {
		t.Fatalf("init app: %v", err)
	}
	defer application.Close()
	if err := application.Auth.InitAdmin(context.Background(), "admin", "ChangeMe123!"); err != nil {
		t.Fatalf("init admin: %v", err)
	}
	token, err := application.Auth.Login(context.Background(), "admin", "ChangeMe123!")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	server := New(application)
	req := httptest.NewRequest(http.MethodGet, "/v1/extensions", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	plugins, ok := payload["plugins"].([]any)
	if !ok || len(plugins) != 1 {
		t.Fatalf("unexpected plugins payload: %#v", payload["plugins"])
	}
	skills, ok := payload["skills"].([]any)
	if !ok || len(skills) != 1 {
		t.Fatalf("unexpected skills payload: %#v", payload["skills"])
	}
}

func TestExtensionStateEndpointPersistsDisableAndRemovesTool(t *testing.T) {
	cfg := config.Default()
	root := t.TempDir()
	cfg.DataDir = filepath.Join(root, "data")
	cfg.Extensions.PluginsDir = filepath.Join(root, "plugins")
	if err := os.MkdirAll(filepath.Join(cfg.Extensions.PluginsDir, "echoer"), 0o755); err != nil {
		t.Fatalf("mkdir plugin: %v", err)
	}
	script := filepath.Join(root, "plugin-tool.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'plugin:%s' \"$1\"\n"), 0o755); err != nil {
		t.Fatalf("write plugin script: %v", err)
	}
	manifest := "name: echoer\nenabled: true\ntool:\n  name: plugin.echoer\n  command: " + script + "\n  args_template:\n    - \"{{message}}\"\n  input_keys:\n    - message\n"
	if err := os.WriteFile(filepath.Join(cfg.Extensions.PluginsDir, "echoer", "plugin.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write plugin manifest: %v", err)
	}
	application, err := app.New(cfg)
	if err != nil {
		t.Fatalf("init app: %v", err)
	}
	defer application.Close()
	if err := application.Auth.InitAdmin(context.Background(), "admin", "ChangeMe123!"); err != nil {
		t.Fatalf("init admin: %v", err)
	}
	token, err := application.Auth.Login(context.Background(), "admin", "ChangeMe123!")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if _, err := application.Tools.Execute(context.Background(), "plugin.echoer", map[string]any{"username": "admin", "message": "alpha"}); err != nil {
		t.Fatalf("expected plugin tool before disable: %v", err)
	}
	server := New(application)
	body := []byte(`{"kind":"plugin","name":"echoer","enabled":false}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/extensions/state", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := application.Tools.Execute(context.Background(), "plugin.echoer", map[string]any{"username": "admin", "message": "alpha"}); err == nil {
		t.Fatal("expected plugin tool to be removed after disable")
	}
	states, err := application.Store.ListExtensionStates(context.Background())
	if err != nil {
		t.Fatalf("list extension states: %v", err)
	}
	if len(states) != 1 || states[0].Kind != "plugin" || states[0].Name != "echoer" || states[0].Enabled {
		t.Fatalf("unexpected persisted states: %#v", states)
	}
}

func TestModelsEndpointReturnsProfiles(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = filepath.Join(t.TempDir(), "data")
	application, err := app.New(cfg)
	if err != nil {
		t.Fatalf("init app: %v", err)
	}
	defer application.Close()
	if err := application.Auth.InitAdmin(context.Background(), "admin", "ChangeMe123!"); err != nil {
		t.Fatalf("init admin: %v", err)
	}
	token, err := application.Auth.Login(context.Background(), "admin", "ChangeMe123!")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	server := New(application)
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	profiles, ok := payload["profiles"].(map[string]any)
	if !ok || len(profiles) == 0 {
		t.Fatalf("expected non-empty profiles: %#v", payload["profiles"])
	}
}

func TestModelSwitchEndpointChangesCurrentProfile(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = filepath.Join(t.TempDir(), "data")
	application, err := app.New(cfg)
	if err != nil {
		t.Fatalf("init app: %v", err)
	}
	defer application.Close()
	if err := application.Auth.InitAdmin(context.Background(), "admin", "ChangeMe123!"); err != nil {
		t.Fatalf("init admin: %v", err)
	}
	token, err := application.Auth.Login(context.Background(), "admin", "ChangeMe123!")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	server := New(application)
	body := []byte(`{"profile":"ollama-qwen3"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/models/switch", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if application.CurrentModelProfile() != "ollama-qwen3" {
		t.Fatalf("unexpected current profile: %s", application.CurrentModelProfile())
	}
	if application.CurrentLLM().Model != "qwen3:14b" {
		t.Fatalf("unexpected current model: %s", application.CurrentLLM().Model)
	}
}

func TestMemoryEndpointReadsAndWrites(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = filepath.Join(t.TempDir(), "data")
	application, err := app.New(cfg)
	if err != nil {
		t.Fatalf("init app: %v", err)
	}
	defer application.Close()
	if err := application.Auth.InitAdmin(context.Background(), "admin", "ChangeMe123!"); err != nil {
		t.Fatalf("init admin: %v", err)
	}
	token, err := application.Auth.Login(context.Background(), "admin", "ChangeMe123!")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	server := New(application)

	writeBody := []byte(`{"target":"memory","action":"add","content":"Project uses SQLite"}`)
	writeReq := httptest.NewRequest(http.MethodPost, "/v1/memory", bytes.NewReader(writeBody))
	writeReq.Header.Set("Authorization", "Bearer "+token)
	writeRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(writeRec, writeReq)
	if writeRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", writeRec.Code, writeRec.Body.String())
	}

	readReq := httptest.NewRequest(http.MethodGet, "/v1/memory", nil)
	readReq.Header.Set("Authorization", "Bearer "+token)
	readRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(readRec, readReq)
	if readRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", readRec.Code, readRec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(readRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	memories, ok := payload["memory_entries"].([]any)
	if !ok || len(memories) != 1 {
		t.Fatalf("unexpected memory entries: %#v", payload["memory_entries"])
	}
}

func TestContextEndpointReturnsBudget(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = filepath.Join(t.TempDir(), "data")
	cfg.Context.HistoryWindowMessages = 8
	cfg.Context.CompressionEnabled = true
	cfg.Context.CompressThresholdMessages = 4
	cfg.Context.ProtectLastMessages = 2
	application, err := app.New(cfg)
	if err != nil {
		t.Fatalf("init app: %v", err)
	}
	defer application.Close()
	if err := application.Auth.InitAdmin(context.Background(), "admin", "ChangeMe123!"); err != nil {
		t.Fatalf("init admin: %v", err)
	}
	token, err := application.Auth.Login(context.Background(), "admin", "ChangeMe123!")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	sessionID, err := application.Store.CreateSession(context.Background(), "admin", "test-model", "p", "r")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	_ = application.Store.AddMessage(context.Background(), sessionID, "user", "hello history")
	_ = application.Store.AddMessage(context.Background(), sessionID, "assistant", "world history")
	_ = application.Store.AddMessage(context.Background(), sessionID, "user", "third history")
	_ = application.Store.AddMessage(context.Background(), sessionID, "assistant", "fourth history")
	_ = application.Store.AddMessage(context.Background(), sessionID, "user", "fifth history")
	server := New(application)
	req := httptest.NewRequest(http.MethodGet, "/v1/context?prompt=history+hello", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["history_messages_used"] == nil {
		t.Fatalf("expected history_messages_used in payload: %#v", payload)
	}
	if payload["compressed"] != true {
		t.Fatalf("expected compressed=true in payload: %#v", payload)
	}
	if payload["compressed_messages"] == nil {
		t.Fatalf("expected compressed_messages in payload: %#v", payload)
	}
}

func TestModelSwitchEndpointSupportsAlias(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = filepath.Join(t.TempDir(), "data")
	application, err := app.New(cfg)
	if err != nil {
		t.Fatalf("init app: %v", err)
	}
	defer application.Close()
	if err := application.Auth.InitAdmin(context.Background(), "admin", "ChangeMe123!"); err != nil {
		t.Fatalf("init admin: %v", err)
	}
	token, err := application.Auth.Login(context.Background(), "admin", "ChangeMe123!")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	server := New(application)
	body := []byte(`{"profile":"sonnet"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/models/switch", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if application.CurrentModelProfile() != "openrouter-claude-sonnet" {
		t.Fatalf("unexpected current profile: %s", application.CurrentModelProfile())
	}
}

func TestModelSwitchEndpointSupportsDirectModel(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = filepath.Join(t.TempDir(), "data")
	application, err := app.New(cfg)
	if err != nil {
		t.Fatalf("init app: %v", err)
	}
	defer application.Close()
	if err := application.Auth.InitAdmin(context.Background(), "admin", "ChangeMe123!"); err != nil {
		t.Fatalf("init admin: %v", err)
	}
	token, err := application.Auth.Login(context.Background(), "admin", "ChangeMe123!")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	server := New(application)
	body := []byte(`{"profile":"ollama-deepseek","model":"deepseek-r1:8b","base_url":"http://127.0.0.1:11434/v1","provider":"openai-compatible","local":true}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/models/switch", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if application.CurrentModelProfile() != "ollama-deepseek" {
		t.Fatalf("unexpected current profile: %s", application.CurrentModelProfile())
	}
	if application.CurrentLLM().Model != "deepseek-r1:8b" {
		t.Fatalf("unexpected current model: %s", application.CurrentLLM().Model)
	}
}
