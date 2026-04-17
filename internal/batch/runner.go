package batch

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go-hermes-agent/internal/app"
	"go-hermes-agent/internal/trajectory"
)

// Item is one dataset prompt.
type Item struct {
	Prompt string         `json:"prompt"`
	Meta   map[string]any `json:"meta,omitempty"`
}

// Result is one processed batch item result.
type Result struct {
	Index        int       `json:"index"`
	Prompt       string    `json:"prompt"`
	Success      bool      `json:"success"`
	Response     string    `json:"response,omitempty"`
	SessionID    int64     `json:"session_id,omitempty"`
	TrajectoryID string    `json:"trajectory_id,omitempty"`
	Error        string    `json:"error,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
}

// Summary aggregates one batch run.
type Summary struct {
	RunName       string   `json:"run_name"`
	Username      string   `json:"username"`
	Total         int      `json:"total"`
	Succeeded     int      `json:"succeeded"`
	Failed        int      `json:"failed"`
	Trajectories  int      `json:"trajectories"`
	TrajectoryDir string   `json:"trajectory_dir"`
	Errors        []string `json:"errors,omitempty"`
}

// Checkpoint stores resumable batch progress for one run.
type Checkpoint struct {
	RunName        string         `json:"run_name"`
	Username       string         `json:"username"`
	DatasetFile    string         `json:"dataset_file,omitempty"`
	Completed      map[int]Result `json:"completed"`
	UpdatedAt      time.Time      `json:"updated_at"`
	TotalRequested int            `json:"total_requested"`
}

// RunOptions controls checkpoint and resume behavior.
type RunOptions struct {
	DatasetFile string
	Resume      bool
}

// Runner executes sequential batch runs on top of App chat.
type Runner struct {
	app        *app.App
	trajectory *trajectory.Manager
	stateDir   string
}

// NewRunner creates a lightweight batch runner.
func NewRunner(application *app.App, manager *trajectory.Manager) *Runner {
	return &Runner{
		app:        application,
		trajectory: manager,
		stateDir:   filepath.Join(application.Config.DataDir, "batch"),
	}
}

// LoadJSONL loads prompt items from one JSONL dataset file.
func LoadJSONL(path string) ([]Item, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	items := make([]Item, 0)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var item Item
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			return nil, err
		}
		if strings.TrimSpace(item.Prompt) == "" {
			continue
		}
		items = append(items, item)
	}
	return items, scanner.Err()
}

// Run executes one sequential batch and saves trajectories for successful items.
func (r *Runner) Run(ctx context.Context, username, runName string, items []Item) ([]Result, Summary, error) {
	return r.RunWithOptions(ctx, username, runName, items, RunOptions{})
}

// RunWithOptions executes one sequential batch with optional checkpoint resume.
func (r *Runner) RunWithOptions(ctx context.Context, username, runName string, items []Item, opts RunOptions) ([]Result, Summary, error) {
	results := make([]Result, 0, len(items))
	summary := Summary{
		RunName:       runName,
		Username:      username,
		Total:         len(items),
		TrajectoryDir: r.trajectory.Dir(),
	}
	checkpoint := Checkpoint{
		RunName:        runName,
		Username:       username,
		DatasetFile:    opts.DatasetFile,
		Completed:      make(map[int]Result),
		UpdatedAt:      time.Now().UTC(),
		TotalRequested: len(items),
	}
	if opts.Resume {
		if loaded, err := r.LoadCheckpoint(username, runName); err == nil {
			checkpoint = loaded
			summary.Total = len(items)
			for _, result := range checkpoint.Completed {
				results = append(results, result)
				if result.Success {
					summary.Succeeded++
					if result.TrajectoryID != "" {
						summary.Trajectories++
					}
				} else {
					summary.Failed++
					if result.Error != "" {
						summary.Errors = append(summary.Errors, fmt.Sprintf("index=%d err=%s", result.Index, result.Error))
					}
				}
			}
			sort.Slice(results, func(i, j int) bool { return results[i].Index < results[j].Index })
		}
	}
	for index, item := range items {
		if previous, ok := checkpoint.Completed[index]; ok {
			_ = previous
			continue
		}
		result := Result{
			Index:     index,
			Prompt:    item.Prompt,
			Timestamp: time.Now().UTC(),
		}
		chatResult, err := r.app.ChatDetailed(ctx, username, item.Prompt)
		if err != nil {
			result.Error = err.Error()
			summary.Failed++
			summary.Errors = append(summary.Errors, fmt.Sprintf("index=%d err=%s", index, err.Error()))
			results = append(results, result)
			checkpoint.Completed[index] = result
			checkpoint.UpdatedAt = time.Now().UTC()
			_ = r.SaveCheckpoint(checkpoint)
			continue
		}
		record, err := r.trajectory.BuildFromSession(ctx, r.app.Store, username, chatResult.SessionID)
		if err == nil {
			record.Attributes = map[string]string{
				"run_name": runName,
				"index":    fmt.Sprintf("%d", index),
			}
			if item.Meta != nil {
				record.Metadata["dataset_meta"] = item.Meta
			}
			if _, saveErr := r.trajectory.Save(record); saveErr == nil {
				result.TrajectoryID = record.ID
				summary.Trajectories++
			}
		}
		result.Success = true
		result.Response = chatResult.Response
		result.SessionID = chatResult.SessionID
		summary.Succeeded++
		results = append(results, result)
		checkpoint.Completed[index] = result
		checkpoint.UpdatedAt = time.Now().UTC()
		_ = r.SaveCheckpoint(checkpoint)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Index < results[j].Index })
	return results, summary, nil
}

// LoadCheckpoint loads one saved batch checkpoint when present.
func (r *Runner) LoadCheckpoint(username, runName string) (Checkpoint, error) {
	path := filepath.Join(r.stateDir, sanitizeRunName(username+"-"+runName)+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return Checkpoint{}, err
	}
	var checkpoint Checkpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return Checkpoint{}, err
	}
	if checkpoint.Completed == nil {
		checkpoint.Completed = make(map[int]Result)
	}
	return checkpoint, nil
}

// SaveCheckpoint writes one batch checkpoint file.
func (r *Runner) SaveCheckpoint(checkpoint Checkpoint) error {
	if err := os.MkdirAll(r.stateDir, 0o755); err != nil {
		return err
	}
	checkpoint.UpdatedAt = checkpoint.UpdatedAt.UTC()
	if checkpoint.UpdatedAt.IsZero() {
		checkpoint.UpdatedAt = time.Now().UTC()
	}
	raw, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(r.stateDir, sanitizeRunName(checkpoint.Username+"-"+checkpoint.RunName)+".json")
	return os.WriteFile(path, raw, 0o644)
}

func sanitizeRunName(input string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", " ", "_", ":", "_")
	return replacer.Replace(strings.TrimSpace(input))
}
