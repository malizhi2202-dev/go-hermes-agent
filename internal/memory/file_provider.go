package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go-hermes-agent/internal/config"
)

const entryDelimiter = "\n§\n"

// FileProvider is the built-in file-backed memory provider.
type FileProvider struct {
	root            string
	memoryCharLimit int
	userCharLimit   int
	recallLimit     int
}

// NewFileProvider creates the built-in file-backed memory provider.
func NewFileProvider(cfg config.Config) *FileProvider {
	return &FileProvider{
		root:            filepath.Join(cfg.DataDir, "memories"),
		memoryCharLimit: cfg.Memory.MemoryCharLimit,
		userCharLimit:   cfg.Memory.UserCharLimit,
		recallLimit:     cfg.Memory.RecallLimit,
	}
}

// Name returns the provider identifier.
func (p *FileProvider) Name() string { return "builtin" }

// Prefetch recalls relevant memory snippets for the next prompt.
func (p *FileProvider) Prefetch(_ context.Context, username, query string) (string, error) {
	snapshot, err := p.Read(context.Background(), username)
	if err != nil {
		return "", err
	}
	type scored struct {
		text  string
		score int
		label string
	}
	scores := make([]scored, 0)
	for _, entry := range snapshot.MemoryEntries {
		if score := recallScore(query, entry); score > 0 {
			scores = append(scores, scored{text: entry, score: score, label: "memory"})
		}
	}
	for _, entry := range snapshot.UserEntries {
		if score := recallScore(query, entry); score > 0 {
			scores = append(scores, scored{text: entry, score: score, label: "user"})
		}
	}
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].score == scores[j].score {
			return scores[i].text < scores[j].text
		}
		return scores[i].score > scores[j].score
	})
	if len(scores) == 0 {
		return "", nil
	}
	if len(scores) > p.recallLimit {
		scores = scores[:p.recallLimit]
	}
	lines := []string{
		"<memory-context>",
		"[System note: The following is recalled memory context, not new user input.]",
	}
	for _, item := range scores {
		lines = append(lines, fmt.Sprintf("[%s] %s", item.label, item.text))
	}
	lines = append(lines, "</memory-context>")
	return strings.Join(lines, "\n"), nil
}

// SyncTurn is currently a no-op for the file-backed provider.
func (p *FileProvider) SyncTurn(_ context.Context, _ string, _, _ string) error {
	return nil
}

// Read returns the current memory snapshot for a user.
func (p *FileProvider) Read(_ context.Context, username string) (Snapshot, error) {
	if err := os.MkdirAll(p.userDir(username), 0o755); err != nil {
		return Snapshot{}, err
	}
	return Snapshot{
		MemoryEntries: p.readEntries(p.memoryPath(username)),
		UserEntries:   p.readEntries(p.userPath(username)),
	}, nil
}

// Write mutates one memory target and returns the updated snapshot.
func (p *FileProvider) Write(_ context.Context, username, target, action, content, match string) (Snapshot, error) {
	path, limit, err := p.resolveTarget(username, target)
	if err != nil {
		return Snapshot{}, err
	}
	entries := p.readEntries(path)
	switch action {
	case "add":
		content = strings.TrimSpace(content)
		if content == "" {
			return Snapshot{}, fmt.Errorf("content is required")
		}
		if contains(entries, content) {
			return p.Read(context.Background(), username)
		}
		next := append(entries, content)
		if len(strings.Join(next, entryDelimiter)) > limit {
			return Snapshot{}, fmt.Errorf("memory limit exceeded")
		}
		entries = next
	case "replace":
		content = strings.TrimSpace(content)
		match = strings.TrimSpace(match)
		if match == "" || content == "" {
			return Snapshot{}, fmt.Errorf("match and content are required")
		}
		index := indexContaining(entries, match)
		if index < 0 {
			return Snapshot{}, fmt.Errorf("no entry matched %q", match)
		}
		next := append([]string(nil), entries...)
		next[index] = content
		if len(strings.Join(next, entryDelimiter)) > limit {
			return Snapshot{}, fmt.Errorf("memory limit exceeded")
		}
		entries = next
	case "remove":
		match = strings.TrimSpace(match)
		if match == "" {
			return Snapshot{}, fmt.Errorf("match is required")
		}
		index := indexContaining(entries, match)
		if index < 0 {
			return Snapshot{}, fmt.Errorf("no entry matched %q", match)
		}
		entries = append(entries[:index], entries[index+1:]...)
	case "read":
		return p.Read(context.Background(), username)
	default:
		return Snapshot{}, fmt.Errorf("unsupported memory action %q", action)
	}
	if err := p.writeEntries(path, entries); err != nil {
		return Snapshot{}, err
	}
	return p.Read(context.Background(), username)
}

func (p *FileProvider) resolveTarget(username, target string) (string, int, error) {
	switch strings.ToLower(strings.TrimSpace(target)) {
	case "memory":
		return p.memoryPath(username), p.memoryCharLimit, nil
	case "user":
		return p.userPath(username), p.userCharLimit, nil
	default:
		return "", 0, fmt.Errorf("target must be memory or user")
	}
}

func (p *FileProvider) userDir(username string) string {
	return filepath.Join(p.root, hashUsername(username))
}

func (p *FileProvider) memoryPath(username string) string {
	return filepath.Join(p.userDir(username), "MEMORY.md")
}

func (p *FileProvider) userPath(username string) string {
	return filepath.Join(p.userDir(username), "USER.md")
}

func (p *FileProvider) readEntries(path string) []string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return nil
	}
	parts := strings.Split(text, entryDelimiter)
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func (p *FileProvider) writeEntries(path string, entries []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body := strings.Join(entries, entryDelimiter)
	return os.WriteFile(path, []byte(body), 0o644)
}

func recallScore(query, candidate string) int {
	query = strings.ToLower(strings.TrimSpace(query))
	candidate = strings.ToLower(strings.TrimSpace(candidate))
	if query == "" || candidate == "" {
		return 0
	}
	score := 0
	for _, token := range strings.FieldsFunc(query, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	}) {
		if len(token) < 2 {
			continue
		}
		if strings.Contains(candidate, token) {
			score++
		}
	}
	return score
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func indexContaining(values []string, match string) int {
	for i, value := range values {
		if strings.Contains(value, match) {
			return i
		}
	}
	return -1
}

func hashUsername(username string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(username)))
	return hex.EncodeToString(sum[:8])
}
