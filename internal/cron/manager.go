package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go-hermes-agent/internal/app"
)

// ScheduleKind identifies the supported lightweight schedule modes.
type ScheduleKind string

const (
	// ScheduleKindOnce runs one time and disables itself after one attempt.
	ScheduleKindOnce ScheduleKind = "once"
	// ScheduleKindInterval repeats after a fixed duration.
	ScheduleKindInterval ScheduleKind = "interval"
)

// Schedule stores the normalized schedule contract for one cron job.
type Schedule struct {
	Raw      string       `json:"raw"`
	Kind     ScheduleKind `json:"kind"`
	RunAt    string       `json:"run_at,omitempty"`
	Interval int          `json:"interval_minutes,omitempty"`
	Display  string       `json:"display"`
}

// Job is one persisted scheduler entry.
type Job struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Username       string    `json:"username"`
	Prompt         string    `json:"prompt"`
	Schedule       Schedule  `json:"schedule"`
	Enabled        bool      `json:"enabled"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	NextRunAt      string    `json:"next_run_at,omitempty"`
	LastRunAt      string    `json:"last_run_at,omitempty"`
	LastSessionID  int64     `json:"last_session_id,omitempty"`
	LastOutputFile string    `json:"last_output_file,omitempty"`
	LastError      string    `json:"last_error,omitempty"`
	RunCount       int       `json:"run_count"`
}

// CreateInput defines the user-supplied fields needed to create one job.
type CreateInput struct {
	Name     string
	Username string
	Prompt   string
	Schedule string
}

// TickResult is one execution outcome produced by a scheduler tick.
type TickResult struct {
	JobID      string    `json:"job_id"`
	Name       string    `json:"name"`
	Username   string    `json:"username"`
	Triggered  bool      `json:"triggered"`
	Success    bool      `json:"success"`
	SessionID  int64     `json:"session_id,omitempty"`
	OutputFile string    `json:"output_file,omitempty"`
	Error      string    `json:"error,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
}

// ChatRunner abstracts one scheduler-triggered chat turn.
type ChatRunner func(ctx context.Context, username, prompt string) (app.ChatResult, error)

// AuditWriter abstracts scheduler audit logging.
type AuditWriter func(ctx context.Context, username, action, detail string) error

// Manager persists lightweight cron jobs and executes due jobs.
type Manager struct {
	dir       string
	jobsFile  string
	outputDir string
	mu        sync.Mutex
}

// NewManager creates a cron manager rooted in one data directory.
func NewManager(dataDir string) *Manager {
	dir := filepath.Join(dataDir, "cron")
	return &Manager{
		dir:       dir,
		jobsFile:  filepath.Join(dir, "jobs.json"),
		outputDir: filepath.Join(dir, "output"),
	}
}

// ParseSchedule normalizes one lightweight schedule expression.
func ParseSchedule(raw string, now time.Time) (Schedule, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Schedule{}, fmt.Errorf("schedule is required")
	}
	now = now.UTC()
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "every ") {
		minutes, err := parseDurationMinutes(strings.TrimSpace(raw[6:]))
		if err != nil {
			return Schedule{}, err
		}
		return Schedule{
			Raw:      raw,
			Kind:     ScheduleKindInterval,
			Interval: minutes,
			Display:  fmt.Sprintf("every %dm", minutes),
		}, nil
	}
	if runAt, err := time.Parse(time.RFC3339, raw); err == nil {
		runAt = runAt.UTC()
		return Schedule{
			Raw:     raw,
			Kind:    ScheduleKindOnce,
			RunAt:   runAt.Format(time.RFC3339),
			Display: "once at " + runAt.Format(time.RFC3339),
		}, nil
	}
	minutes, err := parseDurationMinutes(raw)
	if err != nil {
		return Schedule{}, fmt.Errorf("invalid schedule %q: use RFC3339, 30m, 2h, 1d, or every <duration>", raw)
	}
	runAt := now.Add(time.Duration(minutes) * time.Minute)
	return Schedule{
		Raw:     raw,
		Kind:    ScheduleKindOnce,
		RunAt:   runAt.Format(time.RFC3339),
		Display: "once in " + raw,
	}, nil
}

// AddJob validates and persists one cron job.
func (m *Manager) AddJob(input CreateInput) (Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	schedule, err := ParseSchedule(input.Schedule, now)
	if err != nil {
		return Job{}, err
	}
	if err := m.ensureDirs(); err != nil {
		return Job{}, err
	}
	jobs, err := m.loadJobs()
	if err != nil {
		return Job{}, err
	}
	job := Job{
		ID:        fmt.Sprintf("cron-%d", now.UnixNano()),
		Name:      strings.TrimSpace(input.Name),
		Username:  strings.TrimSpace(input.Username),
		Prompt:    strings.TrimSpace(input.Prompt),
		Schedule:  schedule,
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if job.Name == "" {
		return Job{}, fmt.Errorf("name is required")
	}
	if job.Username == "" {
		return Job{}, fmt.Errorf("username is required")
	}
	if job.Prompt == "" {
		return Job{}, fmt.Errorf("prompt is required")
	}
	job.NextRunAt = firstNextRun(schedule, now)
	jobs = append(jobs, job)
	if err := m.saveJobs(jobs); err != nil {
		return Job{}, err
	}
	return job, nil
}

// ListJobs returns all jobs sorted by next run time and creation time.
func (m *Manager) ListJobs() ([]Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensureDirs(); err != nil {
		return nil, err
	}
	jobs, err := m.loadJobs()
	if err != nil {
		return nil, err
	}
	sort.Slice(jobs, func(i, j int) bool {
		left := firstNonEmpty(jobs[i].NextRunAt, jobs[i].CreatedAt.Format(time.RFC3339))
		right := firstNonEmpty(jobs[j].NextRunAt, jobs[j].CreatedAt.Format(time.RFC3339))
		if left == right {
			return jobs[i].ID < jobs[j].ID
		}
		return left < right
	})
	return jobs, nil
}

// GetJob returns one job by ID.
func (m *Manager) GetJob(id string) (Job, error) {
	jobs, err := m.ListJobs()
	if err != nil {
		return Job{}, err
	}
	for _, job := range jobs {
		if job.ID == id {
			return job, nil
		}
	}
	return Job{}, fmt.Errorf("cron job %q not found", id)
}

// DeleteJob removes one job by ID.
func (m *Manager) DeleteJob(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	jobs, err := m.loadJobs()
	if err != nil {
		return err
	}
	filtered := make([]Job, 0, len(jobs))
	found := false
	for _, job := range jobs {
		if job.ID == id {
			found = true
			continue
		}
		filtered = append(filtered, job)
	}
	if !found {
		return fmt.Errorf("cron job %q not found", id)
	}
	return m.saveJobs(filtered)
}

// Tick executes every due job once and persists the updated schedule state.
func (m *Manager) Tick(ctx context.Context, application *app.App) ([]TickResult, error) {
	return m.TickWith(ctx, application.ChatDetailed, application.Store.WriteAudit)
}

// TickWith executes every due job once and persists the updated schedule state.
func (m *Manager) TickWith(ctx context.Context, runner ChatRunner, audit AuditWriter) ([]TickResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	if err := m.ensureDirs(); err != nil {
		return nil, err
	}
	jobs, err := m.loadJobs()
	if err != nil {
		return nil, err
	}
	results := make([]TickResult, 0)
	for index := range jobs {
		job := &jobs[index]
		if !job.Enabled || !jobDue(*job, now) {
			continue
		}
		result := TickResult{
			JobID:     job.ID,
			Name:      job.Name,
			Username:  job.Username,
			Triggered: true,
			Timestamp: now,
		}
		chatResult, runErr := runner(ctx, job.Username, job.Prompt)
		job.LastRunAt = now.Format(time.RFC3339)
		job.UpdatedAt = now
		job.RunCount++
		if runErr != nil {
			result.Error = runErr.Error()
			job.LastError = runErr.Error()
			job.NextRunAt = nextRunAfterFailure(*job, now)
			if job.Schedule.Kind == ScheduleKindOnce {
				job.Enabled = false
			}
			results = append(results, result)
			if audit != nil {
				_ = audit(ctx, job.Username, "cron_job_failed", fmt.Sprintf("job_id=%s name=%s err=%s", job.ID, job.Name, runErr.Error()))
			}
			continue
		}

		outputFile, outputErr := m.writeOutput(*job, chatResult, now)
		result.Success = outputErr == nil
		result.SessionID = chatResult.SessionID
		result.OutputFile = outputFile
		if outputErr != nil {
			result.Error = outputErr.Error()
			job.LastError = outputErr.Error()
		} else {
			job.LastOutputFile = outputFile
			job.LastError = ""
		}
		job.LastSessionID = chatResult.SessionID
		job.NextRunAt = nextRunAfterSuccess(*job, now)
		if job.Schedule.Kind == ScheduleKindOnce {
			job.Enabled = false
		}
		results = append(results, result)
		if audit != nil {
			_ = audit(ctx, job.Username, "cron_job_ran", fmt.Sprintf("job_id=%s name=%s session_id=%d", job.ID, job.Name, chatResult.SessionID))
		}
	}
	if err := m.saveJobs(jobs); err != nil {
		return nil, err
	}
	return results, nil
}

func (m *Manager) ensureDirs() error {
	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		return err
	}
	return os.MkdirAll(m.outputDir, 0o755)
}

func (m *Manager) loadJobs() ([]Job, error) {
	data, err := os.ReadFile(m.jobsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []Job{}, nil
		}
		return nil, err
	}
	var jobs []Job
	if len(data) == 0 {
		return []Job{}, nil
	}
	if err := json.Unmarshal(data, &jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

func (m *Manager) saveJobs(jobs []Job) error {
	raw, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := m.jobsFile + ".tmp"
	if err := os.WriteFile(tmpPath, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, m.jobsFile)
}

func (m *Manager) writeOutput(job Job, result app.ChatResult, now time.Time) (string, error) {
	dir := filepath.Join(m.outputDir, sanitizeName(job.ID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	filename := filepath.Join(dir, now.Format("20060102T150405Z")+".md")
	body := fmt.Sprintf(
		"# Cron Job Output\n\n- job_id: %s\n- name: %s\n- username: %s\n- session_id: %d\n- model: %s\n- ran_at: %s\n\n## Prompt\n\n%s\n\n## Response\n\n%s\n",
		job.ID,
		job.Name,
		job.Username,
		result.SessionID,
		result.Model,
		now.Format(time.RFC3339),
		job.Prompt,
		result.Response,
	)
	if err := os.WriteFile(filename, []byte(body), 0o644); err != nil {
		return "", err
	}
	return filename, nil
}

func firstNextRun(schedule Schedule, now time.Time) string {
	switch schedule.Kind {
	case ScheduleKindOnce:
		return schedule.RunAt
	case ScheduleKindInterval:
		return now.Add(time.Duration(schedule.Interval) * time.Minute).Format(time.RFC3339)
	default:
		return ""
	}
}

func jobDue(job Job, now time.Time) bool {
	if strings.TrimSpace(job.NextRunAt) == "" {
		return false
	}
	nextRun, err := time.Parse(time.RFC3339, job.NextRunAt)
	if err != nil {
		return false
	}
	return !nextRun.After(now)
}

func nextRunAfterSuccess(job Job, now time.Time) string {
	if job.Schedule.Kind != ScheduleKindInterval {
		return ""
	}
	return now.Add(time.Duration(job.Schedule.Interval) * time.Minute).Format(time.RFC3339)
}

func nextRunAfterFailure(job Job, now time.Time) string {
	if job.Schedule.Kind != ScheduleKindInterval {
		return ""
	}
	return now.Add(time.Duration(job.Schedule.Interval) * time.Minute).Format(time.RFC3339)
}

func parseDurationMinutes(raw string) (int, error) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return 0, fmt.Errorf("duration is required")
	}
	if len(raw) < 2 {
		return 0, fmt.Errorf("invalid duration %q", raw)
	}
	unit := raw[len(raw)-1]
	valueText := strings.TrimSpace(raw[:len(raw)-1])
	value, err := strconv.Atoi(valueText)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("invalid duration %q", raw)
	}
	switch unit {
	case 'm':
		return value, nil
	case 'h':
		return value * 60, nil
	case 'd':
		return value * 1440, nil
	default:
		return 0, fmt.Errorf("invalid duration %q", raw)
	}
}

func sanitizeName(raw string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", " ", "_", ":", "_")
	return replacer.Replace(strings.TrimSpace(raw))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
