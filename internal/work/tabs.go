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
	"github.com/sargehq/sarge/internal/zmx"
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
	if m.isZmx() {
		return m.openConsoleZmx(ctx, workID, projectName, workDir, friendlyName, hooksEnv, w)
	}
	return m.openConsoleZellij(ctx, workID, projectName, workDir, friendlyName, hooksEnv, w)
}

// buildShellCommand builds the shell command and args for a console session.
func buildShellCommand(hooksEnv []string) (command string, args []string, shellName string) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "bash"
	}
	shellName = filepath.Base(shell)

	if len(hooksEnv) > 0 {
		var exports []string
		for _, env := range hooksEnv {
			exports = append(exports, fmt.Sprintf("export %s", env))
		}
		shellCmd := fmt.Sprintf("%s && exec %s", strings.Join(exports, " && "), shell)
		command = shell
		args = []string{"-c", shellCmd}
	} else {
		command = shell
		args = nil
	}
	return
}

// openConsoleZmx creates a zmx session with a shell in the work's worktree.
func (m *DefaultOrchestratorManager) openConsoleZmx(ctx context.Context, workID string, projectName string, workDir string, friendlyName string, hooksEnv []string, w io.Writer) error {
	tabName := project.FormatTabName("console", workID, friendlyName)
	zmxName := zmx.SessionName(projectName, tabName)

	// Check if session already exists
	exists, _ := m.zmx.SessionExists(ctx, zmxName)
	if exists {
		fmt.Fprintf(w, "Console session %s already exists\n", zmxName)
		return nil
	}

	command, args, _ := buildShellCommand(hooksEnv)

	fmt.Fprintf(w, "Creating console zmx session: %s\n", zmxName)
	if err := m.zmx.RunSession(ctx, zmxName, command, args, workDir); err != nil {
		return fmt.Errorf("failed to create zmx session: %w", err)
	}

	fmt.Fprintf(w, "Console opened in zmx session %s\n", zmxName)
	return nil
}

// openConsoleZellij creates a zellij tab with a shell in the work's worktree.
func (m *DefaultOrchestratorManager) openConsoleZellij(ctx context.Context, workID string, projectName string, workDir string, friendlyName string, hooksEnv []string, w io.Writer) error {
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

	command, args, shellName := buildShellCommand(hooksEnv)

	// Create tab with shell using layout approach
	fmt.Fprintf(w, "Creating console tab: %s in session %s\n", tabName, sessionName)
	if err := session.CreateTabWithCommand(ctx, tabName, workDir, command, args, shellName); err != nil {
		return fmt.Errorf("failed to create tab: %w", err)
	}

	fmt.Fprintf(w, "Console opened in zellij session %s, tab %s\n", sessionName, tabName)
	return nil
}

// OpenAgentSession creates a zellij tab with an interactive agent session in the work's worktree.
// The tab is named "<agent>-<work-id>" or "<agent>-<work-id> (friendlyName)" for easy identification.
// The agent type is determined by cfg.Agent.Type (defaults to "claude").
// The hooksEnv parameter contains environment variables to export (format: "KEY=value").
// The config parameter controls agent settings like --dangerously-skip-permissions.
// Progress messages are written to the provided writer. Pass io.Discard to suppress output.
//
// IMPORTANT: The zellij session must already exist before calling this function.
// Callers should use control.EnsureControlPlane to ensure
// the session exists with the control plane running.
func (m *DefaultOrchestratorManager) OpenAgentSession(ctx context.Context, workID string, projectName string, workDir string, friendlyName string, hooksEnv []string, cfg *project.Config, w io.Writer) error {
	if m.isZmx() {
		return m.openAgentSessionZmx(ctx, workID, projectName, workDir, friendlyName, hooksEnv, cfg, w)
	}
	return m.openAgentSessionZellij(ctx, workID, projectName, workDir, friendlyName, hooksEnv, cfg, w)
}

// buildAgentCommand builds the agent command and args based on config.
func buildAgentCommand(agentType string, hooksEnv []string, cfg *project.Config) (command string, args []string) {
	var agentBinary string
	var agentArgs []string
	switch agentType {
	case "pi":
		agentBinary = "pi"
		if cfg != nil {
			if cfg.Pi.Provider != "" {
				agentArgs = append(agentArgs, "--provider", cfg.Pi.Provider)
			}
			if cfg.Pi.Model != "" {
				agentArgs = append(agentArgs, "--model", cfg.Pi.Model)
			}
			if cfg.Pi.Thinking != "" {
				agentArgs = append(agentArgs, "--thinking", cfg.Pi.Thinking)
			}
		}
	default: // "claude"
		agentBinary = "claude"
		if cfg != nil && cfg.Claude.ShouldSkipPermissions() {
			agentArgs = []string{"--dangerously-skip-permissions"}
		}
	}

	if len(hooksEnv) > 0 {
		var exports []string
		for _, env := range hooksEnv {
			exports = append(exports, fmt.Sprintf("export %s", env))
		}
		agentCmd := agentBinary
		if len(agentArgs) > 0 {
			agentCmd = agentBinary + " " + strings.Join(agentArgs, " ")
		}
		shellCmd := fmt.Sprintf("%s && %s", strings.Join(exports, " && "), agentCmd)
		command = "bash"
		args = []string{"-c", shellCmd}
	} else {
		command = agentBinary
		args = agentArgs
	}
	return
}

// openAgentSessionZmx creates a zmx session with an interactive agent.
func (m *DefaultOrchestratorManager) openAgentSessionZmx(ctx context.Context, workID string, projectName string, workDir string, friendlyName string, hooksEnv []string, cfg *project.Config, w io.Writer) error {
	agentType := "claude"
	if cfg != nil && cfg.Agent.Type != "" {
		agentType = cfg.Agent.Type
	}

	tabName := project.FormatTabName(agentType, workID, friendlyName)
	zmxName := zmx.SessionName(projectName, tabName)

	// Check if session already exists
	exists, _ := m.zmx.SessionExists(ctx, zmxName)
	if exists {
		fmt.Fprintf(w, "Agent session %s already exists\n", zmxName)
		return nil
	}

	command, args := buildAgentCommand(agentType, hooksEnv, cfg)

	fmt.Fprintf(w, "Creating %s zmx session: %s\n", agentType, zmxName)
	if err := m.zmx.RunSession(ctx, zmxName, command, args, workDir); err != nil {
		return fmt.Errorf("failed to create zmx session: %w", err)
	}

	fmt.Fprintf(w, "%s session opened in zmx session %s\n", agentType, zmxName)
	return nil
}

// openAgentSessionZellij creates a zellij tab with an interactive agent session.
func (m *DefaultOrchestratorManager) openAgentSessionZellij(ctx context.Context, workID string, projectName string, workDir string, friendlyName string, hooksEnv []string, cfg *project.Config, w io.Writer) error {
	// Determine agent type from config (default to "claude")
	agentType := "claude"
	if cfg != nil && cfg.Agent.Type != "" {
		agentType = cfg.Agent.Type
	}

	sessionName := project.SessionNameForProject(projectName)
	tabName := project.FormatTabName(agentType, workID, friendlyName)

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
		fmt.Fprintf(w, "Agent session tab %s already exists\n", tabName)
		return nil
	}

	command, args := buildAgentCommand(agentType, hooksEnv, cfg)

	// Create tab with command using layout approach
	fmt.Fprintf(w, "Creating %s session tab: %s in session %s\n", agentType, tabName, sessionName)
	if err := session.CreateTabWithCommand(ctx, tabName, workDir, command, args, agentType); err != nil {
		return fmt.Errorf("failed to create tab: %w", err)
	}

	fmt.Fprintf(w, "%s session opened in zellij session %s, tab %s\n", agentType, sessionName, tabName)
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
	if m.isZmx() {
		return m.spawnPlanSessionZmx(ctx, beadID, projectName, mainRepoPath, w)
	}
	return m.spawnPlanSessionZellij(ctx, beadID, projectName, mainRepoPath, w)
}

// spawnPlanSessionZmx creates a zmx session running the plan command.
func (m *DefaultOrchestratorManager) spawnPlanSessionZmx(ctx context.Context, beadID string, projectName string, mainRepoPath string, w io.Writer) error {
	tabName := PlanTabName(beadID)
	zmxName := zmx.SessionName(projectName, tabName)

	// Kill existing session if present (kill is a no-op if session doesn't exist,
	// avoiding a redundant SessionExists call)
	if err := m.zmx.KillSession(ctx, zmxName); err == nil {
		fmt.Fprintf(w, "Session %s already existed, killed and recreating...\n", zmxName)
	}

	// Create a new zmx session with the plan command
	fmt.Fprintf(w, "Creating zmx session: %s\n", zmxName)
	if err := m.zmx.RunSession(ctx, zmxName, "sarge", []string{"plan", beadID}, mainRepoPath); err != nil {
		return fmt.Errorf("failed to create zmx session: %w", err)
	}

	fmt.Fprintf(w, "Plan session spawned in zmx session %s\n", zmxName)
	return nil
}

// spawnPlanSessionZellij creates a zellij tab running the plan command.
func (m *DefaultOrchestratorManager) spawnPlanSessionZellij(ctx context.Context, beadID string, projectName string, mainRepoPath string, w io.Writer) error {
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
	if err := session.CreateTabWithCommand(ctx, tabName, mainRepoPath, "sarge", []string{"plan", beadID}, "planning"); err != nil {
		return fmt.Errorf("failed to create tab: %w", err)
	}

	fmt.Fprintf(w, "Plan session spawned in zellij session %s, tab %s\n", sessionName, tabName)
	return nil
}
