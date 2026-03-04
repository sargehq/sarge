// Package ptysession manages interactive pi sessions backed by a PTY and
// virtual terminal emulator. Instead of parsing structured RPC events, it
// captures the full rendered terminal output from pi and makes it available
// for embedding in a Bubbletea TUI.
package ptysession

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"

	"github.com/charmbracelet/x/vt"
	pty "github.com/creack/pty/v2"
	"github.com/sargehq/sarge/internal/logging"
)

// SessionState describes the current state of a PTY session.
type SessionState int

const (
	SessionStarting SessionState = iota
	SessionRunning
	SessionDead
)

func (s SessionState) String() string {
	switch s {
	case SessionStarting:
		return "starting"
	case SessionRunning:
		return "running"
	case SessionDead:
		return "dead"
	default:
		return "unknown"
	}
}

// SessionConfig holds configuration for spawning a pi PTY session.
type SessionConfig struct {
	// Args are the arguments to pass to pi (e.g. no --mode rpc).
	Args []string
	// WorkDir is the working directory for the pi process.
	WorkDir string
	// Env contains additional environment variables for the process.
	Env []string
	// Width and Height are the initial terminal dimensions.
	Width  int
	Height int
	// InitialPrompt is an optional prompt to send to stdin after startup.
	// This is written directly to the PTY as if the user typed it + Enter.
	InitialPrompt string
}

// Session represents a single pi session backed by a PTY and virtual terminal.
type Session struct {
	id     string
	config SessionConfig

	cmd *exec.Cmd
	ptm *os.File // PTY master

	emu   *vt.Emulator
	state atomic.Int32

	mu       sync.Mutex
	err      error
	cancelFn context.CancelFunc

	// done is closed when the session has fully exited.
	done chan struct{}

	// dirty is set when new output has been written to the emulator.
	dirty atomic.Bool

	// onOutput is called (if set) when new output arrives, for waking the TUI.
	onOutput func()
}

// New creates a Session but does not start it. Call Start() to spawn the process.
func New(id string, cfg SessionConfig) *Session {
	if cfg.Width <= 0 {
		cfg.Width = 80
	}
	if cfg.Height <= 0 {
		cfg.Height = 24
	}
	s := &Session{
		id:     id,
		config: cfg,
		done:   make(chan struct{}),
	}
	s.state.Store(int32(SessionStarting))
	return s
}

// ID returns the session identifier.
func (s *Session) ID() string { return s.id }

// State returns the current session state.
func (s *Session) State() SessionState { return SessionState(s.state.Load()) }

// IsDirty returns true if there is new output since the last Render call.
func (s *Session) IsDirty() bool { return s.dirty.Load() }

// SetOnOutput sets a callback invoked when new output arrives from the PTY.
// This is typically used to send a tea.Msg to wake the Bubbletea event loop.
func (s *Session) SetOnOutput(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onOutput = fn
}

// Start spawns the pi process with a PTY and begins reading output.
func (s *Session) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	s.cancelFn = cancel

	// Build the command. Run pi normally (not RPC mode).
	args := make([]string, len(s.config.Args))
	copy(args, s.config.Args)

	s.cmd = exec.CommandContext(ctx, "pi", args...)
	if s.config.WorkDir != "" {
		s.cmd.Dir = s.config.WorkDir
	}
	if len(s.config.Env) > 0 {
		s.cmd.Env = append(os.Environ(), s.config.Env...)
	}

	// Create the virtual terminal emulator.
	s.emu = vt.NewEmulator(s.config.Width, s.config.Height)

	// Start the process with a PTY.
	ptm, err := pty.StartWithSize(s.cmd, &pty.Winsize{
		Rows: uint16(s.config.Height),
		Cols: uint16(s.config.Width),
	})
	if err != nil {
		cancel()
		return fmt.Errorf("start pi with pty: %w", err)
	}
	s.ptm = ptm

	s.state.Store(int32(SessionRunning))

	// Read PTY output and feed it to the emulator.
	go s.readLoop()
	// Wait for process exit.
	go s.waitLoop()

	// Send initial prompt if configured.
	if s.config.InitialPrompt != "" {
		go func() {
			if _, err := s.ptm.WriteString(s.config.InitialPrompt + "\n"); err != nil {
				logging.Warn("failed to write initial prompt to pty", "error", err, "session", s.id)
			}
		}()
	}

	return nil
}

// WriteInput sends raw keyboard input to the PTY.
func (s *Session) WriteInput(data []byte) error {
	if s.ptm == nil {
		return fmt.Errorf("session %s: pty not open", s.id)
	}
	_, err := s.ptm.Write(data)
	return err
}

// WriteString sends a string to the PTY (convenience wrapper).
func (s *Session) WriteString(text string) error {
	return s.WriteInput([]byte(text))
}

// Resize updates the PTY and emulator dimensions.
func (s *Session) Resize(width, height int) {
	if width <= 0 || height <= 0 {
		return
	}
	if s.ptm != nil {
		_ = pty.Setsize(s.ptm, &pty.Winsize{
			Rows: uint16(height),
			Cols: uint16(width),
		})
	}
	if s.emu != nil {
		s.emu.Resize(width, height)
	}
}

// Render returns the current terminal screen as a string with ANSI escape codes.
// Clears the dirty flag.
func (s *Session) Render() string {
	s.dirty.Store(false)
	if s.emu == nil {
		return ""
	}
	return s.emu.Render()
}

// Kill terminates the pi process.
func (s *Session) Kill() error {
	if s.cancelFn != nil {
		s.cancelFn()
	}
	if s.ptm != nil {
		_ = s.ptm.Close()
	}
	if s.emu != nil {
		_ = s.emu.Close()
	}
	return nil
}

// Wait blocks until the session process exits.
func (s *Session) Wait() {
	<-s.done
}

// Err returns the last error, if any.
func (s *Session) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// readLoop reads from the PTY master and writes to the virtual terminal emulator.
func (s *Session) readLoop() {
	buf := make([]byte, 32*1024)
	for {
		n, err := s.ptm.Read(buf)
		if n > 0 {
			// Feed output to the virtual terminal emulator.
			_, _ = s.emu.Write(buf[:n])
			s.dirty.Store(true)

			// Notify the TUI that new output is available.
			s.mu.Lock()
			cb := s.onOutput
			s.mu.Unlock()
			if cb != nil {
				cb()
			}
		}
		if err != nil {
			if err != io.EOF {
				logging.Debug("pty read error", "error", err, "session", s.id)
			}
			return
		}
	}
}

// waitLoop waits for the process to exit and cleans up.
func (s *Session) waitLoop() {
	defer close(s.done)

	err := s.cmd.Wait()
	s.mu.Lock()
	s.err = err
	s.mu.Unlock()

	s.state.Store(int32(SessionDead))

	// Signal one final dirty so the TUI picks up the dead state.
	s.dirty.Store(true)
	s.mu.Lock()
	cb := s.onOutput
	s.mu.Unlock()
	if cb != nil {
		cb()
	}
}
