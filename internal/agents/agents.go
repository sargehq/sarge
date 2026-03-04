package agents

import (
	"context"
	"io"

	"github.com/sargehq/sarge/internal/agents/pi"
	"github.com/sargehq/sarge/internal/agents/types"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/project"
)

// Compile-time check that the pi agent implements Agent.
var _ Agent = (*pi.Agent)(nil)

// Agent encapsulates all agent-specific behavior: prompt building and execution.
type Agent interface {
	// BuildPrompt builds a prompt string from the given task parameters.
	BuildPrompt(params types.TaskParams) (string, error)

	// Run builds a prompt from params and executes the agent directly in the current terminal (fork/exec).
	Run(ctx context.Context, database *db.DB, taskID string, params types.TaskParams, workDir string, cfg *project.Config) error

	// RunInteractive builds a prompt from params and runs the agent interactively,
	// connecting the given stdin/stdout/stderr for direct user interaction (e.g. plan sessions).
	RunInteractive(ctx context.Context, params types.TaskParams, workDir string, stdin io.Reader, stdout, stderr io.Writer, cfg *project.Config) error
}

// NewAgent creates a pi Agent. The cfg parameter is accepted for interface
// compatibility but is not used for agent selection (pi is the only agent).
func NewAgent(cfg *project.Config) (Agent, error) {
	return pi.New(), nil
}
