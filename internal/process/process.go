// Package process provides cross-platform process detection utilities.
package process

//go:generate moq -stub -out process_mock.go . ProcessLister ProcessKiller

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

// ProcessLister provides an interface for listing processes.
type ProcessLister interface {
	GetProcessList(ctx context.Context) ([]string, error)
}

// ProcessKiller provides an interface for killing processes.
type ProcessKiller interface {
	KillByPattern(ctx context.Context, pattern string) error
}

// defaultLister is the default implementation using system commands.
var defaultLister ProcessLister = &systemProcessLister{}

// defaultKiller is the default implementation using system commands.
var defaultKiller ProcessKiller = &systemProcessKiller{}

// IsProcessRunningWith checks if a process is running using the provided lister.
func IsProcessRunningWith(ctx context.Context, pattern string, lister ProcessLister) (bool, error) {
	processes, err := lister.GetProcessList(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get process list: %w", err)
	}

	for _, proc := range processes {
		if strings.Contains(proc, pattern) {
			return true, nil
		}
	}

	return false, nil
}

// KillProcess kills all processes matching the given pattern.
// The pattern is matched against the full command line of the process.
func KillProcess(ctx context.Context, pattern string) error {
	return KillProcessWith(ctx, pattern, defaultLister, defaultKiller)
}

// KillProcessWith kills processes using the provided lister and killer.
func KillProcessWith(ctx context.Context, pattern string, lister ProcessLister, killer ProcessKiller) error {
	// Empty pattern would match all processes - this is dangerous and likely a bug
	if pattern == "" {
		return fmt.Errorf("empty pattern not allowed: would match all processes")
	}

	// First check if any process matches
	processes, err := lister.GetProcessList(ctx)
	if err != nil {
		return fmt.Errorf("failed to get process list: %w", err)
	}

	found := false
	for _, proc := range processes {
		if strings.Contains(proc, pattern) {
			found = true
			break
		}
	}

	if !found {
		// No process found, nothing to kill
		return nil
	}

	return killer.KillByPattern(ctx, pattern)
}

// StartDetached starts a command as a detached process that will continue running
// after this process exits. The process runs in its own session.
func StartDetached(_ context.Context, command string, args ...string) error {
	cmd := exec.Command(command, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // Create a new session, detaching from terminal
	}
	return cmd.Start()
}
