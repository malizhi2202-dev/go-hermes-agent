package trajectory

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

	"go-hermes-agent/internal/store"
)

// ListFilters scopes trajectory listing.
type ListFilters struct {
	Limit     int
	RunName   string
	Model     string
	Source    string
	Completed *bool
}

// Summary aggregates trajectory records by source, model, and run name.
type Summary struct {
	Total      int            `json:"total"`
	Completed  int            `json:"completed"`
	BySource   map[string]int `json:"by_source"`
	ByModel    map[string]int `json:"by_model"`
	ByRunName  map[string]int `json:"by_run_name"`
	LastRecord string         `json:"last_record,omitempty"`
}

// Message is one ShareGPT-style trajectory message.
type Message struct {
	From  string `json:"from"`
	Value string `json:"value"`
}

// Record is one persisted trajectory entry.
type Record struct {
	ID         string            `json:"id"`
	Username   string            `json:"username"`
	SessionID  int64             `json:"session_id"`
	Model      string            `json:"model"`
	Prompt     string            `json:"prompt"`
	Response   string            `json:"response"`
	Completed  bool              `json:"completed"`
	Messages   []Message         `json:"conversations"`
	Metadata   map[string]any    `json:"metadata,omitempty"`
	Timestamp  time.Time         `json:"timestamp"`
	Source     string            `json:"source"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// Manager stores and retrieves lightweight trajectory JSONL files.
type Manager struct {
	dir string
}

// NewManager creates a trajectory manager rooted in one data directory.
func NewManager(dataDir string) *Manager {
	return &Manager{dir: filepath.Join(dataDir, "trajectories")}
}

// Dir returns the trajectory storage directory.
func (m *Manager) Dir() string {
	return m.dir
}

// BuildFromSession converts one stored chat session into a trajectory record.
func (m *Manager) BuildFromSession(ctx context.Context, st *store.Store, username string, sessionID int64) (Record, error) {
	session, err := st.GetSession(ctx, sessionID)
	if err != nil {
		return Record{}, err
	}
	messages, err := st.GetMessages(ctx, sessionID)
	if err != nil {
		return Record{}, err
	}
	conversations := make([]Message, 0, len(messages))
	for _, msg := range messages {
		conversations = append(conversations, Message{
			From:  normalizeRole(msg.Role),
			Value: msg.Content,
		})
	}
	timestamp := session.CreatedAt.UTC()
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	return Record{
		ID:        fmt.Sprintf("session-%d", sessionID),
		Username:  username,
		SessionID: sessionID,
		Model:     session.Model,
		Prompt:    session.Prompt,
		Response:  session.Response,
		Completed: true,
		Messages:  conversations,
		Metadata: map[string]any{
			"kind":              session.Kind,
			"task_id":           session.TaskID,
			"parent_session_id": session.ParentSessionID.Int64,
		},
		Timestamp: timestamp,
		Source:    "chat-session",
	}, nil
}

// Save appends one trajectory record to a per-user JSONL file.
func (m *Manager) Save(record Record) (string, error) {
	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		return "", err
	}
	record.Timestamp = record.Timestamp.UTC()
	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now().UTC()
	}
	if strings.TrimSpace(record.ID) == "" {
		record.ID = fmt.Sprintf("%s-%d", record.Username, record.Timestamp.UnixNano())
	}
	path := filepath.Join(m.dir, sanitizeFilename(record.Username)+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(record); err != nil {
		return "", err
	}
	return path, nil
}

// List returns recent trajectory records for one user.
func (m *Manager) List(username string, limit int) ([]Record, error) {
	return m.ListFiltered(username, ListFilters{Limit: limit})
}

// ListFiltered returns recent trajectory records for one user with lightweight filters.
func (m *Manager) ListFiltered(username string, filters ListFilters) ([]Record, error) {
	if filters.Limit <= 0 {
		filters.Limit = 50
	}
	path := filepath.Join(m.dir, sanitizeFilename(username)+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	records := make([]Record, 0, filters.Limit)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var record Record
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			continue
		}
		if filters.RunName != "" && record.Attributes["run_name"] != filters.RunName {
			continue
		}
		if filters.Model != "" && record.Model != filters.Model {
			continue
		}
		if filters.Source != "" && record.Source != filters.Source {
			continue
		}
		if filters.Completed != nil && record.Completed != *filters.Completed {
			continue
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].Timestamp.After(records[j].Timestamp)
	})
	if len(records) > filters.Limit {
		records = records[:filters.Limit]
	}
	return records, nil
}

// Get loads one trajectory by ID for one user.
func (m *Manager) Get(username, id string) (Record, error) {
	records, err := m.List(username, 10_000)
	if err != nil {
		return Record{}, err
	}
	for _, record := range records {
		if record.ID == id {
			return record, nil
		}
	}
	return Record{}, fmt.Errorf("trajectory %q not found", id)
}

// Summarize returns lightweight aggregate counts for one user's trajectories.
func (m *Manager) Summarize(username string, filters ListFilters) (Summary, error) {
	records, err := m.ListFiltered(username, ListFilters{
		Limit:     10_000,
		RunName:   filters.RunName,
		Model:     filters.Model,
		Source:    filters.Source,
		Completed: filters.Completed,
	})
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{
		BySource:  make(map[string]int),
		ByModel:   make(map[string]int),
		ByRunName: make(map[string]int),
	}
	for _, record := range records {
		summary.Total++
		if record.Completed {
			summary.Completed++
		}
		summary.BySource[record.Source]++
		summary.ByModel[record.Model]++
		if runName := record.Attributes["run_name"]; strings.TrimSpace(runName) != "" {
			summary.ByRunName[runName]++
		}
		if summary.LastRecord == "" {
			summary.LastRecord = record.ID
		}
	}
	return summary, nil
}

func normalizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "assistant":
		return "assistant"
	case "tool":
		return "tool"
	default:
		return "user"
	}
}

func sanitizeFilename(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return "anonymous"
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", " ", "_", ":", "_")
	return replacer.Replace(input)
}
