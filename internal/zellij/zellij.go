// Package zellij provides a client for interacting with the zellij terminal multiplexer.
// It abstracts session, tab, and pane management operations into a type-safe API.
package zellij

//go:generate moq -stub -out zellij_mock.go . SessionManager Session

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"text/template"
	"time"
)

//go:embed tab.kdl.tmpl
var tabLayoutTemplate string

// TabLayoutData contains the data for rendering a tab layout template.
type TabLayoutData struct {
	TabName  string
	PaneName string
	Command  string
	Args     []string
	Cwd      string
}

// CurrentSessionName returns the name of the zellij session we're currently inside,
// or empty string if not inside a zellij session.
func CurrentSessionName() string {
	return os.Getenv("ZELLIJ_SESSION_NAME")
}

// IsInsideTargetSession returns true if we're inside the specified session.
func IsInsideTargetSession(session string) bool {
	return CurrentSessionName() == session
}

// SessionManager defines the interface for managing zellij sessions.
// This abstraction enables testing without actual zellij commands.
//
// All session creation should go through EnsureSessionWithCommand to ensure
// every session starts with an initial tab. Direct session creation methods
// are intentionally excluded from this interface.
type SessionManager interface {
	SessionExists(ctx context.Context, name string) (bool, error)
	EnsureSessionWithCommand(ctx context.Context, sessionName, tabName, cwd, command string, args []string) (bool, error)

	// Session returns a Session interface bound to the specified session name.
	Session(name string) Session
}

// Session defines the interface for operations within a specific zellij session.
// Each Session instance is bound to a specific session name.
type Session interface {
	// Tab management
	CreateTabWithCommand(ctx context.Context, name, cwd, command string, args []string, paneName string) error
	SwitchToTab(ctx context.Context, name string) error
	QueryTabNames(ctx context.Context) ([]string, error)
	TabExists(ctx context.Context, name string) (bool, error)

	// High-level operations
	TerminateAndCloseTab(ctx context.Context, tabName string) error
	CloseTabByName(ctx context.Context, tabName string) error
}

// ASCIICtrlC is the ASCII code for Ctrl+C (interrupt)
const ASCIICtrlC = 3

// sessionManager implements SessionManager and creates session instances.
type sessionManager struct {
	TabCreateDelay   time.Duration
	CtrlCDelay       time.Duration
	CommandDelay     time.Duration
	SessionStartWait time.Duration
}

// session implements the Session interface for a specific zellij session.
type session struct {
	name           string
	TabCreateDelay time.Duration
	CtrlCDelay     time.Duration
}

// Compile-time checks.
var (
	_ SessionManager = (*sessionManager)(nil)
	_ Session        = (*session)(nil)
)

// New creates a new zellij client with default configuration.
func New() SessionManager {
	return &sessionManager{
		TabCreateDelay:   500 * time.Millisecond,
		CtrlCDelay:       500 * time.Millisecond,
		CommandDelay:     100 * time.Millisecond,
		SessionStartWait: 1 * time.Second,
	}
}

// Session returns a Session interface bound to the specified session name.
func (m *sessionManager) Session(name string) Session {
	return &session{
		name:           name,
		TabCreateDelay: m.TabCreateDelay,
		CtrlCDelay:     m.CtrlCDelay,
	}
}

// sessionArgs returns the appropriate session arguments for zellij commands.
// If we're inside the target session, returns empty slice (use local actions).
// Otherwise returns ["-s", session] to target the specific session.
func sessionArgs(session string) []string {
	if IsInsideTargetSession(session) {
		return nil
	}
	return []string{"-s", session}
}

// =============================================================================
// SessionManager implementation (client)
// =============================================================================

// ListSessions returns a list of all zellij session names.
func (m *sessionManager) ListSessions(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "zellij", "list-sessions", "-s")
	output, err := cmd.Output()
	if err != nil {
		// No sessions or zellij not running
		return nil, nil
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var sessions []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			// Session names may have additional info, take first field
			parts := strings.Fields(line)
			if len(parts) > 0 {
				sessions = append(sessions, parts[0])
			}
		}
	}
	return sessions, nil
}

// SessionExists checks if a session with the given name exists.
func (m *sessionManager) SessionExists(ctx context.Context, name string) (bool, error) {
	sessions, err := m.ListSessions(ctx)
	if err != nil {
		return false, err
	}
	return slices.Contains(sessions, name), nil
}

// IsSessionActive checks if a session exists and is active (not exited).
func (m *sessionManager) IsSessionActive(ctx context.Context, name string) (bool, error) {
	cmd := exec.CommandContext(ctx, "zellij", "list-sessions", "-n")
	output, err := cmd.Output()
	if err != nil {
		return false, nil
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}
		if parts[0] == name {
			// Session found - check if it's exited
			return !strings.Contains(line, "EXITED"), nil
		}
	}
	return false, nil
}

// createSessionWithCommand creates a new zellij session with an initial tab running a command.
// Uses the same layout template as CreateTabWithCommand for consistency.
func (m *sessionManager) createSessionWithCommand(ctx context.Context, sessionName, tabName, cwd, command string, args []string) error {
	// Render the tab layout template (same as CreateTabWithCommand)
	tmpl, err := template.New("tab").Parse(tabLayoutTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse tab layout template: %w", err)
	}

	data := TabLayoutData{
		TabName:  tabName,
		PaneName: tabName,
		Command:  command,
		Args:     args,
		Cwd:      cwd,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to render tab layout template: %w", err)
	}

	// Write layout to temp file
	tmpFile, err := os.CreateTemp("", "zellij-session-*.kdl")
	if err != nil {
		return fmt.Errorf("failed to create temp layout file: %w", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	if _, err := tmpFile.WriteString(buf.String()); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to write session layout file: %w", err)
	}
	_ = tmpFile.Close()

	// Create session with the layout
	// Use --new-session-with-layout for detached session creation
	cmd := exec.CommandContext(ctx, "zellij", "-s", sessionName, "--new-session-with-layout", tmpFile.Name())
	// Detach immediately by not connecting stdin/stdout
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start zellij session: %w", err)
	}

	// Don't wait for it - let it run in background
	go func() { _ = cmd.Wait() }()

	// Give it time to start
	time.Sleep(m.SessionStartWait)
	return nil
}

// DeleteSession deletes a zellij session by name.
func (m *sessionManager) DeleteSession(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "zellij", "delete-session", name)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete zellij session: %w", err)
	}
	return nil
}

// EnsureSessionWithCommand creates a session with an initial tab running a command if it doesn't exist,
// or deletes and recreates it if exited.
// Returns true if a new session was created, false if an existing session was reused.
func (m *sessionManager) EnsureSessionWithCommand(ctx context.Context, sessionName, tabName, cwd, command string, args []string) (bool, error) {
	active, err := m.IsSessionActive(ctx, sessionName)
	if err != nil {
		return false, err
	}
	if active {
		return false, nil
	}

	// Check if session exists but is exited
	exists, err := m.SessionExists(ctx, sessionName)
	if err != nil {
		return false, err
	}
	if exists {
		// Session is exited - delete it first
		if err := m.DeleteSession(ctx, sessionName); err != nil {
			return false, fmt.Errorf("failed to delete exited session: %w", err)
		}
	}

	if err := m.createSessionWithCommand(ctx, sessionName, tabName, cwd, command, args); err != nil {
		return false, err
	}
	return true, nil
}

// =============================================================================
// Session implementation (session)
// =============================================================================

// CreateTabWithCommand creates a new tab that runs a specific command.
// This uses a layout file to ensure the command runs correctly even when
// called from outside the zellij session.
// The paneName parameter sets the pane title (optional, empty string uses command as title).
func (s *session) CreateTabWithCommand(ctx context.Context, name, cwd, command string, args []string, paneName string) error {
	// Parse and render the tab layout template
	tmpl, err := template.New("tab").Parse(tabLayoutTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse tab layout template: %w", err)
	}

	data := TabLayoutData{
		TabName:  name,
		PaneName: paneName,
		Command:  command,
		Args:     args,
		Cwd:      cwd,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to render tab layout template: %w", err)
	}

	// Write layout to temp file
	tmpFile, err := os.CreateTemp("", "zellij-tab-*.kdl")
	if err != nil {
		return fmt.Errorf("failed to create temp layout file: %w", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	if _, err := tmpFile.WriteString(buf.String()); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to write tab layout file: %w", err)
	}
	_ = tmpFile.Close()

	// Create the tab using the layout
	cmdArgs := append(sessionArgs(s.name), "action", "new-tab", "--layout", tmpFile.Name())
	cmd := exec.CommandContext(ctx, "zellij", cmdArgs...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create tab with command: %w", err)
	}
	time.Sleep(s.TabCreateDelay)
	return nil
}

// SwitchToTab switches to a tab by name.
func (s *session) SwitchToTab(ctx context.Context, name string) error {
	args := append(sessionArgs(s.name), "action", "go-to-tab-name", name)
	cmd := exec.CommandContext(ctx, "zellij", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to switch to tab: %w", err)
	}
	return nil
}

// QueryTabNames returns a list of all tab names in the session.
func (s *session) QueryTabNames(ctx context.Context) ([]string, error) {
	args := append(sessionArgs(s.name), "action", "query-tab-names")
	cmd := exec.CommandContext(ctx, "zellij", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to query tab names: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var tabs []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			tabs = append(tabs, line)
		}
	}
	return tabs, nil
}

// TabExists checks if a tab with the given name exists in the session.
func (s *session) TabExists(ctx context.Context, name string) (bool, error) {
	tabs, err := s.QueryTabNames(ctx)
	if err != nil {
		// If we can't query tabs, assume it doesn't exist
		return false, nil
	}
	return slices.Contains(tabs, name), nil
}

// closeTab closes the current tab.
func (s *session) closeTab(ctx context.Context) error {
	args := append(sessionArgs(s.name), "action", "close-tab")
	cmd := exec.CommandContext(ctx, "zellij", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to close tab: %w", err)
	}
	return nil
}

// sendCtrlC sends Ctrl+C (interrupt signal) to the current pane.
func (s *session) sendCtrlC(ctx context.Context) error {
	args := append(sessionArgs(s.name), "action", "write", fmt.Sprintf("%d", ASCIICtrlC))
	cmd := exec.CommandContext(ctx, "zellij", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to send ctrl+c: %w", err)
	}
	return nil
}

// terminateProcess sends Ctrl+C twice with a delay to ensure process termination.
// This handles cases where the first Ctrl+C might be caught by a signal handler.
func (s *session) terminateProcess(ctx context.Context) error {
	if err := s.sendCtrlC(ctx); err != nil {
		return err
	}
	time.Sleep(s.CtrlCDelay)
	if err := s.sendCtrlC(ctx); err != nil {
		return err
	}
	time.Sleep(s.CtrlCDelay)
	return nil
}

// CloseTabByName switches to a tab by name and closes it without sending Ctrl+C.
// Closing a zellij tab inherently kills all processes running in it.
// This is safer than TerminateAndCloseTab because it doesn't send Ctrl+C
// via "zellij action write 3", which can hit the wrong pane if focus shifts.
func (s *session) CloseTabByName(ctx context.Context, tabName string) error {
	if err := s.SwitchToTab(ctx, tabName); err != nil {
		return fmt.Errorf("failed to switch to tab: %w", err)
	}

	if err := s.closeTab(ctx); err != nil {
		return fmt.Errorf("failed to close tab: %w", err)
	}

	return nil
}

// TerminateAndCloseTab terminates any running process in a tab and closes it.
// It first switches to the tab, sends Ctrl+C to terminate, then closes the tab.
func (s *session) TerminateAndCloseTab(ctx context.Context, tabName string) error {
	// Switch to the tab
	if err := s.SwitchToTab(ctx, tabName); err != nil {
		return fmt.Errorf("failed to switch to tab: %w", err)
	}

	// Terminate any running process
	if err := s.terminateProcess(ctx); err != nil {
		return fmt.Errorf("failed to terminate process: %w", err)
	}

	// Close the tab
	if err := s.closeTab(ctx); err != nil {
		return fmt.Errorf("failed to close tab: %w", err)
	}

	return nil
}
