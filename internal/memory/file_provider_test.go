package memory

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"go-hermes-agent/internal/config"
)

func TestFileProviderWriteReadAndPrefetch(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = filepath.Join(t.TempDir(), "data")
	provider := NewFileProvider(cfg)
	if _, err := provider.Write(context.Background(), "alice", "memory", "add", "Project uses Telegram gateway", ""); err != nil {
		t.Fatalf("write memory: %v", err)
	}
	if _, err := provider.Write(context.Background(), "alice", "user", "add", "User prefers concise answers", ""); err != nil {
		t.Fatalf("write user memory: %v", err)
	}
	snapshot, err := provider.Read(context.Background(), "alice")
	if err != nil {
		t.Fatalf("read memory: %v", err)
	}
	if len(snapshot.MemoryEntries) != 1 || len(snapshot.UserEntries) != 1 {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	block, err := provider.Prefetch(context.Background(), "alice", "telegram concise")
	if err != nil {
		t.Fatalf("prefetch: %v", err)
	}
	if !strings.Contains(block, "Telegram gateway") || !strings.Contains(block, "concise answers") {
		t.Fatalf("unexpected prefetch block: %s", block)
	}
}
