package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type User struct {
	ID             int64
	Username       string
	PasswordHash   string
	Role           string
	CreatedAt      time.Time
	LockedUntil    sql.NullTime
	FailedAttempts int
}

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

type Message struct {
	ID        int64     `json:"id"`
	SessionID int64     `json:"session_id"`
	Username  string    `json:"username,omitempty"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

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

type SearchFilters struct {
	Username  string
	Query     string
	Role      string
	SessionID int64
	FromTime  time.Time
	ToTime    time.Time
	Limit     int
}

type AuditRecord struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Action    string    `json:"action"`
	Detail    string    `json:"detail"`
	CreatedAt time.Time `json:"created_at"`
}

type AuditFilters struct {
	Username string
	Action   string
	FromTime time.Time
	ToTime   time.Time
	Limit    int
	Offset   int
}

type ExtensionState struct {
	Kind      string    `json:"kind"`
	Name      string    `json:"name"`
	Enabled   bool      `json:"enabled"`
	Hash      string    `json:"hash"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ContextSummary struct {
	Username  string    `json:"username"`
	Summary   string    `json:"summary"`
	Strategy  string    `json:"strategy"`
	UpdatedAt time.Time `json:"updated_at"`
}

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
CREATE TABLE IF NOT EXISTS context_summaries (
    username TEXT NOT NULL PRIMARY KEY,
    summary TEXT NOT NULL,
    strategy TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
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

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) CreateUser(ctx context.Context, username, passwordHash, role string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO users (username, password_hash, role, created_at)
VALUES (?, ?, ?, ?)`, username, passwordHash, role, time.Now().UTC().Format(time.RFC3339))
	return err
}

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

func (s *Store) ResetLoginFailures(ctx context.Context, username string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE users SET failed_attempts = 0, locked_until = NULL WHERE username = ?`, username)
	return err
}

func (s *Store) WriteAudit(ctx context.Context, username, action, detail string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO audit_log (username, action, detail, created_at)
VALUES (?, ?, ?, ?)`, username, action, detail, time.Now().UTC().Format(time.RFC3339))
	return err
}

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

func (s *Store) ListAudit(ctx context.Context, username, action string, limit int) ([]AuditRecord, error) {
	return s.ListAuditFiltered(ctx, AuditFilters{
		Username: username,
		Action:   action,
		Limit:    limit,
	})
}

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
		query += ` AND action = ?`
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

type CreateSessionOptions struct {
	Kind            string
	TaskID          string
	ParentSessionID int64
}

func (s *Store) CreateSession(ctx context.Context, username, model, prompt, response string) (int64, error) {
	return s.CreateSessionWithOptions(ctx, username, model, prompt, response, CreateSessionOptions{})
}

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

func (s *Store) ListSessions(ctx context.Context, username string, limit int) ([]Session, error) {
	return s.ListSessionsPage(ctx, username, limit, 0)
}

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

func (s *Store) AddMessage(ctx context.Context, sessionID int64, role, content string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO messages (session_id, role, content, created_at)
VALUES (?, ?, ?, ?)`,
		sessionID, role, content, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) GetMessages(ctx context.Context, sessionID int64) ([]Message, error) {
	return s.GetMessagesPage(ctx, sessionID, 0, 0)
}

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
