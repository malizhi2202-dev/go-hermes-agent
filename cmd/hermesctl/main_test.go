package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go-hermes-agent/internal/multiagent"
	"go-hermes-agent/internal/store"
)

func TestParseCommaList(t *testing.T) {
	got := parseCommaList(" session.search, memory.read ,, system.exec_profile ")
	want := []string{"session.search", "memory.read", "system.exec_profile"}
	if len(got) != len(want) {
		t.Fatalf("unexpected slice length: got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected slice: got=%v want=%v", got, want)
		}
	}
}

func TestParseOptionalTime(t *testing.T) {
	parsed, err := parseOptionalTime("from", "2026-04-17T08:00:00Z")
	if err != nil {
		t.Fatalf("parse time: %v", err)
	}
	if parsed.Format(time.RFC3339) != "2026-04-17T08:00:00Z" {
		t.Fatalf("unexpected parsed time: %s", parsed.Format(time.RFC3339))
	}
	if _, err := parseOptionalTime("from", "bad-time"); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestParseOptionalBoolPointer(t *testing.T) {
	if got := parseOptionalBoolPointer(""); got != nil {
		t.Fatalf("expected nil pointer, got %#v", got)
	}
	got := parseOptionalBoolPointer("true")
	if got == nil || !*got {
		t.Fatalf("expected true pointer, got %#v", got)
	}
	got = parseOptionalBoolPointer("false")
	if got == nil || *got {
		t.Fatalf("expected false pointer, got %#v", got)
	}
}

func TestSplitCommandAndRest(t *testing.T) {
	command, rest := splitCommandAndRest("search gateway status")
	if command != "search" || rest != "gateway status" {
		t.Fatalf("unexpected split result: command=%q rest=%q", command, rest)
	}
	command, rest = splitCommandAndRest("help")
	if command != "help" || rest != "" {
		t.Fatalf("unexpected single token split: command=%q rest=%q", command, rest)
	}
}

func TestSplitAroundDoubleDash(t *testing.T) {
	left, right, ok := splitAroundDoubleDash("tasks.json -- inspect gateway")
	if !ok || left != "tasks.json" || right != "inspect gateway" {
		t.Fatalf("unexpected split around double dash: ok=%v left=%q right=%q", ok, left, right)
	}
	if _, _, ok := splitAroundDoubleDash("tasks.json inspect gateway"); ok {
		t.Fatal("expected missing separator to fail")
	}
}

func TestParsePositiveIntDefault(t *testing.T) {
	if got := parsePositiveIntDefault("12", 5); got != 12 {
		t.Fatalf("unexpected parsed int: %d", got)
	}
	if got := parsePositiveIntDefault("bad", 5); got != 5 {
		t.Fatalf("unexpected fallback for bad input: %d", got)
	}
	if got := parsePositiveIntDefault("", 5); got != 5 {
		t.Fatalf("unexpected fallback for empty input: %d", got)
	}
}

func TestLoadTasksFileSupportsArrayAndWrapper(t *testing.T) {
	dir := t.TempDir()
	arrayFile := filepath.Join(dir, "tasks-array.json")
	wrapperFile := filepath.Join(dir, "tasks-wrapper.json")
	if err := osWriteFile(arrayFile, `[ {"id":"a","goal":"inspect"} ]`); err != nil {
		t.Fatalf("write array file: %v", err)
	}
	if err := osWriteFile(wrapperFile, `{ "tasks": [ {"id":"b","goal":"repair"} ] }`); err != nil {
		t.Fatalf("write wrapper file: %v", err)
	}
	arrayTasks, err := loadTasksFile(arrayFile)
	if err != nil {
		t.Fatalf("load array tasks: %v", err)
	}
	wrapperTasks, err := loadTasksFile(wrapperFile)
	if err != nil {
		t.Fatalf("load wrapper tasks: %v", err)
	}
	if len(arrayTasks) != 1 || arrayTasks[0].ID != "a" {
		t.Fatalf("unexpected array tasks: %#v", arrayTasks)
	}
	if len(wrapperTasks) != 1 || wrapperTasks[0].ID != "b" {
		t.Fatalf("unexpected wrapper tasks: %#v", wrapperTasks)
	}
}

func TestLoadPlanFile(t *testing.T) {
	dir := t.TempDir()
	planFile := filepath.Join(dir, "plan.json")
	if err := osWriteFile(planFile, `{ "objective": "ship", "mode": "parallel", "max_concurrent": 2, "tasks": [ {"id":"a","goal":"inspect"} ] }`); err != nil {
		t.Fatalf("write plan file: %v", err)
	}
	plan, err := loadPlanFile(planFile)
	if err != nil {
		t.Fatalf("load plan file: %v", err)
	}
	if plan.Objective != "ship" || plan.Mode != multiagent.TaskModeParallel || len(plan.Tasks) != 1 {
		t.Fatalf("unexpected plan: %#v", plan)
	}
}

func TestBuildHistoryPayload(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	sessionID, err := st.CreateSession(ctx, "alice", "test-model", "hello", "world")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := st.AddMessage(ctx, sessionID, "user", "alpha"); err != nil {
		t.Fatalf("add message: %v", err)
	}
	if err := st.AddMessage(ctx, sessionID, "assistant", "beta"); err != nil {
		t.Fatalf("add message: %v", err)
	}
	payload, err := buildHistoryPayload(ctx, st, "alice", 10, 0, 10, 0)
	if err != nil {
		t.Fatalf("build history payload: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("unexpected history payload length: %#v", payload)
	}
	messages, ok := payload[0]["messages"].([]store.Message)
	if !ok || len(messages) != 2 {
		t.Fatalf("unexpected history messages payload: %#v", payload[0]["messages"])
	}
}

func TestBuildExecutionProfileAuditPayload(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.WriteAudit(ctx, "alice", "system_exec_profile_attempt", "profile=deploy approved=true"); err != nil {
		t.Fatalf("write audit attempt: %v", err)
	}
	if err := st.WriteAudit(ctx, "alice", "system_exec_profile_success", "profile=deploy approved=true"); err != nil {
		t.Fatalf("write audit success: %v", err)
	}
	if err := st.WriteAudit(ctx, "alice", "system_exec_profile_denied", "profile=deploy approved=false"); err != nil {
		t.Fatalf("write audit denied: %v", err)
	}
	payload, err := buildExecutionProfileAuditPayload(ctx, st, "alice", time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("build audit payload: %v", err)
	}
	records, ok := payload["records"].([]store.AuditRecord)
	if !ok || len(records) != 3 {
		t.Fatalf("unexpected records payload: %#v", payload["records"])
	}
	profileSummary, ok := payload["profile_summary"].([]executionProfileSummary)
	if !ok || len(profileSummary) != 1 {
		t.Fatalf("unexpected profile summary: %#v", payload["profile_summary"])
	}
	if profileSummary[0].Profile != "deploy" || profileSummary[0].Success != 1 || profileSummary[0].Denied != 1 {
		t.Fatalf("unexpected profile summary entry: %#v", profileSummary[0])
	}
}

func TestWritePrettyJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := writePrettyJSON(&buf, map[string]any{"ok": true}); err != nil {
		t.Fatalf("write json: %v", err)
	}
	if !strings.Contains(buf.String(), "\n") || !strings.Contains(buf.String(), "\"ok\": true") {
		t.Fatalf("unexpected json output: %q", buf.String())
	}
}

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		_ = st.Close()
	})
	return st
}

func osWriteFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
