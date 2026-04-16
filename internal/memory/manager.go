package memory

import "context"

// Manager coordinates memory reads, writes, and recall prefetches.
type Manager struct {
	provider Provider
	enabled  bool
}

// NewManager creates a memory manager for the supplied provider.
func NewManager(provider Provider, enabled bool) *Manager {
	return &Manager{provider: provider, enabled: enabled}
}

// Enabled reports whether memory is available for use.
func (m *Manager) Enabled() bool {
	return m != nil && m.enabled && m.provider != nil
}

// Prefetch returns memory context relevant to the next user query.
func (m *Manager) Prefetch(ctx context.Context, username, query string) (string, error) {
	if !m.Enabled() {
		return "", nil
	}
	return m.provider.Prefetch(ctx, username, query)
}

// SyncTurn lets the provider update memory after one completed turn.
func (m *Manager) SyncTurn(ctx context.Context, username, userContent, assistantContent string) error {
	if !m.Enabled() {
		return nil
	}
	return m.provider.SyncTurn(ctx, username, userContent, assistantContent)
}

// Read returns the current memory snapshot for a user.
func (m *Manager) Read(ctx context.Context, username string) (Snapshot, error) {
	if !m.Enabled() {
		return Snapshot{}, nil
	}
	return m.provider.Read(ctx, username)
}

// Write applies a memory update operation and returns the new snapshot.
func (m *Manager) Write(ctx context.Context, username, target, action, content, match string) (Snapshot, error) {
	if !m.Enabled() {
		return Snapshot{}, nil
	}
	return m.provider.Write(ctx, username, target, action, content, match)
}
