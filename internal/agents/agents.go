package agents

import (
	"context"
	"io"

	"github.com/sargehq/sarge/internal/agents/claude"
	"github.com/sargehq/sarge/internal/agents/pi"
	"github.com/sargehq/sarge/internal/agents/types"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/project"
)

// Compile-time check that templateAgent implements Agent.
var _ Agent = (*templateAgent)(nil)

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
	// BuildPrompt builds a prompt string for the given task parameters.
	BuildPrompt(params types.TaskParams) (string, error)

	// Run executes the agent directly in the current terminal (fork/exec).
	Run(ctx context.Context, database *db.DB, taskID string, prompt string, workDir string, cfg *project.Config) error

	// RunInteractive runs the agent interactively, connecting the given
	// stdin/stdout/stderr for direct user interaction (e.g. plan sessions).
	RunInteractive(ctx context.Context, prompt, workDir string, stdin io.Reader, stdout, stderr io.Writer, cfg *project.Config) error
}

// NewAgent creates an Agent from project configuration.
// Returns a Claude agent by default if cfg is nil or unconfigured.
func NewAgent(cfg *project.Config) Agent {
	switch agentTypeFromConfig(cfg) {
	case agentPi:
		return &templateAgent{
			binaryName: pi.Binary,
			templates:  pi.Templates(),
			baseArgs:   pi.BaseArgs,
			taskArgs:   pi.TaskArgs,
		}
	default:
		return &templateAgent{
			binaryName: claude.Binary,
			templates:  claude.Templates(),
			baseArgs:   claude.BaseArgs,
			taskArgs:   claude.TaskArgs,
		}
	}
}
