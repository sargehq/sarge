// Package zmx provides a client for interacting with the zmx terminal multiplexer.
// Each "tab" is a separate zmx session using the naming convention:
// sarge-<project>--<tabname> (double-dash separates project from tab name).
package zmx

//go:generate moq -stub -out zmx_mock.go . Client

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Client defines the interface for managing zmx sessions.
type Client interface {
	// SessionExists checks if a zmx session with the given name exists.
	SessionExists(ctx context.Context, name string) (bool, error)

	// RunSession creates a new detached zmx session running the given command.
	RunSession(ctx context.Context, name, command string, args []string, cwd string) error

	// KillSession kills a zmx session by name.
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
// Convention: sarge-<project>--<tabname>
func SessionName(project, tab string) string {
	return fmt.Sprintf("sarge-%s--%s", project, tab)
}

// ParseSessionName extracts the project and tab from a zmx session name.
// Returns empty strings if the name doesn't match the convention.
func ParseSessionName(name string) (project, tab string) {
	if !strings.HasPrefix(name, "sarge-") {
		return "", ""
	}
	rest := strings.TrimPrefix(name, "sarge-")
	parts := strings.SplitN(rest, "--", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

// listAllSessions returns all zmx session names.
func listAllSessions(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "zmx", "list", "--short")
	output, err := cmd.Output()
	if err != nil {
		// zmx not running or no sessions
		return nil, nil
	}

	raw := strings.TrimSpace(string(output))
	if raw == "" {
		return nil, nil
	}

	lines := strings.Split(raw, "\n")
	var sessions []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			sessions = append(sessions, line)
		}
	}
	return sessions, nil
}

// SessionExists checks if a zmx session with the given name exists.
func (c *client) SessionExists(ctx context.Context, name string) (bool, error) {
	sessions, err := listAllSessions(ctx)
	if err != nil {
		return false, err
	}
	for _, s := range sessions {
		if s == name {
			return true, nil
		}
	}
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

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to run zmx session %q: %w: %s", name, err, strings.TrimSpace(string(output)))
	}
	return nil
}

// KillSession kills a zmx session by name.
// Uses: zmx kill <name>
func (c *client) KillSession(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "zmx", "kill", name)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to kill zmx session %q: %w: %s", name, err, strings.TrimSpace(string(output)))
	}
	return nil
}

// ListSessions returns all zmx session names matching the given prefix.
func (c *client) ListSessions(ctx context.Context, prefix string) ([]string, error) {
	sessions, err := listAllSessions(ctx)
	if err != nil {
		return nil, err
	}

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
