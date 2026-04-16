package memory

import "context"

// Snapshot is the normalized memory view returned to callers.
type Snapshot struct {
	MemoryEntries []string `json:"memory_entries"`
	UserEntries   []string `json:"user_entries"`
}

// Provider is the pluggable memory backend contract.
type Provider interface {
	Name() string
	Prefetch(ctx context.Context, username, query string) (string, error)
	SyncTurn(ctx context.Context, username, userContent, assistantContent string) error
	Read(ctx context.Context, username string) (Snapshot, error)
	Write(ctx context.Context, username, target, action, content, match string) (Snapshot, error)
}
