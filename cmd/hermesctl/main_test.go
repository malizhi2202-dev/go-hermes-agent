package main

import (
	"bufio"
	"bytes"
	"context"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go-hermes-agent/internal/config"
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

func TestParseConsoleFlagArgs(t *testing.T) {
	fs := flag.NewFlagSet("console", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	limit := fs.Int("limit", 50, "record limit")
	offset := fs.Int("offset", 0, "record offset")
	if err := parseConsoleFlagArgs(fs, "--limit 12 --offset 3"); err != nil {
		t.Fatalf("parse console flags: %v", err)
	}
	if *limit != 12 || *offset != 3 {
		t.Fatalf("unexpected parsed values: limit=%d offset=%d", *limit, *offset)
	}
}

func TestParseConsoleTraceFilters(t *testing.T) {
	filters, err := parseConsoleTraceFilters("--parent-session-id 11 --child-session-id 12 --task-id gateway --from 2026-04-17T08:00:00Z --to 2026-04-17T10:00:00Z --limit 7 --offset 3", 50)
	if err != nil {
		t.Fatalf("parse trace filters: %v", err)
	}
	if filters.ParentSessionID != 11 || filters.ChildSessionID != 12 || filters.TaskID != "gateway" || filters.Limit != 7 || filters.Offset != 3 {
		t.Fatalf("unexpected trace filters: %#v", filters)
	}
	if filters.FromTime.Format(time.RFC3339) != "2026-04-17T08:00:00Z" || filters.ToTime.Format(time.RFC3339) != "2026-04-17T10:00:00Z" {
		t.Fatalf("unexpected trace filter times: %#v", filters)
	}
}

func TestParseToolExecInput(t *testing.T) {
	name, input, err := parseToolExecInput(`system.exec -- {"command":"echo","args":["hello"]}`)
	if err != nil {
		t.Fatalf("parse tool exec input: %v", err)
	}
	if name != "system.exec" {
		t.Fatalf("unexpected tool name: %q", name)
	}
	if input["command"] != "echo" {
		t.Fatalf("unexpected command input: %#v", input)
	}
	args, ok := input["args"].([]any)
	if !ok || len(args) != 1 || args[0] != "hello" {
		t.Fatalf("unexpected args input: %#v", input["args"])
	}
}

func TestInteractiveConsoleHelpIncludesMigratedCommands(t *testing.T) {
	var out bytes.Buffer
	console := newInteractiveConsole("", config.Config{}, nil, bufio.NewReader(strings.NewReader("")), &out, "alice")
	console.printHelp()
	help := out.String()
	wantSnippets := []string{
		"/new",
		"/clear",
		"/status",
		"/usage",
		"/insights [days]",
		"/prompt-inspect <prompt>",
		"/prompt-cache-stats",
		"/prompt-cache-clear",
		"/prompt-config",
		"/resume [session-id]",
		"/discover-models",
		"/model-metadata [profile-or-alias]",
		"/auxiliary-info [task]",
		"/auxiliary-chat [task] -- <prompt>",
		"/auxiliary-switch <profile> [default|summary|compression]",
		"/extension-hooks [--kind <kind>] [--name <name>] [--phase <phase>] [--limit n] [--offset n]",
		"/extension-refresh",
		"/extension-state --kind <kind> --name <name> --enabled <true|false>",
		"/extension-validate --kind <kind> --name <name>",
		"/retry",
		"/undo",
		"/tool-exec <tool-name> [-- <json-object>]",
		"/execution-audit [--limit n] [--offset n]",
		"/execution-profile-audit [--from <rfc3339>] [--to <rfc3339>]",
		"/multiagent-traces [--parent-session-id n] [--child-session-id n] [--task-id <id>] [--from <rfc3339>] [--to <rfc3339>] [--limit n] [--offset n]",
		"/multiagent-summary [--parent-session-id n] [--child-session-id n] [--task-id <id>] [--from <rfc3339>] [--to <rfc3339>]",
		"/multiagent-verifiers [--parent-session-id n] [--child-session-id n] [--task-id <id>] [--from <rfc3339>] [--to <rfc3339>]",
		"/multiagent-failures [--parent-session-id n] [--child-session-id n] [--task-id <id>] [--from <rfc3339>] [--to <rfc3339>] [--limit n] [--offset n]",
		"/multiagent-hotspots [--parent-session-id n] [--child-session-id n] [--task-id <id>] [--from <rfc3339>] [--to <rfc3339>] [--limit n] [--offset n]",
		"/multiagent-resume <child-session-id> [--allowed-tools a,b] [--history-window n]",
		"/trajectory-show <trajectory-id>",
	}
	for _, snippet := range wantSnippets {
		if !strings.Contains(help, snippet) {
			t.Fatalf("expected help to contain %q, got:\n%s", snippet, help)
		}
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
