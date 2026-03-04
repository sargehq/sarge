package ptysession

import (
	"context"
	"fmt"
	"sync"
)

// Manager manages a collection of PTY sessions. It is the interactive-session
// counterpart to bridge.Bridge (which manages headless RPC sessions).
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewManager creates a new PTY session manager.
func NewManager() *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		sessions: make(map[string]*Session),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Spawn creates and starts a new PTY session with the given ID.
// If a session with the same ID exists and is alive, returns an error.
func (m *Manager) Spawn(id string, cfg SessionConfig) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.sessions[id]; ok {
		if existing.State() != SessionDead {
			return nil, fmt.Errorf("pty session %s already exists and is alive", id)
		}
		delete(m.sessions, id)
	}

	session := New(id, cfg)
	if err := session.Start(m.ctx); err != nil {
		return nil, fmt.Errorf("spawn pty session %s: %w", id, err)
	}

	m.sessions[id] = session
	return session, nil
}

// Get returns the session with the given ID, or nil if not found.
func (m *Manager) Get(id string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[id]
}

// List returns a snapshot of all session IDs and their states.
func (m *Manager) List() map[string]SessionState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]SessionState, len(m.sessions))
	for id, s := range m.sessions {
		result[id] = s.State()
	}
	return result
}

// Kill terminates the session with the given ID.
func (m *Manager) Kill(id string) error {
	m.mu.Lock()
	s, ok := m.sessions[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("pty session %s not found", id)
	}
	delete(m.sessions, id)
	m.mu.Unlock()

	return s.Kill()
}

// KillAll terminates all sessions.
func (m *Manager) KillAll() error {
	m.cancel()

	m.mu.Lock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.sessions = make(map[string]*Session)
	m.mu.Unlock()

	var firstErr error
	for _, s := range sessions {
		if err := s.Kill(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// RemoveDead removes all dead sessions from the registry.
func (m *Manager) RemoveDead() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, s := range m.sessions {
		if s.State() == SessionDead {
			delete(m.sessions, id)
		}
	}
}
