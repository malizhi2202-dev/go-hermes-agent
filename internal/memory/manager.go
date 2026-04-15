package memory

import "context"

type Manager struct {
	provider Provider
	enabled  bool
}

func NewManager(provider Provider, enabled bool) *Manager {
	return &Manager{provider: provider, enabled: enabled}
}

func (m *Manager) Enabled() bool {
	return m != nil && m.enabled && m.provider != nil
}

func (m *Manager) Prefetch(ctx context.Context, username, query string) (string, error) {
	if !m.Enabled() {
		return "", nil
	}
	return m.provider.Prefetch(ctx, username, query)
}

func (m *Manager) SyncTurn(ctx context.Context, username, userContent, assistantContent string) error {
	if !m.Enabled() {
		return nil
	}
	return m.provider.SyncTurn(ctx, username, userContent, assistantContent)
}

func (m *Manager) Read(ctx context.Context, username string) (Snapshot, error) {
	if !m.Enabled() {
		return Snapshot{}, nil
	}
	return m.provider.Read(ctx, username)
}

func (m *Manager) Write(ctx context.Context, username, target, action, content, match string) (Snapshot, error) {
	if !m.Enabled() {
		return Snapshot{}, nil
	}
	return m.provider.Write(ctx, username, target, action, content, match)
}
