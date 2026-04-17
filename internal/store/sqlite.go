package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Store wraps the SQLite connection used by the Go runtime.
//
// It owns user auth state, sessions, messages, audit rows, extension state,
// context summaries, and multi-agent traces.
type Store struct {
	db *sql.DB
}

// User is the persisted local account record.
type User struct {
	ID             int64
	Username       string
	PasswordHash   string
	Role           string
	CreatedAt      time.Time
	LockedUntil    sql.NullTime
	FailedAttempts int
}

// Session represents one stored conversation or delegated task run.
type Session struct {
	ID              int64         `json:"id"`
	Username        string        `json:"username"`
	Model           string        `json:"model"`
	Prompt          string        `json:"prompt"`
	Response        string        `json:"response"`
	Kind            string        `json:"kind"`
	TaskID          string        `json:"task_id,omitempty"`
	ParentSessionID sql.NullInt64 `json:"parent_session_id,omitempty"`
	CreatedAt       time.Time     `json:"created_at"`
}

// Message is one chat or tool transcript row inside a session.
type Message struct {
	ID        int64     `json:"id"`
	SessionID int64     `json:"session_id"`
	Username  string    `json:"username,omitempty"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// SearchResult is one FTS-backed history match.
type SearchResult struct {
	SessionID   int64     `json:"session_id"`
	MessageID   int64     `json:"message_id"`
	Username    string    `json:"username"`
	Model       string    `json:"model"`
	Role        string    `json:"role"`
	Content     string    `json:"content"`
	CreatedAt   time.Time `json:"created_at"`
	SessionTime time.Time `json:"session_time"`
}

// SearchFilters controls history search scope.
type SearchFilters struct {
	Username  string
	Query     string
	Role      string
	SessionID int64
	FromTime  time.Time
	ToTime    time.Time
	Limit     int
}

// AuditRecord is one audit event written by the runtime.
type AuditRecord struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Action    string    `json:"action"`
	Detail    string    `json:"detail"`
	CreatedAt time.Time `json:"created_at"`
}

// AuditFilters controls audit log queries.
type AuditFilters struct {
	Username string
	Action   string
	FromTime time.Time
	ToTime   time.Time
	Limit    int
	Offset   int
}

// ExtensionState stores persisted enable/disable state for dynamic extensions.
type ExtensionState struct {
	Kind      string    `json:"kind"`
	Name      string    `json:"name"`
	Enabled   bool      `json:"enabled"`
	Hash      string    `json:"hash"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ExtensionHookRecord stores one lifecycle hook execution result.
type ExtensionHookRecord struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Kind      string    `json:"kind"`
	Name      string    `json:"name"`
	Phase     string    `json:"phase"`
	Hook      string    `json:"hook"`
	Status    string    `json:"status"`
	Output    string    `json:"output,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// ExtensionHookFilters scopes lifecycle hook result queries.
type ExtensionHookFilters struct {
	Username string
	Kind     string
	Name     string
	Phase    string
	Limit    int
	Offset   int
}

// ContextSummary stores the persisted compressed summary for one user.
type ContextSummary struct {
	Username  string    `json:"username"`
	Summary   string    `json:"summary"`
	Strategy  string    `json:"strategy"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MultiAgentTraceRecord stores one structured child-agent trajectory step.
type MultiAgentTraceRecord struct {
	ID                int64     `json:"id"`
	Username          string    `json:"username"`
	ParentSessionID   int64     `json:"parent_session_id"`
	ChildSessionID    int64     `json:"child_session_id"`
	TaskID            string    `json:"task_id"`
	Iteration         int       `json:"iteration"`
	Type              string    `json:"type"`
	Tool              string    `json:"tool,omitempty"`
	InputJSON         string    `json:"input_json,omitempty"`
	OutputJSON        string    `json:"output_json,omitempty"`
	SnapshotJSON      string    `json:"snapshot_json,omitempty"`
	Verified          bool      `json:"verified"`
	Verifier          string    `json:"verifier,omitempty"`
	VerificationClass string    `json:"verification_class,omitempty"`
	Error             string    `json:"error,omitempty"`
	Note              string    `json:"note,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

// MultiAgentTraceFilters scopes multi-agent trace queries.
type MultiAgentTraceFilters struct {
	Username        string
	ParentSessionID int64
	ChildSessionID  int64
	TaskID          string
	FromTime        time.Time
	ToTime          time.Time
	Limit           int
	Offset          int
}

// MultiAgentTraceSummary summarizes trace usage and failures by tool and step type.
type MultiAgentTraceSummary struct {
	Tool       string `json:"tool,omitempty"`
	Type       string `json:"type"`
	Total      int    `json:"total"`
	Failures   int    `json:"failures"`
	LastError  string `json:"last_error,omitempty"`
	LastSeenAt string `json:"last_seen_at,omitempty"`
}

// MultiAgentTraceHotspot summarizes failure-heavy child sessions and tasks.
type MultiAgentTraceHotspot struct {
	ParentSessionID int64  `json:"parent_session_id"`
	ChildSessionID  int64  `json:"child_session_id"`
	TaskID          string `json:"task_id"`
	Total           int    `json:"total"`
	Failures        int    `json:"failures"`
}

// MultiAgentVerifierSummary summarizes delegated tool verification outcomes.
type MultiAgentVerifierSummary struct {
	Tool              string `json:"tool"`
	Verifier          string `json:"verifier"`
	VerificationClass string `json:"verification_class,omitempty"`
	Verified          int    `json:"verified"`
	Failed            int    `json:"failed"`
	Total             int    `json:"total"`
}

// AuditActionSummary groups audit events by action.
type AuditActionSummary struct {
	Action string `json:"action"`
	Total  int    `json:"total"`
}

// UserInsights summarizes recent user activity for CLI observability.
type UserInsights struct {
	Username               string               `json:"username"`
	Since                  time.Time            `json:"since"`
	SessionsTotal          int                  `json:"sessions_total"`
	ChatSessions           int                  `json:"chat_sessions"`
	MultiAgentParent       int                  `json:"multiagent_parent_sessions"`
	MultiAgentChild        int                  `json:"multiagent_child_sessions"`
	MessagesTotal          int                  `json:"messages_total"`
	AuditRecordsTotal      int                  `json:"audit_records_total"`
	AuditActions           []AuditActionSummary `json:"audit_actions"`
	LatestSessionCreatedAt string               `json:"latest_session_created_at,omitempty"`
}

// Open opens or creates the SQLite database and applies the required schema.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`
PRAGMA journal_mode=WAL;
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'admin',
    created_at TEXT NOT NULL,
    locked_until TEXT,
    failed_attempts INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL,
    model TEXT NOT NULL,
    prompt TEXT NOT NULL,
    response TEXT NOT NULL,
    kind TEXT NOT NULL DEFAULT 'chat',
    task_id TEXT NOT NULL DEFAULT '',
    parent_session_id INTEGER,
    created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id, id);
CREATE INDEX IF NOT EXISTS idx_sessions_username ON sessions(username, id DESC);
CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
    content,
    username UNINDEXED,
    model UNINDEXED,
    role UNINDEXED,
    session_id UNINDEXED,
    message_id UNINDEXED,
    created_at UNINDEXED
);
CREATE TRIGGER IF NOT EXISTS messages_ai AFTER INSERT ON messages BEGIN
  INSERT INTO messages_fts(content, username, model, role, session_id, message_id, created_at)
  SELECT NEW.content, s.username, s.model, NEW.role, NEW.session_id, NEW.id, NEW.created_at
  FROM sessions s WHERE s.id = NEW.session_id;
END;
CREATE TRIGGER IF NOT EXISTS messages_ad AFTER DELETE ON messages BEGIN
  DELETE FROM messages_fts WHERE message_id = OLD.id;
END;
CREATE TABLE IF NOT EXISTS audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT,
    action TEXT NOT NULL,
    detail TEXT,
    created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS processed_gateway_updates (
    provider TEXT NOT NULL,
    external_id TEXT NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY (provider, external_id)
);
CREATE TABLE IF NOT EXISTS extension_states (
    kind TEXT NOT NULL,
    name TEXT NOT NULL,
    enabled INTEGER NOT NULL,
    hash TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL,
    PRIMARY KEY (kind, name)
);
CREATE TABLE IF NOT EXISTS extension_hook_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL,
    kind TEXT NOT NULL,
    name TEXT NOT NULL,
    phase TEXT NOT NULL,
    hook TEXT NOT NULL,
    status TEXT NOT NULL,
    output TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS context_summaries (
    username TEXT NOT NULL PRIMARY KEY,
    summary TEXT NOT NULL,
    strategy TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS multiagent_traces (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL,
    parent_session_id INTEGER NOT NULL,
    child_session_id INTEGER NOT NULL,
    task_id TEXT NOT NULL,
    iteration INTEGER NOT NULL,
    type TEXT NOT NULL,
    tool TEXT NOT NULL DEFAULT '',
    input_json TEXT NOT NULL DEFAULT '',
    output_json TEXT NOT NULL DEFAULT '',
    snapshot_json TEXT NOT NULL DEFAULT '',
    verified INTEGER NOT NULL DEFAULT 0,
    verifier TEXT NOT NULL DEFAULT '',
    verification_class TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_multiagent_traces_parent ON multiagent_traces(parent_session_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_multiagent_traces_child ON multiagent_traces(child_session_id, id DESC);
`); err != nil {
		return nil, fmt.Errorf("init schema: %w", err)
	}
	if err := ensureColumn(db, "sessions", "kind", "TEXT NOT NULL DEFAULT 'chat'"); err != nil {
		return nil, fmt.Errorf("ensure sessions.kind: %w", err)
	}
	if err := ensureColumn(db, "sessions", "task_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return nil, fmt.Errorf("ensure sessions.task_id: %w", err)
	}
	if err := ensureColumn(db, "sessions", "parent_session_id", "INTEGER"); err != nil {
		return nil, fmt.Errorf("ensure sessions.parent_session_id: %w", err)
	}
	if err := ensureColumn(db, "multiagent_traces", "snapshot_json", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return nil, fmt.Errorf("ensure multiagent_traces.snapshot_json: %w", err)
	}
	if err := ensureColumn(db, "multiagent_traces", "verified", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return nil, fmt.Errorf("ensure multiagent_traces.verified: %w", err)
	}
	if err := ensureColumn(db, "multiagent_traces", "verifier", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return nil, fmt.Errorf("ensure multiagent_traces.verifier: %w", err)
	}
	if err := ensureColumn(db, "multiagent_traces", "verification_class", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return nil, fmt.Errorf("ensure multiagent_traces.verification_class: %w", err)
	}
	return &Store{db: db}, nil
}

func ensureColumn(db *sql.DB, table, column, definition string) error {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid       int
			name      string
			colType   string
			notNull   int
			dfltValue sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	_, err = db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition))
	return err
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// Close closes the underlying SQLite connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// CreateUser inserts a new local user.
func (s *Store) CreateUser(ctx context.Context, username, passwordHash, role string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO users (username, password_hash, role, created_at)
VALUES (?, ?, ?, ?)`, username, passwordHash, role, time.Now().UTC().Format(time.RFC3339))
	return err
}

// GetUser loads a local user by username.
func (s *Store) GetUser(ctx context.Context, username string) (*User, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, username, password_hash, role, created_at, locked_until, failed_attempts
FROM users WHERE username = ?`, username)
	var user User
	var createdAt string
	var lockedUntil sql.NullString
	if err := row.Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &createdAt, &lockedUntil, &user.FailedAttempts); err != nil {
		return nil, err
	}
	parsedCreated, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, err
	}
	user.CreatedAt = parsedCreated
	if lockedUntil.Valid {
		parsedLocked, err := time.Parse(time.RFC3339, lockedUntil.String)
		if err != nil {
			return nil, err
		}
		user.LockedUntil = sql.NullTime{Valid: true, Time: parsedLocked}
	}
	return &user, nil
}

// UpdateLoginFailure updates failed login counters and optional lockout time.
func (s *Store) UpdateLoginFailure(ctx context.Context, username string, failedAttempts int, lockedUntil *time.Time) error {
	var value any
	if lockedUntil != nil {
		value = lockedUntil.UTC().Format(time.RFC3339)
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE users SET failed_attempts = ?, locked_until = ? WHERE username = ?`,
		failedAttempts, value, username)
	return err
}

// ResetLoginFailures clears failed login counters for a user.
func (s *Store) ResetLoginFailures(ctx context.Context, username string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE users SET failed_attempts = 0, locked_until = NULL WHERE username = ?`, username)
	return err
}

// WriteAudit appends one audit log row.
func (s *Store) WriteAudit(ctx context.Context, username, action, detail string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO audit_log (username, action, detail, created_at)
VALUES (?, ?, ?, ?)`, username, action, detail, time.Now().UTC().Format(time.RFC3339))
	return err
}

// UpsertExtensionState saves extension enablement and integrity state.
func (s *Store) UpsertExtensionState(ctx context.Context, kind, name string, enabled bool, hash string) error {
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO extension_states (kind, name, enabled, hash, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(kind, name) DO UPDATE SET
  enabled = excluded.enabled,
  hash = excluded.hash,
  updated_at = excluded.updated_at
`, kind, name, enabledInt, hash, time.Now().UTC().Format(time.RFC3339))
	return err
}

// ListExtensionStates returns all persisted extension state rows.
func (s *Store) ListExtensionStates(ctx context.Context) ([]ExtensionState, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT kind, name, enabled, hash, updated_at
FROM extension_states
ORDER BY kind, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var states []ExtensionState
	for rows.Next() {
		var state ExtensionState
		var enabledInt int
		var updatedAt string
		if err := rows.Scan(&state.Kind, &state.Name, &enabledInt, &state.Hash, &updatedAt); err != nil {
			return nil, err
		}
		state.Enabled = enabledInt == 1
		state.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
		if err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, rows.Err()
}

// InsertExtensionHookRun persists one lifecycle hook execution result.
func (s *Store) InsertExtensionHookRun(ctx context.Context, record ExtensionHookRecord) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO extension_hook_runs (username, kind, name, phase, hook, status, output, error, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.Username,
		record.Kind,
		record.Name,
		record.Phase,
		record.Hook,
		record.Status,
		record.Output,
		record.Error,
		time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// ListExtensionHookRuns returns persisted lifecycle hook results.
func (s *Store) ListExtensionHookRuns(ctx context.Context, filters ExtensionHookFilters) ([]ExtensionHookRecord, error) {
	if filters.Limit <= 0 {
		filters.Limit = 50
	}
	if filters.Offset < 0 {
		filters.Offset = 0
	}
	query := `
SELECT id, username, kind, name, phase, hook, status, output, error, created_at
FROM extension_hook_runs
WHERE 1=1`
	args := make([]any, 0, 6)
	if filters.Username != "" {
		query += ` AND username = ?`
		args = append(args, filters.Username)
	}
	if filters.Kind != "" {
		query += ` AND kind = ?`
		args = append(args, filters.Kind)
	}
	if filters.Name != "" {
		query += ` AND name = ?`
		args = append(args, filters.Name)
	}
	if filters.Phase != "" {
		query += ` AND phase = ?`
		args = append(args, filters.Phase)
	}
	query += ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, filters.Limit, filters.Offset)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []ExtensionHookRecord
	for rows.Next() {
		var record ExtensionHookRecord
		var createdAt string
		if err := rows.Scan(&record.ID, &record.Username, &record.Kind, &record.Name, &record.Phase, &record.Hook, &record.Status, &record.Output, &record.Error, &createdAt); err != nil {
			return nil, err
		}
		record.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

// GetContextSummary returns the stored compressed context summary for a user.
func (s *Store) GetContextSummary(ctx context.Context, username string) (ContextSummary, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT username, summary, strategy, updated_at
FROM context_summaries
WHERE username = ?`, username)
	var summary ContextSummary
	var updatedAt string
	if err := row.Scan(&summary.Username, &summary.Summary, &summary.Strategy, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return ContextSummary{}, nil
		}
		return ContextSummary{}, err
	}
	parsed, err := time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return ContextSummary{}, err
	}
	summary.UpdatedAt = parsed
	return summary, nil
}

// UpsertContextSummary writes or updates the stored context summary for a user.
func (s *Store) UpsertContextSummary(ctx context.Context, username, summary, strategy string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO context_summaries (username, summary, strategy, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(username) DO UPDATE SET
  summary = excluded.summary,
  strategy = excluded.strategy,
  updated_at = excluded.updated_at
`, username, summary, strategy, time.Now().UTC().Format(time.RFC3339))
	return err
}

// ListAudit returns audit events filtered by username and action.
func (s *Store) ListAudit(ctx context.Context, username, action string, limit int) ([]AuditRecord, error) {
	return s.ListAuditFiltered(ctx, AuditFilters{
		Username: username,
		Action:   action,
		Limit:    limit,
	})
}

// ListAuditFiltered returns audit events using structured filters.
func (s *Store) ListAuditFiltered(ctx context.Context, filters AuditFilters) ([]AuditRecord, error) {
	if filters.Limit <= 0 {
		filters.Limit = 50
	}
	if filters.Offset < 0 {
		filters.Offset = 0
	}
	query := `
SELECT id, username, action, detail, created_at
FROM audit_log
WHERE 1=1`
	args := make([]any, 0, 6)
	if filters.Username != "" {
		query += ` AND username = ?`
		args = append(args, filters.Username)
	}
	if filters.Action != "" {
		operator := "="
		if strings.Contains(filters.Action, "%") || strings.Contains(filters.Action, "_") {
			operator = "LIKE"
		}
		query += ` AND action ` + operator + ` ?`
		args = append(args, filters.Action)
	}
	if !filters.FromTime.IsZero() {
		query += ` AND created_at >= ?`
		args = append(args, filters.FromTime.UTC().Format(time.RFC3339))
	}
	if !filters.ToTime.IsZero() {
		query += ` AND created_at <= ?`
		args = append(args, filters.ToTime.UTC().Format(time.RFC3339))
	}
	query += ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, filters.Limit, filters.Offset)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []AuditRecord
	for rows.Next() {
		var record AuditRecord
		var createdAt string
		if err := rows.Scan(&record.ID, &record.Username, &record.Action, &record.Detail, &createdAt); err != nil {
			return nil, err
		}
		record.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

// SummarizeAuditActions groups audit events by action name.
func (s *Store) SummarizeAuditActions(ctx context.Context, filters AuditFilters) ([]AuditActionSummary, error) {
	query := `
SELECT action, COUNT(*) AS total
FROM audit_log
WHERE 1=1`
	args := make([]any, 0, 6)
	if filters.Username != "" {
		query += ` AND username = ?`
		args = append(args, filters.Username)
	}
	if filters.Action != "" {
		query += ` AND action LIKE ?`
		args = append(args, filters.Action)
	}
	if !filters.FromTime.IsZero() {
		query += ` AND created_at >= ?`
		args = append(args, filters.FromTime.UTC().Format(time.RFC3339))
	}
	if !filters.ToTime.IsZero() {
		query += ` AND created_at <= ?`
		args = append(args, filters.ToTime.UTC().Format(time.RFC3339))
	}
	query += ` GROUP BY action ORDER BY total DESC, action ASC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var summaries []AuditActionSummary
	for rows.Next() {
		var summary AuditActionSummary
		if err := rows.Scan(&summary.Action, &summary.Total); err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
	}
	return summaries, rows.Err()
}

// BuildUserInsights returns lightweight recent activity aggregates for one user.
func (s *Store) BuildUserInsights(ctx context.Context, username string, since time.Time) (UserInsights, error) {
	insights := UserInsights{
		Username: username,
		Since:    since.UTC(),
	}
	sinceRFC3339 := since.UTC().Format(time.RFC3339)
	row := s.db.QueryRowContext(ctx, `
SELECT
  COUNT(*) AS sessions_total,
  SUM(CASE WHEN kind = 'chat' THEN 1 ELSE 0 END) AS chat_sessions,
  SUM(CASE WHEN kind = 'multiagent_parent' THEN 1 ELSE 0 END) AS multiagent_parent_sessions,
  SUM(CASE WHEN kind = 'multiagent_child' THEN 1 ELSE 0 END) AS multiagent_child_sessions,
  COALESCE(MAX(created_at), '')
FROM sessions
WHERE username = ?
  AND created_at >= ?`, username, sinceRFC3339)
	if err := row.Scan(
		&insights.SessionsTotal,
		&insights.ChatSessions,
		&insights.MultiAgentParent,
		&insights.MultiAgentChild,
		&insights.LatestSessionCreatedAt,
	); err != nil {
		return UserInsights{}, err
	}
	msgRow := s.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM messages m
JOIN sessions s ON s.id = m.session_id
WHERE s.username = ?
  AND m.created_at >= ?`, username, sinceRFC3339)
	if err := msgRow.Scan(&insights.MessagesTotal); err != nil {
		return UserInsights{}, err
	}
	auditRow := s.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM audit_log
WHERE username = ?
  AND created_at >= ?`, username, sinceRFC3339)
	if err := auditRow.Scan(&insights.AuditRecordsTotal); err != nil {
		return UserInsights{}, err
	}
	actions, err := s.SummarizeAuditActions(ctx, AuditFilters{
		Username: username,
		FromTime: since,
		Limit:    10,
	})
	if err != nil {
		return UserInsights{}, err
	}
	insights.AuditActions = actions
	return insights, nil
}

// CreateSessionOptions controls extra metadata recorded for a session.
type CreateSessionOptions struct {
	Kind            string
	TaskID          string
	ParentSessionID int64
}

// CreateSession creates a standard chat session row.
func (s *Store) CreateSession(ctx context.Context, username, model, prompt, response string) (int64, error) {
	return s.CreateSessionWithOptions(ctx, username, model, prompt, response, CreateSessionOptions{})
}

// CreateSessionWithOptions creates a session row with delegated-task metadata.
func (s *Store) CreateSessionWithOptions(ctx context.Context, username, model, prompt, response string, opts CreateSessionOptions) (int64, error) {
	kind := opts.Kind
	if kind == "" {
		kind = "chat"
	}
	var parent any
	if opts.ParentSessionID > 0 {
		parent = opts.ParentSessionID
	}
	result, err := s.db.ExecContext(ctx, `
INSERT INTO sessions (username, model, prompt, response, kind, task_id, parent_session_id, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		username, model, prompt, response, kind, opts.TaskID, parent, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// ListSessions returns recent sessions for a user.
func (s *Store) ListSessions(ctx context.Context, username string, limit int) ([]Session, error) {
	return s.ListSessionsPage(ctx, username, limit, 0)
}

// ListSessionsPage returns recent sessions for a user with pagination.
func (s *Store) ListSessionsPage(ctx context.Context, username string, limit, offset int) ([]Session, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, username, model, prompt, response, kind, task_id, parent_session_id, created_at
FROM sessions
WHERE username = ?
ORDER BY id DESC
LIMIT ? OFFSET ?`, username, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sessions []Session
	for rows.Next() {
		var session Session
		var createdAt string
		if err := rows.Scan(&session.ID, &session.Username, &session.Model, &session.Prompt, &session.Response, &session.Kind, &session.TaskID, &session.ParentSessionID, &createdAt); err != nil {
			return nil, err
		}
		session.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

// GetSession loads one session by ID.
func (s *Store) GetSession(ctx context.Context, sessionID int64) (Session, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, username, model, prompt, response, kind, task_id, parent_session_id, created_at
FROM sessions
WHERE id = ?`, sessionID)
	var session Session
	var createdAt string
	if err := row.Scan(&session.ID, &session.Username, &session.Model, &session.Prompt, &session.Response, &session.Kind, &session.TaskID, &session.ParentSessionID, &createdAt); err != nil {
		return Session{}, err
	}
	parsed, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return Session{}, err
	}
	session.CreatedAt = parsed
	return session, nil
}

// AddMessage appends one transcript row to a session.
func (s *Store) AddMessage(ctx context.Context, sessionID int64, role, content string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO messages (session_id, role, content, created_at)
VALUES (?, ?, ?, ?)`,
		sessionID, role, content, time.Now().UTC().Format(time.RFC3339))
	return err
}

// UpdateSessionResponse updates the latest response snapshot on a session.
func (s *Store) UpdateSessionResponse(ctx context.Context, sessionID int64, response string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE sessions
SET response = ?
WHERE id = ?`, response, sessionID)
	return err
}

// DeleteSession removes one session and its transcript rows.
func (s *Store) DeleteSession(ctx context.Context, sessionID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
DELETE FROM messages
WHERE session_id = ?`, sessionID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
DELETE FROM sessions
WHERE id = ?`, sessionID); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteLastTurn removes the most recent user/assistant turn pair from a session
// and refreshes the session snapshot fields to match the new tail state.
func (s *Store) DeleteLastTurn(ctx context.Context, sessionID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `
SELECT id, role
FROM messages
WHERE session_id = ?
ORDER BY id DESC
LIMIT 2`, sessionID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var ids []int64
	var roles []string
	for rows.Next() {
		var id int64
		var role string
		if err := rows.Scan(&id, &role); err != nil {
			return err
		}
		ids = append(ids, id)
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(ids) < 2 {
		return fmt.Errorf("session %d does not have a full turn to delete", sessionID)
	}
	if roles[0] != "assistant" || roles[1] != "user" {
		return fmt.Errorf("session %d last turn is not a user/assistant pair", sessionID)
	}
	if _, err := tx.ExecContext(ctx, `
DELETE FROM messages
WHERE id IN (?, ?)`, ids[0], ids[1]); err != nil {
		return err
	}

	row := tx.QueryRowContext(ctx, `
SELECT
  COALESCE((
    SELECT content
    FROM messages
    WHERE session_id = ? AND role = 'user'
    ORDER BY id DESC
    LIMIT 1
  ), ''),
  COALESCE((
    SELECT content
    FROM messages
    WHERE session_id = ? AND role = 'assistant'
    ORDER BY id DESC
    LIMIT 1
  ), '')`, sessionID, sessionID)
	var prompt string
	var response string
	if err := row.Scan(&prompt, &response); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE sessions
SET prompt = ?, response = ?
WHERE id = ?`, prompt, response, sessionID); err != nil {
		return err
	}
	return tx.Commit()
}

// InsertMultiAgentTrace inserts one structured child-agent trajectory step.
func (s *Store) InsertMultiAgentTrace(ctx context.Context, record MultiAgentTraceRecord) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO multiagent_traces (
    username, parent_session_id, child_session_id, task_id, iteration, type, tool,
    input_json, output_json, snapshot_json, verified, verifier, verification_class, error, note, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.Username,
		record.ParentSessionID,
		record.ChildSessionID,
		record.TaskID,
		record.Iteration,
		record.Type,
		record.Tool,
		record.InputJSON,
		record.OutputJSON,
		record.SnapshotJSON,
		boolToInt(record.Verified),
		record.Verifier,
		record.VerificationClass,
		record.Error,
		record.Note,
		time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// ListMultiAgentTraces returns stored child-agent trajectory steps.
func (s *Store) ListMultiAgentTraces(ctx context.Context, filters MultiAgentTraceFilters) ([]MultiAgentTraceRecord, error) {
	if filters.Limit <= 0 {
		filters.Limit = 50
	}
	if filters.Offset < 0 {
		filters.Offset = 0
	}
	query := `
SELECT id, username, parent_session_id, child_session_id, task_id, iteration, type, tool,
       input_json, output_json, snapshot_json, verified, verifier, verification_class, error, note, created_at
FROM multiagent_traces
WHERE 1=1`
	args := make([]any, 0, 8)
	if filters.Username != "" {
		query += ` AND username = ?`
		args = append(args, filters.Username)
	}
	if filters.ParentSessionID > 0 {
		query += ` AND parent_session_id = ?`
		args = append(args, filters.ParentSessionID)
	}
	if filters.ChildSessionID > 0 {
		query += ` AND child_session_id = ?`
		args = append(args, filters.ChildSessionID)
	}
	if filters.TaskID != "" {
		query += ` AND task_id = ?`
		args = append(args, filters.TaskID)
	}
	if !filters.FromTime.IsZero() {
		query += ` AND created_at >= ?`
		args = append(args, filters.FromTime.UTC().Format(time.RFC3339))
	}
	if !filters.ToTime.IsZero() {
		query += ` AND created_at <= ?`
		args = append(args, filters.ToTime.UTC().Format(time.RFC3339))
	}
	query += ` ORDER BY id ASC LIMIT ? OFFSET ?`
	args = append(args, filters.Limit, filters.Offset)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []MultiAgentTraceRecord
	for rows.Next() {
		var record MultiAgentTraceRecord
		var createdAt string
		if err := rows.Scan(
			&record.ID,
			&record.Username,
			&record.ParentSessionID,
			&record.ChildSessionID,
			&record.TaskID,
			&record.Iteration,
			&record.Type,
			&record.Tool,
			&record.InputJSON,
			&record.OutputJSON,
			&record.SnapshotJSON,
			&record.Verified,
			&record.Verifier,
			&record.VerificationClass,
			&record.Error,
			&record.Note,
			&createdAt,
		); err != nil {
			return nil, err
		}
		record.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

// ListMultiAgentTraceFailures returns only trace rows that contain execution errors.
func (s *Store) ListMultiAgentTraceFailures(ctx context.Context, filters MultiAgentTraceFilters) ([]MultiAgentTraceRecord, error) {
	if filters.Limit <= 0 {
		filters.Limit = 50
	}
	if filters.Offset < 0 {
		filters.Offset = 0
	}
	query := `
SELECT id, username, parent_session_id, child_session_id, task_id, iteration, type, tool,
       input_json, output_json, snapshot_json, verified, verifier, verification_class, error, note, created_at
FROM multiagent_traces
WHERE error <> ''`
	args := make([]any, 0, 8)
	if filters.Username != "" {
		query += ` AND username = ?`
		args = append(args, filters.Username)
	}
	if filters.ParentSessionID > 0 {
		query += ` AND parent_session_id = ?`
		args = append(args, filters.ParentSessionID)
	}
	if filters.ChildSessionID > 0 {
		query += ` AND child_session_id = ?`
		args = append(args, filters.ChildSessionID)
	}
	if filters.TaskID != "" {
		query += ` AND task_id = ?`
		args = append(args, filters.TaskID)
	}
	if !filters.FromTime.IsZero() {
		query += ` AND created_at >= ?`
		args = append(args, filters.FromTime.UTC().Format(time.RFC3339))
	}
	if !filters.ToTime.IsZero() {
		query += ` AND created_at <= ?`
		args = append(args, filters.ToTime.UTC().Format(time.RFC3339))
	}
	query += ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, filters.Limit, filters.Offset)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []MultiAgentTraceRecord
	for rows.Next() {
		var record MultiAgentTraceRecord
		var createdAt string
		if err := rows.Scan(
			&record.ID, &record.Username, &record.ParentSessionID, &record.ChildSessionID,
			&record.TaskID, &record.Iteration, &record.Type, &record.Tool,
			&record.InputJSON, &record.OutputJSON, &record.SnapshotJSON, &record.Verified, &record.Verifier, &record.VerificationClass, &record.Error, &record.Note, &createdAt,
		); err != nil {
			return nil, err
		}
		record.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

// SummarizeMultiAgentTraces returns grouped counts and failure totals for traces.
func (s *Store) SummarizeMultiAgentTraces(ctx context.Context, filters MultiAgentTraceFilters) ([]MultiAgentTraceSummary, error) {
	query := `
SELECT tool, type, COUNT(*) AS total,
       SUM(CASE WHEN error <> '' THEN 1 ELSE 0 END) AS failures,
       MAX(created_at) AS last_seen_at,
       MAX(CASE WHEN error <> '' THEN error ELSE '' END) AS last_error
FROM multiagent_traces
WHERE 1=1`
	args := make([]any, 0, 8)
	if filters.Username != "" {
		query += ` AND username = ?`
		args = append(args, filters.Username)
	}
	if filters.ParentSessionID > 0 {
		query += ` AND parent_session_id = ?`
		args = append(args, filters.ParentSessionID)
	}
	if filters.ChildSessionID > 0 {
		query += ` AND child_session_id = ?`
		args = append(args, filters.ChildSessionID)
	}
	if filters.TaskID != "" {
		query += ` AND task_id = ?`
		args = append(args, filters.TaskID)
	}
	if !filters.FromTime.IsZero() {
		query += ` AND created_at >= ?`
		args = append(args, filters.FromTime.UTC().Format(time.RFC3339))
	}
	if !filters.ToTime.IsZero() {
		query += ` AND created_at <= ?`
		args = append(args, filters.ToTime.UTC().Format(time.RFC3339))
	}
	query += ` GROUP BY tool, type ORDER BY failures DESC, total DESC, tool ASC, type ASC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var summaries []MultiAgentTraceSummary
	for rows.Next() {
		var summary MultiAgentTraceSummary
		if err := rows.Scan(&summary.Tool, &summary.Type, &summary.Total, &summary.Failures, &summary.LastSeenAt, &summary.LastError); err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
	}
	return summaries, rows.Err()
}

// ListMultiAgentTraceHotspots returns task-level trace hotspots ordered by failures.
func (s *Store) ListMultiAgentTraceHotspots(ctx context.Context, filters MultiAgentTraceFilters) ([]MultiAgentTraceHotspot, error) {
	query := `
SELECT parent_session_id, child_session_id, task_id,
       COUNT(*) AS total,
       SUM(CASE WHEN error <> '' THEN 1 ELSE 0 END) AS failures
FROM multiagent_traces
WHERE 1=1`
	args := make([]any, 0, 8)
	if filters.Username != "" {
		query += ` AND username = ?`
		args = append(args, filters.Username)
	}
	if filters.ParentSessionID > 0 {
		query += ` AND parent_session_id = ?`
		args = append(args, filters.ParentSessionID)
	}
	if filters.ChildSessionID > 0 {
		query += ` AND child_session_id = ?`
		args = append(args, filters.ChildSessionID)
	}
	if filters.TaskID != "" {
		query += ` AND task_id = ?`
		args = append(args, filters.TaskID)
	}
	if !filters.FromTime.IsZero() {
		query += ` AND created_at >= ?`
		args = append(args, filters.FromTime.UTC().Format(time.RFC3339))
	}
	if !filters.ToTime.IsZero() {
		query += ` AND created_at <= ?`
		args = append(args, filters.ToTime.UTC().Format(time.RFC3339))
	}
	query += ` GROUP BY parent_session_id, child_session_id, task_id
ORDER BY failures DESC, total DESC, child_session_id DESC`
	if filters.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, filters.Limit)
		if filters.Offset > 0 {
			query += ` OFFSET ?`
			args = append(args, filters.Offset)
		}
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hotspots []MultiAgentTraceHotspot
	for rows.Next() {
		var hotspot MultiAgentTraceHotspot
		if err := rows.Scan(&hotspot.ParentSessionID, &hotspot.ChildSessionID, &hotspot.TaskID, &hotspot.Total, &hotspot.Failures); err != nil {
			return nil, err
		}
		hotspots = append(hotspots, hotspot)
	}
	return hotspots, rows.Err()
}

// SummarizeMultiAgentVerifierResults groups verification outcomes by tool and verifier.
func (s *Store) SummarizeMultiAgentVerifierResults(ctx context.Context, filters MultiAgentTraceFilters) ([]MultiAgentVerifierSummary, error) {
	query := `
SELECT tool, verifier, verification_class,
       SUM(CASE WHEN verified = 1 THEN 1 ELSE 0 END) AS verified_count,
       SUM(CASE WHEN verified = 0 AND verifier <> '' THEN 1 ELSE 0 END) AS failed_count,
       COUNT(*) AS total
FROM multiagent_traces
WHERE verifier <> ''`
	args := make([]any, 0, 8)
	if filters.Username != "" {
		query += ` AND username = ?`
		args = append(args, filters.Username)
	}
	if filters.ParentSessionID > 0 {
		query += ` AND parent_session_id = ?`
		args = append(args, filters.ParentSessionID)
	}
	if filters.ChildSessionID > 0 {
		query += ` AND child_session_id = ?`
		args = append(args, filters.ChildSessionID)
	}
	if filters.TaskID != "" {
		query += ` AND task_id = ?`
		args = append(args, filters.TaskID)
	}
	if !filters.FromTime.IsZero() {
		query += ` AND created_at >= ?`
		args = append(args, filters.FromTime.UTC().Format(time.RFC3339))
	}
	if !filters.ToTime.IsZero() {
		query += ` AND created_at <= ?`
		args = append(args, filters.ToTime.UTC().Format(time.RFC3339))
	}
	query += ` GROUP BY tool, verifier, verification_class ORDER BY failed_count DESC, total DESC, tool ASC, verification_class ASC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var summaries []MultiAgentVerifierSummary
	for rows.Next() {
		var summary MultiAgentVerifierSummary
		if err := rows.Scan(&summary.Tool, &summary.Verifier, &summary.VerificationClass, &summary.Verified, &summary.Failed, &summary.Total); err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
	}
	return summaries, rows.Err()
}

// GetMessages returns all messages for a session.
func (s *Store) GetMessages(ctx context.Context, sessionID int64) ([]Message, error) {
	return s.GetMessagesPage(ctx, sessionID, 0, 0)
}

// GetMessagesPage returns session messages using pagination.
func (s *Store) GetMessagesPage(ctx context.Context, sessionID int64, limit, offset int) ([]Message, error) {
	query := `
SELECT id, session_id, role, content, created_at
FROM messages
WHERE session_id = ?
ORDER BY id ASC`
	args := []any{sessionID}
	if limit > 0 {
		if offset < 0 {
			offset = 0
		}
		query += `
LIMIT ? OFFSET ?`
		args = append(args, limit, offset)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var messages []Message
	for rows.Next() {
		var message Message
		var createdAt string
		if err := rows.Scan(&message.ID, &message.SessionID, &message.Role, &message.Content, &createdAt); err != nil {
			return nil, err
		}
		message.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

// ListRecentMessagesByUsername returns the latest messages across a user's sessions.
func (s *Store) ListRecentMessagesByUsername(ctx context.Context, username string, limit int) ([]Message, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT m.id, m.session_id, s.username, m.role, m.content, m.created_at
FROM messages m
JOIN sessions s ON s.id = m.session_id
WHERE s.username = ?
ORDER BY m.id DESC
LIMIT ?`, username, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := make([]Message, 0, limit)
	for rows.Next() {
		var message Message
		var createdAt string
		if err := rows.Scan(&message.ID, &message.SessionID, &message.Username, &message.Role, &message.Content, &createdAt); err != nil {
			return nil, err
		}
		message.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, nil
}

// ListRecentMessagesBySession returns the latest messages inside one session.
func (s *Store) ListRecentMessagesBySession(ctx context.Context, sessionID int64, limit int) ([]Message, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, session_id, role, content, created_at
FROM messages
WHERE session_id = ?
ORDER BY id DESC
LIMIT ?`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := make([]Message, 0, limit)
	for rows.Next() {
		var message Message
		var createdAt string
		if err := rows.Scan(&message.ID, &message.SessionID, &message.Role, &message.Content, &createdAt); err != nil {
			return nil, err
		}
		message.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, nil
}

// SearchMessages runs FTS-backed history search with optional filters.
func (s *Store) SearchMessages(ctx context.Context, filters SearchFilters) ([]SearchResult, error) {
	query := normalizeFTSQuery(filters.Query)
	if filters.Limit <= 0 {
		filters.Limit = 20
	}
	sqlText := `
SELECT
    CAST(session_id AS INTEGER),
    CAST(message_id AS INTEGER),
    username,
    model,
    role,
    snippet(messages_fts, 0, '[', ']', '...', 12),
    created_at,
    (SELECT created_at FROM sessions WHERE id = CAST(session_id AS INTEGER))
FROM messages_fts
WHERE messages_fts MATCH ?
  AND username = ?`
	args := []any{query, filters.Username}
	if filters.Role != "" {
		sqlText += `
  AND role = ?`
		args = append(args, filters.Role)
	}
	if filters.SessionID > 0 {
		sqlText += `
  AND CAST(session_id AS INTEGER) = ?`
		args = append(args, filters.SessionID)
	}
	if !filters.FromTime.IsZero() {
		sqlText += `
  AND created_at >= ?`
		args = append(args, filters.FromTime.UTC().Format(time.RFC3339))
	}
	if !filters.ToTime.IsZero() {
		sqlText += `
  AND created_at <= ?`
		args = append(args, filters.ToTime.UTC().Format(time.RFC3339))
	}
	sqlText += `
ORDER BY CAST(message_id AS INTEGER) DESC
LIMIT ?`
	args = append(args, filters.Limit)
	rows, err := s.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []SearchResult
	for rows.Next() {
		var result SearchResult
		var messageTime string
		var sessionTime string
		if err := rows.Scan(
			&result.SessionID,
			&result.MessageID,
			&result.Username,
			&result.Model,
			&result.Role,
			&result.Content,
			&messageTime,
			&sessionTime,
		); err != nil {
			return nil, err
		}
		var err error
		result.CreatedAt, err = time.Parse(time.RFC3339, messageTime)
		if err != nil {
			return nil, err
		}
		result.SessionTime, err = time.Parse(time.RFC3339, sessionTime)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, rows.Err()
}

// MarkGatewayUpdateProcessed deduplicates gateway updates by provider and external ID.
func (s *Store) MarkGatewayUpdateProcessed(ctx context.Context, provider, externalID string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
INSERT OR IGNORE INTO processed_gateway_updates (provider, external_id, created_at)
VALUES (?, ?, ?)`, provider, externalID, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func normalizeFTSQuery(query string) string {
	parts := make([]string, 0)
	current := make([]rune, 0, len(query))
	flush := func() {
		if len(current) == 0 {
			return
		}
		parts = append(parts, string(current)+"*")
		current = current[:0]
	}
	for _, r := range query {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-':
			current = append(current, r)
		default:
			flush()
		}
	}
	flush()
	if len(parts) == 0 {
		return `""`
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += " AND " + parts[i]
	}
	return result
}
