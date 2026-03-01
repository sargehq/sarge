// Package bridge manages pi RPC sessions as child processes.
//
// Each session spawns a "pi --mode rpc --no-session" process and communicates
// with it over stdin/stdout using a JSON-lines protocol. The Bridge struct acts
// as a session registry, handling spawn, lookup, kill, and bulk shutdown.
package bridge

import (
	"context"
	"fmt"
	"sync"

	"github.com/sargehq/sarge/internal/project"
)

// Bridge manages a collection of pi RPC sessions.
type Bridge struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewBridge creates a new Bridge. The returned Bridge must be closed with
// KillAll when no longer needed.
func NewBridge() *Bridge {
	ctx, cancel := context.WithCancel(context.Background())
	return &Bridge{
		sessions: make(map[string]*Session),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// SessionConfigFromProject creates a SessionConfig from a project Config,
// using the pi-specific settings (provider, model, thinking).
func SessionConfigFromProject(workDir string, cfg *project.Config) SessionConfig {
	sc := SessionConfig{
		WorkDir: workDir,
	}
	if cfg != nil {
		sc.Provider = cfg.Pi.Provider
		sc.Model = cfg.Pi.Model
		sc.Thinking = cfg.Pi.Thinking
	}
	return sc
}

// SpawnSession creates and starts a new pi RPC session with the given ID.
// If a session with the same ID already exists and is alive, an error is returned.
func (b *Bridge) SpawnSession(id string, cfg SessionConfig) (*Session, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if existing, ok := b.sessions[id]; ok {
		if existing.State() != SessionDead {
			return nil, fmt.Errorf("session %s already exists and is alive", id)
		}
		// Clean up dead session entry before replacing.
		delete(b.sessions, id)
	}

	session := newSession(id, cfg)
	if err := session.start(b.ctx); err != nil {
		return nil, fmt.Errorf("spawn session %s: %w", id, err)
	}

	b.sessions[id] = session
	return session, nil
}

// GetSession returns the session with the given ID, or nil if not found.
func (b *Bridge) GetSession(id string) *Session {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.sessions[id]
}

// ListSessions returns a snapshot of all session IDs and their states.
func (b *Bridge) ListSessions() map[string]SessionState {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make(map[string]SessionState, len(b.sessions))
	for id, s := range b.sessions {
		result[id] = s.State()
	}
	return result
}

// KillSession terminates the session with the given ID.
// Returns an error if the session is not found.
func (b *Bridge) KillSession(id string) error {
	b.mu.Lock()
	s, ok := b.sessions[id]
	if !ok {
		b.mu.Unlock()
		return fmt.Errorf("session %s not found", id)
	}
	delete(b.sessions, id)
	b.mu.Unlock()

	return s.Kill()
}

// KillAll terminates all sessions and cancels the bridge context.
func (b *Bridge) KillAll() error {
	b.cancel()

	b.mu.Lock()
	sessions := make([]*Session, 0, len(b.sessions))
	for _, s := range b.sessions {
		sessions = append(sessions, s)
	}
	b.sessions = make(map[string]*Session)
	b.mu.Unlock()

	var firstErr error
	for _, s := range sessions {
		if err := s.Kill(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// RemoveDead removes all sessions in the Dead state from the registry.
func (b *Bridge) RemoveDead() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for id, s := range b.sessions {
		if s.State() == SessionDead {
			delete(b.sessions, id)
		}
	}
}
