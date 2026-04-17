package trajectory

import (
	"context"
	"path/filepath"
	"testing"

	"go-hermes-agent/internal/store"
)

func TestBuildSaveListAndGet(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	sessionID, err := st.CreateSession(context.Background(), "alice", "test-model", "hello", "world")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := st.AddMessage(context.Background(), sessionID, "user", "hello"); err != nil {
		t.Fatalf("add user message: %v", err)
	}
	if err := st.AddMessage(context.Background(), sessionID, "assistant", "world"); err != nil {
		t.Fatalf("add assistant message: %v", err)
	}

	manager := NewManager(t.TempDir())
	record, err := manager.BuildFromSession(context.Background(), st, "alice", sessionID)
	if err != nil {
		t.Fatalf("build trajectory: %v", err)
	}
	if _, err := manager.Save(record); err != nil {
		t.Fatalf("save trajectory: %v", err)
	}
	records, err := manager.List("alice", 10)
	if err != nil {
		t.Fatalf("list trajectories: %v", err)
	}
	if len(records) != 1 || records[0].SessionID != sessionID {
		t.Fatalf("unexpected records: %#v", records)
	}
	got, err := manager.Get("alice", records[0].ID)
	if err != nil {
		t.Fatalf("get trajectory: %v", err)
	}
	if len(got.Messages) != 2 || got.Messages[1].From != "assistant" {
		t.Fatalf("unexpected trajectory payload: %#v", got)
	}

	runName := "demo"
	got.Attributes = map[string]string{"run_name": runName}
	if _, err := manager.Save(got); err != nil {
		t.Fatalf("save filtered trajectory: %v", err)
	}
	completed := true
	filtered, err := manager.ListFiltered("alice", ListFilters{Limit: 10, RunName: runName, Completed: &completed})
	if err != nil {
		t.Fatalf("list filtered trajectories: %v", err)
	}
	if len(filtered) != 1 || filtered[0].Attributes["run_name"] != runName {
		t.Fatalf("unexpected filtered trajectories: %#v", filtered)
	}
	summary, err := manager.Summarize("alice", ListFilters{RunName: runName})
	if err != nil {
		t.Fatalf("summarize trajectories: %v", err)
	}
	if summary.Total != 1 || summary.ByRunName[runName] != 1 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
}
