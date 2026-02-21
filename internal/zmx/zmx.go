// Package zmx provides a client for interacting with the zmx terminal multiplexer.
// Each "tab" is a separate zmx session using the naming convention:
// sarge-<project>.<tabname>.
package zmx

//go:generate moq -stub -out zmx_mock.go . Client

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/sargehq/sarge/internal/logging"
)

// Client defines the interface for managing zmx sessions.
type Client interface {
	// SessionExists checks if a zmx session with the given name exists.
	SessionExists(ctx context.Context, name string) (bool, error)

	// RunSession creates a new detached zmx session running the given command.
	// The session runs in the background; use AttachSession for interactive sessions.
	RunSession(ctx context.Context, name, command string, args []string, cwd string) error

	// AttachSession opens a new terminal window with a zmx session.
	// If the session doesn't exist, zmx attach creates it with the given command.
	// The terminalCmdTemplate is the command template with {session} placeholder
	// (e.g. "ghostty -e zmx attach {session}").
	// The command/args are appended after the session name for create-if-needed.
	AttachSession(ctx context.Context, name string, terminalCmdTemplate string, command string, args []string, cwd string) error

	// KillSession kills a zmx session by name.
	// Returns an error if the session doesn't exist or the kill fails.
	KillSession(ctx context.Context, name string) error

	// ListSessions returns all zmx session names matching the given prefix.
	ListSessions(ctx context.Context, prefix string) ([]string, error)
}

// client implements the Client interface.
type client struct{}

// Compile-time check.
var _ Client = (*client)(nil)

// New creates a new zmx client.
func New() Client {
	return &client{}
}

// SessionName returns the zmx session name for a project and tab.
// Convention: sarge-<project>.<tabname>
// Spaces and parentheses are replaced with underscores to ensure the name
// is a single CLI token (zmx session names are passed as command-line arguments).
func SessionName(project, tab string) string {
	name := fmt.Sprintf("sarge-%s.%s", project, tab)
	return sanitizeSessionName(name)
}

// sanitizeSessionName replaces characters that are unsafe in CLI arguments
// (spaces, parentheses) with hyphens and collapses runs of hyphens.
func sanitizeSessionName(name string) string {
	r := strings.NewReplacer(" ", "-", "(", "", ")", "")
	s := r.Replace(name)
	// Collapse multiple consecutive hyphens into a single hyphen
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	// Trim trailing hyphens
	return strings.TrimRight(s, "-")
}

// shellQuote wraps a string in single quotes for safe shell interpolation.
// Single quotes inside the string are escaped as '\'' (end quote, escaped quote, start quote).
func shellQuote(s string) string {
	// If the string is simple (no special chars), return as-is
	safe := true
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '/' || c == ':' || c == '=') {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// ParseSessionName extracts the project and tab from a zmx session name.
// Returns empty strings if the name doesn't match the convention.
func ParseSessionName(name string) (project, tab string) {
	if !strings.HasPrefix(name, "sarge-") {
		return "", ""
	}
	rest := strings.TrimPrefix(name, "sarge-")
	parts := strings.SplitN(rest, ".", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

// listAllSessions returns all zmx session names.
func listAllSessions(ctx context.Context) []string {
	cmd := exec.CommandContext(ctx, "zmx", "list", "--short")
	output, err := cmd.Output()
	if err != nil {
		// zmx not running or no sessions
		return nil
	}

	raw := strings.TrimSpace(string(output))
	if raw == "" {
		return nil
	}

	lines := strings.Split(raw, "\n")
	var sessions []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			sessions = append(sessions, line)
		}
	}
	return sessions
}

// SessionExists checks if a zmx session with the given name exists.
// This performs a direct scan of the zmx list output without building
// an intermediate slice, making it more efficient for single lookups.
func (c *client) SessionExists(ctx context.Context, name string) (bool, error) {
	cmd := exec.CommandContext(ctx, "zmx", "list", "--short")
	output, err := cmd.Output()
	if err != nil {
		// zmx not running or no sessions
		logging.Debug("zmx SessionExists: list failed", "name", name, "error", err)
		return false, nil
	}

	// Scan line-by-line without allocating a full slice
	raw := strings.TrimSpace(string(output))
	if raw == "" {
		logging.Debug("zmx SessionExists: no sessions listed", "name", name)
		return false, nil
	}

	for _, line := range strings.Split(raw, "\n") {
		if strings.TrimSpace(line) == name {
			logging.Debug("zmx SessionExists: found", "name", name)
			return true, nil
		}
	}
	logging.Debug("zmx SessionExists: not found", "name", name, "sessions", raw)
	return false, nil
}

// RunSession creates a new detached zmx session running the given command.
// Uses: zmx run <name> <command> [args...]
func (c *client) RunSession(ctx context.Context, name, command string, args []string, cwd string) error {
	zmxArgs := []string{"run", name, command}
	zmxArgs = append(zmxArgs, args...)

	cmd := exec.CommandContext(ctx, "zmx", zmxArgs...) //nolint:gosec // G204: args are from trusted internal callers
	if cwd != "" {
		cmd.Dir = cwd
	}

	logging.Debug("zmx RunSession", "name", name, "command", command, "args", args, "cwd", cwd,
		"full_cmd", "zmx "+strings.Join(zmxArgs, " "))

	output, err := cmd.CombinedOutput()
	if err != nil {
		logging.Error("zmx RunSession failed", "name", name, "error", err, "output", strings.TrimSpace(string(output)))
		return fmt.Errorf("failed to run zmx session %q: %w: %s", name, err, strings.TrimSpace(string(output)))
	}
	logging.Debug("zmx RunSession succeeded", "name", name, "output", strings.TrimSpace(string(output)))
	return nil
}

// AttachSession opens a new terminal window with a zmx session.
// If the session doesn't exist, zmx attach creates it running the given command.
// The terminalCmdTemplate contains {session} which is replaced with the full
// "zmx attach <name> <command> [args...]" invocation.
// The terminal process is fire-and-forget.
func (c *client) AttachSession(ctx context.Context, name string, terminalCmdTemplate string, command string, args []string, cwd string) error {
	// Build the zmx attach command: "zmx attach <name> <command> [args...]"
	// If cwd is set and a command is provided, wrap in "cd <dir> && <command>"
	// so the session starts in the right directory (terminal emulators don't
	// reliably inherit the parent process's working directory).
	zmxAttachParts := []string{"zmx", "attach", shellQuote(name)}
	if command != "" {
		zmxAttachParts = append(zmxAttachParts, shellQuote(command))
		for _, arg := range args {
			zmxAttachParts = append(zmxAttachParts, shellQuote(arg))
		}
	}
	zmxAttachCmd := strings.Join(zmxAttachParts, " ")

	// Replace {session} in the template with the full zmx attach command
	// Template example: "ghostty -e zmx attach {session}"
	// We replace "zmx attach {session}" with our full command including args
	cmdStr := strings.ReplaceAll(terminalCmdTemplate, "zmx attach {session}", zmxAttachCmd)
	// Fallback: if template uses just {session} without "zmx attach" prefix
	if strings.Contains(cmdStr, "{session}") {
		cmdStr = strings.ReplaceAll(cmdStr, "{session}", name)
	}

	if cmdStr == "" {
		return fmt.Errorf("empty terminal command for session %q", name)
	}

	logging.Debug("zmx AttachSession", "name", name, "cmd", cmdStr, "cwd", cwd)

	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr) //nolint:gosec // Command comes from user config
	if cwd != "" {
		cmd.Dir = cwd
	}
	if err := cmd.Start(); err != nil {
		logging.Error("zmx AttachSession failed to start", "name", name, "error", err, "cmd", cmdStr)
		return fmt.Errorf("failed to launch terminal for session %q: %w", name, err)
	}
	go cmd.Wait() //nolint:errcheck // Fire-and-forget; prevent zombie process

	logging.Debug("zmx AttachSession launched", "name", name)
	return nil
}

// KillSession kills a zmx session by name.
// Uses: zmx kill <name>
func (c *client) KillSession(ctx context.Context, name string) error {
	logging.Debug("zmx KillSession", "name", name)
	cmd := exec.CommandContext(ctx, "zmx", "kill", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		logging.Error("zmx KillSession failed", "name", name, "error", err, "output", strings.TrimSpace(string(output)))
		return fmt.Errorf("failed to kill zmx session %q: %w: %s", name, err, strings.TrimSpace(string(output)))
	}
	logging.Debug("zmx KillSession succeeded", "name", name)
	return nil
}

// ListSessions returns all zmx session names matching the given prefix.
func (c *client) ListSessions(ctx context.Context, prefix string) ([]string, error) {
	sessions := listAllSessions(ctx)

	if prefix == "" {
		return sessions, nil
	}

	var filtered []string
	for _, s := range sessions {
		if strings.HasPrefix(s, prefix) {
			filtered = append(filtered, s)
		}
	}
	return filtered, nil
}
