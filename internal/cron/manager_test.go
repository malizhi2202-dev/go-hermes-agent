package cron

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"go-hermes-agent/internal/app"
)

func TestParseScheduleSupportsDurationIntervalAndTimestamp(t *testing.T) {
	now := time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)

	once, err := ParseSchedule("30m", now)
	if err != nil {
		t.Fatalf("parse once duration: %v", err)
	}
	if once.Kind != ScheduleKindOnce || once.RunAt == "" {
		t.Fatalf("unexpected once schedule: %#v", once)
	}

	interval, err := ParseSchedule("every 2h", now)
	if err != nil {
		t.Fatalf("parse interval: %v", err)
	}
	if interval.Kind != ScheduleKindInterval || interval.Interval != 120 {
		t.Fatalf("unexpected interval schedule: %#v", interval)
	}

	timestamp, err := ParseSchedule("2026-04-18T08:00:00Z", now)
	if err != nil {
		t.Fatalf("parse timestamp: %v", err)
	}
	if timestamp.Kind != ScheduleKindOnce || timestamp.RunAt != "2026-04-18T08:00:00Z" {
		t.Fatalf("unexpected timestamp schedule: %#v", timestamp)
	}
}

func TestManagerAddListTickAndDelete(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	manager := NewManager(dataDir)
	job, err := manager.AddJob(CreateInput{
		Name:     "demo",
		Username: "alice",
		Prompt:   "hello from cron",
		Schedule: "2026-04-17T10:00:00Z",
	})
	if err != nil {
		t.Fatalf("add job: %v", err)
	}
	jobs, err := manager.ListJobs()
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != job.ID {
		t.Fatalf("unexpected jobs: %#v", jobs)
	}

	jobs[0].NextRunAt = time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	if err := manager.saveJobs(jobs); err != nil {
		t.Fatalf("save jobs: %v", err)
	}

	results, err := manager.TickWith(
		context.Background(),
		func(_ context.Context, username, prompt string) (app.ChatResult, error) {
			return app.ChatResult{
				SessionID: 42,
				Model:     "test-model",
				Prompt:    prompt,
				Response:  fmt.Sprintf("reply:%s:%s", username, prompt),
			}, nil
		},
		nil,
	)
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(results) != 1 || !results[0].Success || results[0].OutputFile == "" {
		t.Fatalf("unexpected tick results: %#v", results)
	}

	loaded, err := manager.GetJob(job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if loaded.Enabled {
		t.Fatalf("expected one-shot job to be disabled after tick: %#v", loaded)
	}
	if loaded.LastSessionID == 0 || loaded.LastOutputFile == "" {
		t.Fatalf("expected persisted run state, got %#v", loaded)
	}

	if err := manager.DeleteJob(job.ID); err != nil {
		t.Fatalf("delete job: %v", err)
	}
	remaining, err := manager.ListJobs()
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected empty jobs after delete, got %#v", remaining)
	}
}
