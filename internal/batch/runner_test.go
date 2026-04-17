package batch

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	apppkg "go-hermes-agent/internal/app"
	"go-hermes-agent/internal/config"
	"go-hermes-agent/internal/trajectory"
)

func TestLoadJSONLAndRun(t *testing.T) {
	dataset := filepath.Join(t.TempDir(), "dataset.jsonl")
	if err := os.WriteFile(dataset, []byte("{\"prompt\":\"hello\"}\n{\"prompt\":\"world\"}\n"), 0o644); err != nil {
		t.Fatalf("write dataset: %v", err)
	}
	items, err := LoadJSONL(dataset)
	if err != nil {
		t.Fatalf("load jsonl: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("unexpected item count: %#v", items)
	}

	cfg := config.Default()
	cfg.DataDir = filepath.Join(t.TempDir(), "data")
	cfg.LLM.BaseURL = "http://127.0.0.1:1"
	cfg.LLM.APIKey = ""
	application, err := apppkg.New(cfg)
	if err != nil {
		t.Fatalf("init app: %v", err)
	}
	defer application.Close()

	runner := NewRunner(application, trajectory.NewManager(cfg.DataDir))
	results, summary, err := runner.RunWithOptions(context.Background(), "admin", "test-run", items, RunOptions{DatasetFile: dataset})
	if err != nil {
		t.Fatalf("run batch: %v", err)
	}
	if len(results) != 2 || summary.Total != 2 {
		t.Fatalf("unexpected batch output: %#v %#v", results, summary)
	}
	checkpoint, err := runner.LoadCheckpoint("admin", "test-run")
	if err != nil {
		t.Fatalf("load checkpoint: %v", err)
	}
	if len(checkpoint.Completed) != 2 {
		t.Fatalf("unexpected checkpoint: %#v", checkpoint)
	}

	resumedResults, resumedSummary, err := runner.RunWithOptions(context.Background(), "admin", "test-run", items, RunOptions{DatasetFile: dataset, Resume: true})
	if err != nil {
		t.Fatalf("resume batch: %v", err)
	}
	if len(resumedResults) != 2 || resumedSummary.Total != 2 {
		t.Fatalf("unexpected resumed output: %#v %#v", resumedResults, resumedSummary)
	}
}
