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

// shellQuoteEnv quotes the value portion of a KEY=value env string for safe shell use.
// e.g. "FOO=hello world" becomes "FOO='hello world'"
// Returns an error if the format is invalid or the key contains invalid characters.
func shellQuoteEnv(env string) (string, error) {
	i := strings.IndexByte(env, '=')
	if i < 0 {
		return "", fmt.Errorf("invalid env var %q: missing '='", env)
	}
	key := env[:i]
	if key == "" {
		return "", fmt.Errorf("invalid env var %q: empty key", env)
	}
	for _, c := range key {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' {
			return "", fmt.Errorf("invalid env var key %q: contains invalid character %q", key, c)
		}
	}
	val := env[i+1:]
	// Single-quote the value, escaping any embedded single quotes
	val = "'" + strings.ReplaceAll(val, "'", "'\\''") + "'"
	return key + "=" + val, nil
}

// buildZmxExportCommand builds "export K='V' K2='V2'" for persisting env in a shell session.
// Returns "" if no hooksEnv.
func buildZmxExportCommand(hooksEnv []string) (string, error) {
	if len(hooksEnv) == 0 {
		return "", nil
	}
	var parts []string
	for _, env := range hooksEnv {
		quoted, err := shellQuoteEnv(env)
		if err != nil {
			return "", err
		}
		parts = append(parts, quoted)
	}
	return "export " + strings.Join(parts, " "), nil
}

// buildZmxEnvPrefix builds quoted "K=V K2=V2 " prefix from hooksEnv, or "" if none.
// Used for "FOO=bar command" syntax where env is set for a single command.
func buildZmxEnvPrefix(hooksEnv []string) (string, error) {
	if len(hooksEnv) == 0 {
		return "", nil
	}
	var parts []string
	for _, env := range hooksEnv {
		quoted, err := shellQuoteEnv(env)
		if err != nil {
			return "", err
		}
		parts = append(parts, quoted)
	}
	return strings.Join(parts, " ") + " ", nil
}

// buildShellCommand builds the shell command and args for a console session.
func buildShellCommand(hooksEnv []string) (command string, args []string, shellName string, err error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "bash"
	}
	shellName = filepath.Base(shell)

	if len(hooksEnv) > 0 {
		var exports []string
		for _, env := range hooksEnv {
			quoted, qerr := shellQuoteEnv(env)
			if qerr != nil {
				return "", nil, "", qerr
			}
			exports = append(exports, "export "+quoted)
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

// openConsoleZmx opens a terminal window with a zmx shell session in the work's worktree.
func (m *DefaultOrchestratorManager) openConsoleZmx(ctx context.Context, workID string, projectName string, workDir string, friendlyName string, hooksEnv []string, w io.Writer) error {
	tabName := project.FormatTabName("console", workID, friendlyName)
	zmxName := zmx.SessionName(projectName, tabName)

	// Create session if it doesn't exist
	exists, _ := m.zmx.SessionExists(ctx, zmxName)
	if !exists {
		fmt.Fprintf(w, "Creating console session: %s\n", zmxName)
		initCmd, err := buildZmxExportCommand(hooksEnv)
		if err != nil {
			return fmt.Errorf("failed to build env command: %w", err)
		}
		if err := m.zmx.RunSession(ctx, zmxName, initCmd, nil, workDir); err != nil {
			return fmt.Errorf("failed to create console session: %w", err)
		}
	} else {
		// Session exists — check if it already has a terminal attached
		hasClients, _ := m.zmx.SessionHasClients(ctx, zmxName)
		if hasClients {
			fmt.Fprintf(w, "Console session %s already open\n", zmxName)
			return nil
		}
	}

	// Open terminal window attached to the session
	fmt.Fprintf(w, "Attaching to console: %s\n", zmxName)
	if err := m.attachZmxSession(ctx, zmxName); err != nil {
		return fmt.Errorf("failed to attach to console: %w", err)
	}
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

	command, args, shellName, err := buildShellCommand(hooksEnv)
	if err != nil {
		return fmt.Errorf("failed to build shell command: %w", err)
	}

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
func buildAgentCommand(agentType string, hooksEnv []string, cfg *project.Config) (command string, args []string, err error) {
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
			quoted, qerr := shellQuoteEnv(env)
			if qerr != nil {
				return "", nil, qerr
			}
			exports = append(exports, "export "+quoted)
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

// openAgentSessionZmx opens a terminal window with an interactive agent zmx session.
func (m *DefaultOrchestratorManager) openAgentSessionZmx(ctx context.Context, workID string, projectName string, workDir string, friendlyName string, hooksEnv []string, cfg *project.Config, w io.Writer) error {
	agentType := "claude"
	if cfg != nil && cfg.Agent.Type != "" {
		agentType = cfg.Agent.Type
	}

	tabName := project.FormatTabName("agent", workID, friendlyName)
	zmxName := zmx.SessionName(projectName, tabName)

	// Create session if it doesn't exist
	exists, _ := m.zmx.SessionExists(ctx, zmxName)
	if !exists {
		fmt.Fprintf(w, "Creating %s session: %s\n", agentType, zmxName)
		// Build "FOO='bar' BAZ='qux' claude [args...]"
		envPrefix, err := buildZmxEnvPrefix(hooksEnv)
		if err != nil {
			return fmt.Errorf("failed to build env prefix: %w", err)
		}
		command, args, err := buildAgentCommand(agentType, nil, cfg)
		if err != nil {
			return fmt.Errorf("failed to build agent command: %w", err)
		}
		agentCmd := envPrefix + command
		for _, a := range args {
			agentCmd += " " + a
		}
		if err := m.zmx.RunSession(ctx, zmxName, agentCmd, nil, workDir); err != nil {
			return fmt.Errorf("failed to create %s session: %w", agentType, err)
		}
	} else {
		// Session exists — check if it already has a terminal attached
		hasClients, _ := m.zmx.SessionHasClients(ctx, zmxName)
		if hasClients {
			fmt.Fprintf(w, "Agent session %s already open\n", zmxName)
			return nil
		}
	}

	// Open terminal window attached to the session
	fmt.Fprintf(w, "Attaching to %s: %s\n", agentType, zmxName)
	if err := m.attachZmxSession(ctx, zmxName); err != nil {
		return fmt.Errorf("failed to attach to %s session: %w", agentType, err)
	}
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
	tabName := project.FormatTabName("agent", workID, friendlyName)

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

	command, args, err := buildAgentCommand(agentType, hooksEnv, cfg)
	if err != nil {
		return fmt.Errorf("failed to build agent command: %w", err)
	}

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

// spawnPlanSessionZmx opens a terminal window with a zmx plan session.
func (m *DefaultOrchestratorManager) spawnPlanSessionZmx(ctx context.Context, beadID string, projectName string, mainRepoPath string, w io.Writer) error {
	tabName := PlanTabName(beadID)
	zmxName := zmx.SessionName(projectName, tabName)

	// Kill existing session if present
	exists, _ := m.zmx.SessionExists(ctx, zmxName)
	if exists {
		_ = m.zmx.KillSession(ctx, zmxName)
		fmt.Fprintf(w, "Session %s already existed, killed and recreating...\n", zmxName)
	}

	// Create session with plan command
	fmt.Fprintf(w, "Creating plan session: %s\n", zmxName)
	if err := m.zmx.RunSession(ctx, zmxName, "sarge", []string{"plan", beadID}, mainRepoPath); err != nil {
		return fmt.Errorf("failed to create plan session: %w", err)
	}

	// Open terminal window attached to the session
	if err := m.attachZmxSession(ctx, zmxName); err != nil {
		return fmt.Errorf("failed to attach to plan session: %w", err)
	}
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
