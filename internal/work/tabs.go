// Package work provides work unit management and tab operations.
package work

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/sargehq/sarge/internal/agents"
	"github.com/sargehq/sarge/internal/agents/types"
	"github.com/sargehq/sarge/internal/logging"
	"github.com/sargehq/sarge/internal/project"
	"github.com/sargehq/sarge/internal/ptysession"
)

// PlanTabName returns the tab name for a bean's planning session.
func PlanTabName(beanID string) string {
	return fmt.Sprintf("plan-%s", beanID)
}

// OpenConsole opens a shell session in the work's worktree.
// The hooksEnv parameter contains environment variables to export (format: "KEY=value").
// Progress messages are written to the provided writer. Pass io.Discard to suppress output.
func (m *DefaultOrchestratorManager) OpenConsole(ctx context.Context, workID string, projectName string, workDir string, friendlyName string, hooksEnv []string, w io.Writer) error {
	if m.ptyManager == nil {
		return fmt.Errorf("PTY manager not configured: console sessions require PTY support")
	}

	sessionID := fmt.Sprintf("console-%s", workID)

	// Check if session already exists and is alive
	if existing := m.ptyManager.Get(sessionID); existing != nil && existing.State() != ptysession.SessionDead {
		fmt.Fprintf(w, "Console session %s already exists\n", sessionID)
		return nil
	}

	// Build session config — run pi normally (not RPC mode)
	cfg := m.buildPTYConfig(workDir, hooksEnv)

	// Spawn the PTY session
	_, err := m.ptyManager.Spawn(sessionID, cfg)
	if err != nil {
		return fmt.Errorf("failed to spawn PTY console session: %w", err)
	}

	logging.Debug("OpenConsole completed", "workID", workID, "sessionID", sessionID)
	fmt.Fprintf(w, "Created PTY console session: %s\n", sessionID)
	return nil
}

// OpenAgentSession creates an interactive PTY session in the work's worktree.
// Returns the PTY session for display in the TUI.
func (m *DefaultOrchestratorManager) OpenAgentSession(ctx context.Context, workID string, projectName string, workDir string, friendlyName string, hooksEnv []string, cfg *project.Config, w io.Writer) (*ptysession.Session, error) {
	if m.ptyManager == nil {
		return nil, fmt.Errorf("PTY manager not configured: agent sessions require PTY support")
	}

	sessionID := fmt.Sprintf("agent-%s", workID)

	// Check if session already exists and is alive
	if existing := m.ptyManager.Get(sessionID); existing != nil && existing.State() != ptysession.SessionDead {
		fmt.Fprintf(w, "Agent session %s already exists\n", sessionID)
		return existing, nil
	}

	// Build session config — run pi normally (not RPC mode)
	ptyCfg := m.buildPTYConfig(workDir, hooksEnv)

	// Spawn the PTY session
	session, err := m.ptyManager.Spawn(sessionID, ptyCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to spawn PTY agent session: %w", err)
	}

	logging.Debug("OpenAgentSession completed", "workID", workID, "sessionID", sessionID)
	fmt.Fprintf(w, "Created PTY agent session: %s\n", sessionID)
	return session, nil
}

// SpawnPlanSession creates a PTY session for a bean and sends the plan prompt.
// Returns the PTY session for display in the TUI.
func (m *DefaultOrchestratorManager) SpawnPlanSession(ctx context.Context, beanID string, projectName string, mainRepoPath string, w io.Writer) (*ptysession.Session, error) {
	if m.ptyManager == nil {
		return nil, fmt.Errorf("PTY manager not configured: plan sessions require PTY support")
	}

	sessionID := PlanTabName(beanID)

	// Kill existing session if present
	if existing := m.ptyManager.Get(sessionID); existing != nil && existing.State() != ptysession.SessionDead {
		_ = m.ptyManager.Kill(sessionID)
		fmt.Fprintf(w, "Killed existing plan session: %s\n", sessionID)
	}

	// Build the plan prompt
	agent, err := agents.NewAgent(m.projCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent for prompt: %w", err)
	}

	prompt, err := agent.BuildPrompt(types.TaskParams{
		Type:      types.TaskTypePlan,
		BeanID:    beanID,
		BeansPath: m.beansPath,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build plan prompt: %w", err)
	}

	// Build session config — run pi normally with the prompt piped to stdin
	env := []string{}
	if m.beansPath != "" {
		env = append(env, "BEANS_PATH="+m.beansPath)
	}
	ptyCfg := m.buildPTYConfig(mainRepoPath, env)
	ptyCfg.InitialPrompt = prompt

	// Spawn the PTY session
	session, err := m.ptyManager.Spawn(sessionID, ptyCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to spawn PTY plan session: %w", err)
	}

	logging.Debug("SpawnPlanSession completed", "beanID", beanID, "sessionID", sessionID)
	fmt.Fprintf(w, "Created PTY plan session: %s\n", sessionID)
	return session, nil
}

// buildPTYConfig creates a ptysession.SessionConfig from the project config.
func (m *DefaultOrchestratorManager) buildPTYConfig(workDir string, extraEnv []string) ptysession.SessionConfig {
	cfg := ptysession.SessionConfig{
		Args:    []string{"--no-session"},
		WorkDir: workDir,
		Width:   80,
		Height:  24,
	}

	if m.projCfg != nil {
		if m.projCfg.Pi.Provider != "" {
			cfg.Args = append(cfg.Args, "--provider", m.projCfg.Pi.Provider)
		}
		if m.projCfg.Pi.Model != "" {
			cfg.Args = append(cfg.Args, "--model", m.projCfg.Pi.Model)
		}
		if m.projCfg.Pi.Thinking != "" {
			cfg.Args = append(cfg.Args, "--thinking", m.projCfg.Pi.Thinking)
		}
	}

	// Inherit parent env and add extras
	cfg.Env = append(cfg.Env, os.Environ()...)
	cfg.Env = append(cfg.Env, extraEnv...)

	return cfg
}
