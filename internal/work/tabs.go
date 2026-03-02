// Package work provides work unit management and tab operations.
package work

import (
	"context"
	"fmt"
	"io"

	"github.com/sargehq/sarge/internal/agents"
	"github.com/sargehq/sarge/internal/agents/types"
	"github.com/sargehq/sarge/internal/bridge"
	"github.com/sargehq/sarge/internal/logging"
	"github.com/sargehq/sarge/internal/project"
)

// PlanTabName returns the tab name for a bean's planning session.
func PlanTabName(beanID string) string {
	return fmt.Sprintf("plan-%s", beanID)
}

// OpenConsole opens a shell session in the work's worktree.
// The hooksEnv parameter contains environment variables to export (format: "KEY=value").
// Progress messages are written to the provided writer. Pass io.Discard to suppress output.
func (m *DefaultOrchestratorManager) OpenConsole(ctx context.Context, workID string, projectName string, workDir string, friendlyName string, hooksEnv []string, w io.Writer) error {
	if m.bridge != nil {
		return m.openConsoleBridge(ctx, workID, workDir, hooksEnv, w)
	}
	// Without a bridge, console opening is not supported
	return fmt.Errorf("bridge not configured: console sessions require bridge architecture")
}

// openConsoleBridge opens a console via the bridge.
func (m *DefaultOrchestratorManager) openConsoleBridge(ctx context.Context, workID string, workDir string, hooksEnv []string, w io.Writer) error {
	sessionID := fmt.Sprintf("console-%s", workID)

	// Check if session already exists and is alive
	if existing := m.bridge.GetSession(sessionID); existing != nil && existing.State() != bridge.SessionDead {
		fmt.Fprintf(w, "Console session %s already exists\n", sessionID)
		return nil
	}

	// Build session config
	sessionCfg := bridge.SessionConfigFromProject(workDir, m.projCfg)
	sessionCfg.Env = append(sessionCfg.Env, hooksEnv...)

	// Spawn the bridge session
	_, err := m.bridge.SpawnSession(sessionID, sessionCfg)
	if err != nil {
		return fmt.Errorf("failed to spawn bridge console session: %w", err)
	}

	logging.Debug("openConsoleBridge completed", "workID", workID, "sessionID", sessionID)
	fmt.Fprintf(w, "Created bridge console session: %s\n", sessionID)
	return nil
}

// OpenAgentSession creates an interactive agent session in the work's worktree.
// When a bridge is configured, spawns a pi RPC session (user interacts via TUI).
// Returns the bridge session (non-nil only when using bridge mode).
func (m *DefaultOrchestratorManager) OpenAgentSession(ctx context.Context, workID string, projectName string, workDir string, friendlyName string, hooksEnv []string, cfg *project.Config, w io.Writer) (*bridge.Session, error) {
	if m.bridge != nil {
		return m.openAgentSessionBridge(ctx, workID, workDir, hooksEnv, cfg, w)
	}
	return nil, fmt.Errorf("bridge not configured: agent sessions require bridge architecture")
}

// openAgentSessionBridge creates a bridge session for interactive agent use.
func (m *DefaultOrchestratorManager) openAgentSessionBridge(ctx context.Context, workID string, workDir string, hooksEnv []string, cfg *project.Config, w io.Writer) (*bridge.Session, error) {
	sessionID := fmt.Sprintf("agent-%s", workID)

	// Check if session already exists and is alive
	if existing := m.bridge.GetSession(sessionID); existing != nil && existing.State() != bridge.SessionDead {
		fmt.Fprintf(w, "Agent session %s already exists\n", sessionID)
		return existing, nil
	}

	// Build session config
	sessionCfg := bridge.SessionConfigFromProject(workDir, cfg)
	sessionCfg.Env = append(sessionCfg.Env, hooksEnv...)

	// Spawn the bridge session
	session, err := m.bridge.SpawnSession(sessionID, sessionCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to spawn bridge agent session: %w", err)
	}

	logging.Debug("openAgentSessionBridge completed", "workID", workID, "sessionID", sessionID)
	fmt.Fprintf(w, "Created bridge agent session: %s\n", sessionID)
	return session, nil
}

// SpawnPlanSession creates a plan session for a bean.
// When a bridge is configured, spawns a pi RPC session and sends the plan prompt.
// Returns the bridge session (non-nil only when using bridge mode).
func (m *DefaultOrchestratorManager) SpawnPlanSession(ctx context.Context, beanID string, projectName string, mainRepoPath string, w io.Writer) (*bridge.Session, error) {
	if m.bridge != nil {
		return m.spawnPlanSessionBridge(ctx, beanID, mainRepoPath, w)
	}
	return nil, fmt.Errorf("bridge not configured: plan sessions require bridge architecture")
}

// spawnPlanSessionBridge creates a bridge session and sends the plan prompt.
func (m *DefaultOrchestratorManager) spawnPlanSessionBridge(ctx context.Context, beanID string, mainRepoPath string, w io.Writer) (*bridge.Session, error) {
	sessionID := PlanTabName(beanID)

	// Kill existing session if present
	if existing := m.bridge.GetSession(sessionID); existing != nil && existing.State() != bridge.SessionDead {
		_ = m.bridge.KillSession(sessionID)
		fmt.Fprintf(w, "Killed existing plan session: %s\n", sessionID)
	}

	// Build session config from project config
	cfg := bridge.SessionConfigFromProject(mainRepoPath, m.projCfg)
	if m.beansPath != "" {
		cfg.Env = append(cfg.Env, "BEANS_PATH="+m.beansPath)
	}

	// Spawn the bridge session
	session, err := m.bridge.SpawnSession(sessionID, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to spawn bridge plan session: %w", err)
	}
	fmt.Fprintf(w, "Created bridge plan session: %s\n", sessionID)

	// Build and send the plan prompt
	agent, err := agents.NewAgent(m.projCfg)
	if err != nil {
		_ = m.bridge.KillSession(sessionID)
		return nil, fmt.Errorf("failed to create agent for prompt: %w", err)
	}

	prompt, err := agent.BuildPrompt(types.TaskParams{
		Type:      types.TaskTypePlan,
		BeanID:    beanID,
		BeansPath: m.beansPath,
	})
	if err != nil {
		_ = m.bridge.KillSession(sessionID)
		return nil, fmt.Errorf("failed to build plan prompt: %w", err)
	}

	if err := session.Prompt(prompt); err != nil {
		_ = m.bridge.KillSession(sessionID)
		return nil, fmt.Errorf("failed to send plan prompt: %w", err)
	}

	logging.Debug("spawnPlanSessionBridge completed", "beanID", beanID, "sessionID", sessionID)
	return session, nil
}
