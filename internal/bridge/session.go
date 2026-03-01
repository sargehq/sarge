package bridge

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

// SessionState describes the current state of a session.
type SessionState int

const (
	// SessionStarting indicates the process is being spawned.
	SessionStarting SessionState = iota
	// SessionReady indicates the session is idle and ready to accept prompts.
	SessionReady
	// SessionStreaming indicates the session is currently processing a prompt.
	SessionStreaming
	// SessionDead indicates the session process has exited.
	SessionDead
)

func (s SessionState) String() string {
	switch s {
	case SessionStarting:
		return "starting"
	case SessionReady:
		return "ready"
	case SessionStreaming:
		return "streaming"
	case SessionDead:
		return "dead"
	default:
		return "unknown"
	}
}

// SessionConfig holds configuration for spawning a pi RPC session.
type SessionConfig struct {
	// Provider selects the AI provider (e.g., "anthropic", "openai", "google").
	Provider string
	// Model specifies which model to use.
	Model string
	// Thinking sets the reasoning level (e.g., "low", "medium", "high").
	Thinking string
	// WorkDir is the working directory for the pi process.
	WorkDir string
	// ExtraArgs contains additional arguments to pass to pi.
	ExtraArgs []string
	// Env contains additional environment variables for the process.
	Env []string
}

// Session represents a single pi RPC session backed by a child process.
type Session struct {
	id     string
	config SessionConfig

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	events    chan Event
	state     atomic.Int32
	streaming atomic.Bool

	mu       sync.Mutex
	err      error // last error, protected by mu
	cancelFn context.CancelFunc

	// done is closed when the session goroutines have finished.
	done chan struct{}
}

// newSession creates a Session but does not start it. Use start() to spawn the process.
func newSession(id string, cfg SessionConfig) *Session {
	s := &Session{
		id:     id,
		config: cfg,
		events: make(chan Event, 256),
		done:   make(chan struct{}),
	}
	s.state.Store(int32(SessionStarting))
	return s
}

// ID returns the session identifier.
func (s *Session) ID() string { return s.id }

// State returns the current session state.
func (s *Session) State() SessionState { return SessionState(s.state.Load()) }

// IsStreaming returns true if the session is currently processing a prompt.
func (s *Session) IsStreaming() bool { return s.streaming.Load() }

// Events returns a read-only channel of events from this session.
// The channel is closed when the session dies.
func (s *Session) Events() <-chan Event { return s.events }

// Err returns the last error that caused the session to die, if any.
func (s *Session) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// start spawns the pi RPC process and begins reading events.
func (s *Session) start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	s.cancelFn = cancel

	args := []string{"--mode", "rpc", "--no-session"}
	if s.config.Provider != "" {
		args = append(args, "--provider", s.config.Provider)
	}
	if s.config.Model != "" {
		args = append(args, "--model", s.config.Model)
	}
	if s.config.Thinking != "" {
		args = append(args, "--thinking", s.config.Thinking)
	}
	args = append(args, s.config.ExtraArgs...)

	s.cmd = exec.CommandContext(ctx, "pi", args...)
	if s.config.WorkDir != "" {
		s.cmd.Dir = s.config.WorkDir
	}
	if len(s.config.Env) > 0 {
		s.cmd.Env = append(s.cmd.Environ(), s.config.Env...)
	}

	var err error
	s.stdin, err = s.cmd.StdinPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("stdin pipe: %w", err)
	}
	s.stdout, err = s.cmd.StdoutPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("stdout pipe: %w", err)
	}
	s.stderr, err = s.cmd.StderrPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := s.cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("start pi process: %w", err)
	}

	s.state.Store(int32(SessionReady))

	// Read events from stdout in background.
	go s.readLoop()
	// Wait for process exit in background.
	go s.waitLoop()
	// Drain stderr in background.
	go s.drainStderr()

	return nil
}

// sendCommand marshals a command to JSON and writes it to the pi process stdin.
func (s *Session) sendCommand(cmd any) error {
	data, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("marshal command: %w", err)
	}
	data = append(data, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stdin == nil {
		return fmt.Errorf("session %s: stdin closed", s.id)
	}
	if _, err := s.stdin.Write(data); err != nil {
		return fmt.Errorf("write to pi stdin: %w", err)
	}
	return nil
}

// Prompt sends a prompt to the pi session.
func (s *Session) Prompt(message string) error {
	if SessionState(s.state.Load()) == SessionDead {
		return fmt.Errorf("session %s is dead", s.id)
	}
	s.streaming.Store(true)
	return s.sendCommand(map[string]string{
		"type":    "prompt",
		"message": message,
	})
}

// PromptWithBehavior sends a prompt with a streaming behavior directive.
func (s *Session) PromptWithBehavior(message string, behavior string) error {
	if SessionState(s.state.Load()) == SessionDead {
		return fmt.Errorf("session %s is dead", s.id)
	}
	return s.sendCommand(map[string]string{
		"type":              "prompt",
		"message":           message,
		"streamingBehavior": behavior,
	})
}

// Steer sends a steering message to interrupt the agent mid-run.
func (s *Session) Steer(message string) error {
	return s.sendCommand(map[string]string{
		"type":    "steer",
		"message": message,
	})
}

// FollowUp queues a follow-up message for after the agent finishes.
func (s *Session) FollowUp(message string) error {
	return s.sendCommand(map[string]string{
		"type":    "follow_up",
		"message": message,
	})
}

// Abort cancels the current agent operation.
func (s *Session) Abort() error {
	return s.sendCommand(map[string]string{
		"type": "abort",
	})
}

// GetState requests the current session state from pi.
func (s *Session) GetState() error {
	return s.sendCommand(map[string]string{
		"type": "get_state",
	})
}

// Kill terminates the pi process.
func (s *Session) Kill() error {
	if s.cancelFn != nil {
		s.cancelFn()
	}
	// Give the process a moment to exit gracefully.
	select {
	case <-s.done:
		return nil
	case <-time.After(5 * time.Second):
	}
	if s.cmd != nil && s.cmd.Process != nil {
		return s.cmd.Process.Kill()
	}
	return nil
}

// Wait blocks until the session process exits.
func (s *Session) Wait() error {
	<-s.done
	return s.Err()
}

// readLoop reads JSON lines from stdout and dispatches events.
func (s *Session) readLoop() {
	scanner := bufio.NewScanner(s.stdout)
	// Allow large lines (some events can be big with embedded messages).
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		// Parse just the type field first.
		var base struct {
			Type EventType `json:"type"`
		}
		if err := json.Unmarshal(line, &base); err != nil {
			continue // skip malformed lines
		}

		evt := Event{
			Type: base.Type,
			Raw:  make(json.RawMessage, len(line)),
		}
		copy(evt.Raw, line)

		// Track streaming state from agent lifecycle events.
		switch base.Type {
		case EventAgentStart:
			s.streaming.Store(true)
		case EventAgentEnd:
			s.streaming.Store(false)
		}

		select {
		case s.events <- evt:
		default:
			// Drop event if channel is full to avoid blocking.
		}
	}
}

// waitLoop waits for the process to exit and cleans up.
func (s *Session) waitLoop() {
	defer close(s.done)
	defer close(s.events)

	err := s.cmd.Wait()
	s.mu.Lock()
	s.err = err
	s.mu.Unlock()

	s.state.Store(int32(SessionDead))
}

// drainStderr reads and discards stderr to prevent the process from blocking.
func (s *Session) drainStderr() {
	if s.stderr != nil {
		_, _ = io.Copy(io.Discard, s.stderr)
	}
}
