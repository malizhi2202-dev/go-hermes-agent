package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionMessagesAndSearch(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	sessionID, err := st.CreateSession(context.Background(), "alice", "test-model", "hello", "hi")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := st.AddMessage(context.Background(), sessionID, "user", "hello world"); err != nil {
		t.Fatalf("add user message: %v", err)
	}
	if err := st.AddMessage(context.Background(), sessionID, "assistant", "hi alice"); err != nil {
		t.Fatalf("add assistant message: %v", err)
	}
	messages, err := st.GetMessages(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("get messages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	results, err := st.SearchMessages(context.Background(), SearchFilters{
		Username: "alice",
		Query:    "ali",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("search messages: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 search result, got %d", len(results))
	}
}

func TestSearchMessagesWithFilters(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	s1, _ := st.CreateSession(context.Background(), "alice", "test-model", "p1", "r1")
	now := time.Now().UTC()
	if err := st.AddMessage(context.Background(), s1, "user", "alpha user note"); err != nil {
		t.Fatalf("add message: %v", err)
	}
	s2, _ := st.CreateSession(context.Background(), "alice", "test-model", "p2", "r2")
	if err := st.AddMessage(context.Background(), s2, "assistant", "alpha assistant note"); err != nil {
		t.Fatalf("add message: %v", err)
	}

	results, err := st.SearchMessages(context.Background(), SearchFilters{
		Username:  "alice",
		Query:     "alpha",
		Role:      "assistant",
		SessionID: s2,
		FromTime:  now.Add(-time.Minute),
		ToTime:    time.Now().UTC().Add(time.Minute),
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("search messages: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 filtered result, got %d", len(results))
	}
	if results[0].Role != "assistant" || results[0].SessionID != s2 {
		t.Fatalf("unexpected filtered result: %#v", results[0])
	}
}

func TestMarkGatewayUpdateProcessedDedupes(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	first, err := st.MarkGatewayUpdateProcessed(context.Background(), "telegram", "42")
	if err != nil {
		t.Fatalf("mark processed: %v", err)
	}
	second, err := st.MarkGatewayUpdateProcessed(context.Background(), "telegram", "42")
	if err != nil {
		t.Fatalf("mark processed again: %v", err)
	}
	if !first || second {
		t.Fatalf("expected first=true second=false, got %v %v", first, second)
	}
}

func TestListAuditReturnsFilteredRecords(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	if err := st.WriteAudit(context.Background(), "alice", "system_exec_attempt", "command=echo"); err != nil {
		t.Fatalf("write audit: %v", err)
	}
	if err := st.WriteAudit(context.Background(), "alice", "chat", "session recorded"); err != nil {
		t.Fatalf("write audit: %v", err)
	}
	records, err := st.ListAudit(context.Background(), "alice", "system_exec_attempt", 10)
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 audit record, got %d", len(records))
	}
}

func TestListSessionsAndMessagesPagination(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	for i := 0; i < 3; i++ {
		sessionID, err := st.CreateSession(context.Background(), "alice", "model", "p", "r")
		if err != nil {
			t.Fatalf("create session: %v", err)
		}
		for j := 0; j < 3; j++ {
			if err := st.AddMessage(context.Background(), sessionID, "user", "msg"); err != nil {
				t.Fatalf("add message: %v", err)
			}
		}
	}
	sessions, err := st.ListSessionsPage(context.Background(), "alice", 2, 0)
	if err != nil {
		t.Fatalf("list sessions page: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	messages, err := st.GetMessagesPage(context.Background(), sessions[0].ID, 2, 1)
	if err != nil {
		t.Fatalf("get messages page: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 paged messages, got %d", len(messages))
	}
}

func TestListAuditFilteredSupportsTimeAndOffset(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	if err := st.WriteAudit(context.Background(), "alice", "system_exec_attempt", "one"); err != nil {
		t.Fatalf("write audit: %v", err)
	}
	if err := st.WriteAudit(context.Background(), "alice", "system_exec_attempt", "two"); err != nil {
		t.Fatalf("write audit: %v", err)
	}
	records, err := st.ListAuditFiltered(context.Background(), AuditFilters{
		Username: "alice",
		Action:   "system_exec_attempt",
		FromTime: time.Now().UTC().Add(-time.Hour),
		ToTime:   time.Now().UTC().Add(time.Hour),
		Limit:    1,
		Offset:   1,
	})
	if err != nil {
		t.Fatalf("list audit filtered: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 paged audit record, got %d", len(records))
	}
}

func TestExtensionStatesPersist(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	if err := st.UpsertExtensionState(context.Background(), "plugin", "echoer", false, "abc123"); err != nil {
		t.Fatalf("upsert extension state: %v", err)
	}
	states, err := st.ListExtensionStates(context.Background())
	if err != nil {
		t.Fatalf("list extension states: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("expected 1 extension state, got %d", len(states))
	}
	if states[0].Kind != "plugin" || states[0].Name != "echoer" || states[0].Enabled || states[0].Hash != "abc123" {
		t.Fatalf("unexpected extension state: %#v", states[0])
	}
}

func TestListRecentMessagesByUsername(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	sessionID, err := st.CreateSession(context.Background(), "alice", "model", "p", "r")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	_ = st.AddMessage(context.Background(), sessionID, "user", "first")
	_ = st.AddMessage(context.Background(), sessionID, "assistant", "second")
	_ = st.AddMessage(context.Background(), sessionID, "user", "third")
	messages, err := st.ListRecentMessagesByUsername(context.Background(), "alice", 2)
	if err != nil {
		t.Fatalf("list recent messages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 recent messages, got %d", len(messages))
	}
	if messages[0].Content != "second" || messages[1].Content != "third" {
		t.Fatalf("unexpected recent messages: %#v", messages)
	}
}

func TestContextSummaryPersists(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	if err := st.UpsertContextSummary(context.Background(), "alice", "summary text", "rule"); err != nil {
		t.Fatalf("upsert context summary: %v", err)
	}
	summary, err := st.GetContextSummary(context.Background(), "alice")
	if err != nil {
		t.Fatalf("get context summary: %v", err)
	}
	if summary.Username != "alice" || summary.Summary != "summary text" || summary.Strategy != "rule" {
		t.Fatalf("unexpected context summary: %#v", summary)
	}
}

func TestCreateSessionWithOptionsPersistsMetadata(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	sessionID, err := st.CreateSessionWithOptions(context.Background(), "alice", "model", "prompt", "response", CreateSessionOptions{
		Kind:            "multiagent_child",
		TaskID:          "task-1",
		ParentSessionID: 42,
	})
	if err != nil {
		t.Fatalf("create session with options: %v", err)
	}
	sessions, err := st.ListSessionsPage(context.Background(), "alice", 10, 0)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != sessionID {
		t.Fatalf("unexpected sessions: %#v", sessions)
	}
	if sessions[0].Kind != "multiagent_child" || sessions[0].TaskID != "task-1" || !sessions[0].ParentSessionID.Valid || sessions[0].ParentSessionID.Int64 != 42 {
		t.Fatalf("unexpected session metadata: %#v", sessions[0])
	}
}

func TestListRecentMessagesBySession(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	sessionID, err := st.CreateSession(context.Background(), "alice", "model", "p", "r")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	_ = st.AddMessage(context.Background(), sessionID, "user", "one")
	_ = st.AddMessage(context.Background(), sessionID, "assistant", "two")
	_ = st.AddMessage(context.Background(), sessionID, "user", "three")
	messages, err := st.ListRecentMessagesBySession(context.Background(), sessionID, 2)
	if err != nil {
		t.Fatalf("list recent messages by session: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[0].Content != "two" || messages[1].Content != "three" {
		t.Fatalf("unexpected messages: %#v", messages)
	}
}

func TestMultiAgentTracePersistsAndLists(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	if err := st.InsertMultiAgentTrace(context.Background(), MultiAgentTraceRecord{
		Username:        "alice",
		ParentSessionID: 1,
		ChildSessionID:  2,
		TaskID:          "gateway",
		Iteration:       1,
		Type:            "tool",
		Tool:            "session.search",
		InputJSON:       `{"query":"alpha"}`,
		OutputJSON:      `{"results":[]}`,
		Note:            "first tool call",
	}); err != nil {
		t.Fatalf("insert trace: %v", err)
	}
	records, err := st.ListMultiAgentTraces(context.Background(), MultiAgentTraceFilters{
		Username:       "alice",
		ChildSessionID: 2,
	})
	if err != nil {
		t.Fatalf("list traces: %v", err)
	}
	if len(records) != 1 || records[0].TaskID != "gateway" || records[0].Tool != "session.search" {
		t.Fatalf("unexpected trace records: %#v", records)
	}
}

func TestMultiAgentTraceSummariesAndFailures(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	ctx := context.Background()
	rows := []MultiAgentTraceRecord{
		{Username: "alice", ParentSessionID: 1, ChildSessionID: 2, TaskID: "t1", Iteration: 1, Type: "tool", Tool: "session.search", OutputJSON: `{"ok":true}`},
		{Username: "alice", ParentSessionID: 1, ChildSessionID: 2, TaskID: "t1", Iteration: 2, Type: "tool", Tool: "session.search", Error: "boom"},
		{Username: "alice", ParentSessionID: 1, ChildSessionID: 2, TaskID: "t1", Iteration: 3, Type: "final", Note: "done"},
	}
	for _, row := range rows {
		if err := st.InsertMultiAgentTrace(ctx, row); err != nil {
			t.Fatalf("insert trace: %v", err)
		}
	}
	failures, err := st.ListMultiAgentTraceFailures(ctx, MultiAgentTraceFilters{Username: "alice"})
	if err != nil {
		t.Fatalf("list failures: %v", err)
	}
	if len(failures) != 1 || failures[0].Tool != "session.search" {
		t.Fatalf("unexpected failures: %#v", failures)
	}
	summary, err := st.SummarizeMultiAgentTraces(ctx, MultiAgentTraceFilters{Username: "alice"})
	if err != nil {
		t.Fatalf("summarize traces: %v", err)
	}
	if len(summary) == 0 {
		t.Fatal("expected non-empty summary")
	}
	if summary[0].Tool != "session.search" || summary[0].Failures != 1 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	hotspots, err := st.ListMultiAgentTraceHotspots(ctx, MultiAgentTraceFilters{Username: "alice", Limit: 10})
	if err != nil {
		t.Fatalf("trace hotspots: %v", err)
	}
	if len(hotspots) != 1 || hotspots[0].ChildSessionID != 2 || hotspots[0].Failures != 1 {
		t.Fatalf("unexpected hotspots: %#v", hotspots)
	}
}
