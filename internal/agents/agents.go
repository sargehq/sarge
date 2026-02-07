package agents

import (
	"context"
	"fmt"
	"io"

	"github.com/sargehq/sarge/internal/agents/claude"
	"github.com/sargehq/sarge/internal/agents/pi"
	"github.com/sargehq/sarge/internal/agents/types"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/project"
)

// Compile-time checks that concrete agents implement Agent.
var (
	_ Agent = (*claude.Agent)(nil)
	_ Agent = (*pi.Agent)(nil)
)

// agentType represents which coding agent to use.
type agentType string

const (
	agentClaude agentType = "claude"
	agentPi     agentType = "pi"
)

// agentTypeFromConfig returns the agent type from project configuration.
// Defaults to agentClaude if not configured.
func agentTypeFromConfig(cfg *project.Config) agentType {
	if cfg == nil || cfg.Agent.Type == "" {
		return agentClaude
	}
	return agentType(cfg.Agent.Type)
}

// Agent encapsulates all agent-specific behavior: prompt building and execution.
type Agent interface {
	// Run builds a prompt from params and executes the agent directly in the current terminal (fork/exec).
	Run(ctx context.Context, database *db.DB, taskID string, params types.TaskParams, workDir string, cfg *project.Config) error

	// RunInteractive builds a prompt from params and runs the agent interactively,
	// connecting the given stdin/stdout/stderr for direct user interaction (e.g. plan sessions).
	RunInteractive(ctx context.Context, params types.TaskParams, workDir string, stdin io.Reader, stdout, stderr io.Writer, cfg *project.Config) error
}

// NewAgent creates an Agent from project configuration.
// Returns a Claude agent by default if cfg is nil or unconfigured.
// Returns an error if the configured agent type is not recognized.
func NewAgent(cfg *project.Config) (Agent, error) {
	at := agentTypeFromConfig(cfg)
	switch at {
	case agentPi:
		return pi.New(), nil
	case agentClaude:
		return claude.New(), nil
	default:
		return nil, fmt.Errorf("unknown agent type %q (supported: %q, %q)", at, agentClaude, agentPi)
	}
}
