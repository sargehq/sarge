// Package work provides work unit management and tab operations.
package work

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sargehq/sarge/internal/project"
)

// PlanTabName returns the zellij tab name for a bead's planning session.
func PlanTabName(beadID string) string {
	return fmt.Sprintf("plan-%s", beadID)
}

// OpenConsole creates a zellij tab with a shell in the work's worktree.
// The tab is named "console-<work-id>" or "console-<work-id> (friendlyName)" for easy identification.
// The hooksEnv parameter contains environment variables to export (format: "KEY=value").
// Progress messages are written to the provided writer. Pass io.Discard to suppress output.
//
// IMPORTANT: The zellij session must already exist before calling this function.
// Callers should use control.EnsureControlPlane to ensure
// the session exists with the control plane running.
func (m *DefaultOrchestratorManager) OpenConsole(ctx context.Context, workID string, projectName string, workDir string, friendlyName string, hooksEnv []string, w io.Writer) error {
	sessionName := project.SessionNameForProject(projectName)
	tabName := project.FormatTabName("console", workID, friendlyName)

	// Verify session exists - callers must initialize it with control plane
	exists, err := m.zellij.SessionExists(ctx, sessionName)
	if err != nil {
		return fmt.Errorf("failed to check session existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("zellij session %s does not exist - call control.EnsureControlPlane first", sessionName)
	}

	// Check if tab already exists
	session := m.zellij.Session(sessionName)
	tabExists, _ := session.TabExists(ctx, tabName)
	if tabExists {
		fmt.Fprintf(w, "Console tab %s already exists\n", tabName)
		return nil
	}

	// Build shell command with exports if needed
	// Use user's preferred shell from $SHELL, default to bash
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "bash"
	}
	shellName := filepath.Base(shell)

	var command string
	var args []string
	if len(hooksEnv) > 0 {
		var exports []string
		for _, env := range hooksEnv {
			exports = append(exports, fmt.Sprintf("export %s", env))
		}
		// Use shell -c to export vars and then exec shell for interactive shell
		shellCmd := fmt.Sprintf("%s && exec %s", strings.Join(exports, " && "), shell)
		command = shell
		args = []string{"-c", shellCmd}
	} else {
		command = shell
		args = nil
	}

	// Create tab with shell using layout approach
	fmt.Fprintf(w, "Creating console tab: %s in session %s\n", tabName, sessionName)
	if err := session.CreateTabWithCommand(ctx, tabName, workDir, command, args, shellName); err != nil {
		return fmt.Errorf("failed to create tab: %w", err)
	}

	fmt.Fprintf(w, "Console opened in zellij session %s, tab %s\n", sessionName, tabName)
	return nil
}

// OpenClaudeSession creates a zellij tab with an interactive Claude Code session in the work's worktree.
// The tab is named "claude-<work-id>" or "claude-<work-id> (friendlyName)" for easy identification.
// The hooksEnv parameter contains environment variables to export (format: "KEY=value").
// The config parameter controls Claude settings like --dangerously-skip-permissions.
// Progress messages are written to the provided writer. Pass io.Discard to suppress output.
//
// IMPORTANT: The zellij session must already exist before calling this function.
// Callers should use control.EnsureControlPlane to ensure
// the session exists with the control plane running.
func (m *DefaultOrchestratorManager) OpenClaudeSession(ctx context.Context, workID string, projectName string, workDir string, friendlyName string, hooksEnv []string, cfg *project.Config, w io.Writer) error {
	sessionName := project.SessionNameForProject(projectName)
	tabName := project.FormatTabName("claude", workID, friendlyName)

	// Verify session exists - callers must initialize it with control plane
	exists, err := m.zellij.SessionExists(ctx, sessionName)
	if err != nil {
		return fmt.Errorf("failed to check session existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("zellij session %s does not exist - call control.EnsureControlPlane first", sessionName)
	}

	// Check if tab already exists
	session := m.zellij.Session(sessionName)
	tabExists, _ := session.TabExists(ctx, tabName)
	if tabExists {
		fmt.Fprintf(w, "Claude session tab %s already exists\n", tabName)
		return nil
	}

	// Build the claude command with exports if needed
	var claudeArgs []string
	if cfg != nil && cfg.Claude.ShouldSkipPermissions() {
		claudeArgs = []string{"--dangerously-skip-permissions"}
	}

	// If we have environment variables, use bash -c to export them
	var command string
	var args []string
	if len(hooksEnv) > 0 {
		var exports []string
		for _, env := range hooksEnv {
			exports = append(exports, fmt.Sprintf("export %s", env))
		}
		claudeCmd := "claude"
		if len(claudeArgs) > 0 {
			claudeCmd = "claude " + strings.Join(claudeArgs, " ")
		}
		shellCmd := fmt.Sprintf("%s && %s", strings.Join(exports, " && "), claudeCmd)
		command = "bash"
		args = []string{"-c", shellCmd}
	} else {
		command = "claude"
		args = claudeArgs
	}

	// Create tab with command using layout approach
	fmt.Fprintf(w, "Creating Claude session tab: %s in session %s\n", tabName, sessionName)
	if err := session.CreateTabWithCommand(ctx, tabName, workDir, command, args, "claude"); err != nil {
		return fmt.Errorf("failed to create tab: %w", err)
	}

	fmt.Fprintf(w, "Claude session opened in zellij session %s, tab %s\n", sessionName, tabName)
	return nil
}

// SpawnPlanSession creates a zellij tab and runs the plan command for a bead.
// The tab is named "plan-<bead-id>" for easy identification.
// The function returns immediately after spawning - the plan session runs in the tab.
// Progress messages are written to the provided writer. Pass io.Discard to suppress output.
//
// IMPORTANT: The zellij session must already exist before calling this function.
// Callers should use control.EnsureControlPlane to ensure
// the session exists with the control plane running.
func (m *DefaultOrchestratorManager) SpawnPlanSession(ctx context.Context, beadID string, projectName string, mainRepoPath string, w io.Writer) error {
	sessionName := project.SessionNameForProject(projectName)
	tabName := PlanTabName(beadID)

	// Verify session exists - callers must initialize it with control plane
	exists, err := m.zellij.SessionExists(ctx, sessionName)
	if err != nil {
		return fmt.Errorf("failed to check session existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("zellij session %s does not exist - call control.EnsureControlPlane first", sessionName)
	}

	// Check if tab already exists
	session := m.zellij.Session(sessionName)
	tabExists, _ := session.TabExists(ctx, tabName)
	if tabExists {
		fmt.Fprintf(w, "Tab %s already exists, terminating and recreating...\n", tabName)

		// Terminate and close the existing tab
		if err := session.TerminateAndCloseTab(ctx, tabName); err != nil {
			fmt.Fprintf(w, "Warning: failed to terminate existing tab: %v\n", err)
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Create a new tab with the plan command using a layout
	fmt.Fprintf(w, "Creating tab: %s in session %s\n", tabName, sessionName)
	if err := session.CreateTabWithCommand(ctx, tabName, mainRepoPath, "co", []string{"plan", beadID}, "planning"); err != nil {
		return fmt.Errorf("failed to create tab: %w", err)
	}

	fmt.Fprintf(w, "Plan session spawned in zellij session %s, tab %s\n", sessionName, tabName)
	return nil
}
