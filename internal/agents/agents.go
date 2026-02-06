package agents

import (
	"context"
	"fmt"
	"io"
	"os/exec"

	"github.com/sargehq/sarge/internal/agents/claude"
	"github.com/sargehq/sarge/internal/agents/pi"
	"github.com/sargehq/sarge/internal/agents/types"
	"github.com/sargehq/sarge/internal/beads"
	"github.com/sargehq/sarge/internal/project"
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

// Agent encapsulates all agent-specific behavior: binary, CLI args, and prompt building.
type Agent interface {
	// Binary returns the CLI binary name for this agent.
	Binary() string

	// BuildArgs returns base CLI arguments for this agent from project configuration.
	BuildArgs(cfg *project.Config) []string

	// TaskArgs returns additional CLI arguments for a specific task type.
	// For example, log_analysis tasks on Claude may need a specific --model flag.
	TaskArgs(taskType string, cfg *project.Config) []string

	// BuildTaskPrompt builds a prompt for a task with multiple beads.
	BuildTaskPrompt(taskID string, beadList []beads.Bead, branchName, baseBranch string) string

	// BuildEstimatePrompt builds a prompt for complexity estimation of beads.
	BuildEstimatePrompt(taskID string, beadList []beads.Bead) string

	// BuildPRPrompt builds a prompt for PR creation.
	BuildPRPrompt(taskID string, workID string, branchName string, baseBranch string) string

	// BuildReviewPrompt builds a prompt for code review.
	BuildReviewPrompt(taskID string, workID string, branchName string, baseBranch string, rootIssueID string) string

	// BuildUpdatePRDescriptionPrompt builds a prompt for updating a PR description.
	BuildUpdatePRDescriptionPrompt(taskID string, workID string, prURL string, branchName string, baseBranch string) string

	// BuildPlanPrompt builds a prompt for planning an issue.
	BuildPlanPrompt(beadID string) string

	// BuildLogAnalysisPrompt builds a prompt for agent-based CI log analysis.
	BuildLogAnalysisPrompt(params types.LogAnalysisParams) string
}

// NewAgent creates an Agent from project configuration.
// Returns a Claude agent by default if cfg is nil or unconfigured.
func NewAgent(cfg *project.Config) Agent {
	switch agentTypeFromConfig(cfg) {
	case agentPi:
		return &pi.Agent{}
	default:
		return &claude.Agent{}
	}
}

// RunPlanSession runs an interactive agent session for planning an issue.
// This launches the agent with the plan prompt and connects stdin/stdout/stderr
// for interactive use.
func RunPlanSession(ctx context.Context, agent Agent, beadID string, workDir string, stdin io.Reader, stdout, stderr io.Writer, cfg *project.Config) error {
	prompt := agent.BuildPlanPrompt(beadID)

	agentBin := agent.Binary()
	var args []string
	args = append(args, agent.BuildArgs(cfg)...)
	args = append(args, prompt)

	cmd := exec.CommandContext(ctx, agentBin, args...)
	cmd.Dir = workDir
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s exited with error: %w", agentBin, err)
	}

	return nil
}
