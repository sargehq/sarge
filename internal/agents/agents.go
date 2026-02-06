package agents

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"text/template"

	"github.com/sargehq/sarge/internal/agents/claude"
	"github.com/sargehq/sarge/internal/agents/pi"
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
	BuildLogAnalysisPrompt(params LogAnalysisParams) string
}

// templateSet holds all compiled templates for a specific agent type.
type templateSet struct {
	estimate            *template.Template
	task                *template.Template
	pr                  *template.Template
	review              *template.Template
	updatePRDescription *template.Template
	plan                *template.Template
	logAnalysis         *template.Template
}

var claudeTemplates = &templateSet{
	estimate:            claude.Estimate,
	task:                claude.Task,
	pr:                  claude.PR,
	review:              claude.Review,
	updatePRDescription: claude.UpdatePRDescription,
	plan:                claude.Plan,
	logAnalysis:         claude.LogAnalysis,
}

var piTemplates = &templateSet{
	estimate:            pi.Estimate,
	task:                pi.Task,
	pr:                  pi.PR,
	review:              pi.Review,
	updatePRDescription: pi.UpdatePRDescription,
	plan:                pi.Plan,
	logAnalysis:         pi.LogAnalysis,
}

// BeadSummary provides a summary of an existing bead for matching.
type BeadSummary struct {
	ID          string
	Title       string
	Description string
}

// LogAnalysisParams contains parameters for building a log analysis prompt.
type LogAnalysisParams struct {
	TaskID        string
	WorkID        string
	BranchName    string
	RootIssueID   string
	WorkflowName  string
	JobName       string
	LogFilePath   string        // Path to temp file containing CI logs
	ExistingBeads []BeadSummary // Existing open beads to match against
}

// baseAgent implements the 7 prompt methods shared by all agents.
type baseAgent struct {
	templates *templateSet
}

func (a *baseAgent) BuildTaskPrompt(taskID string, beadList []beads.Bead, branchName, baseBranch string) string {
	data := struct {
		TaskID     string
		BeadIDs    []string
		BranchName string
		BaseBranch string
	}{
		TaskID:     taskID,
		BeadIDs:    getBeadIDs(beadList),
		BranchName: branchName,
		BaseBranch: baseBranch,
	}

	var buf bytes.Buffer
	if err := a.templates.task.Execute(&buf, data); err != nil {
		return fmt.Sprintf("Task %s on branch %s for beads: %v", taskID, branchName, getBeadIDs(beadList))
	}

	return buf.String()
}

func (a *baseAgent) BuildEstimatePrompt(taskID string, beadList []beads.Bead) string {
	data := struct {
		TaskID  string
		BeadIDs []string
	}{
		TaskID:  taskID,
		BeadIDs: getBeadIDs(beadList),
	}

	var buf bytes.Buffer
	if err := a.templates.estimate.Execute(&buf, data); err != nil {
		return fmt.Sprintf("Estimation task %s for beads: %v", taskID, getBeadIDs(beadList))
	}

	return buf.String()
}

func (a *baseAgent) BuildPRPrompt(taskID string, workID string, branchName string, baseBranch string) string {
	data := struct {
		TaskID     string
		WorkID     string
		BranchName string
		BaseBranch string
	}{
		TaskID:     taskID,
		WorkID:     workID,
		BranchName: branchName,
		BaseBranch: baseBranch,
	}

	var buf bytes.Buffer
	if err := a.templates.pr.Execute(&buf, data); err != nil {
		return fmt.Sprintf("PR creation task %s for work %s on branch %s (base: %s)", taskID, workID, branchName, baseBranch)
	}

	return buf.String()
}

func (a *baseAgent) BuildReviewPrompt(taskID string, workID string, branchName string, baseBranch string, rootIssueID string) string {
	data := struct {
		TaskID      string
		WorkID      string
		BranchName  string
		BaseBranch  string
		RootIssueID string
	}{
		TaskID:      taskID,
		WorkID:      workID,
		BranchName:  branchName,
		BaseBranch:  baseBranch,
		RootIssueID: rootIssueID,
	}

	var buf bytes.Buffer
	if err := a.templates.review.Execute(&buf, data); err != nil {
		return fmt.Sprintf("Review task %s for work %s on branch %s (base: %s)", taskID, workID, branchName, baseBranch)
	}

	return buf.String()
}

func (a *baseAgent) BuildUpdatePRDescriptionPrompt(taskID string, workID string, prURL string, branchName string, baseBranch string) string {
	data := struct {
		TaskID     string
		WorkID     string
		PRURL      string
		BranchName string
		BaseBranch string
	}{
		TaskID:     taskID,
		WorkID:     workID,
		PRURL:      prURL,
		BranchName: branchName,
		BaseBranch: baseBranch,
	}

	var buf bytes.Buffer
	if err := a.templates.updatePRDescription.Execute(&buf, data); err != nil {
		return fmt.Sprintf("Update PR description task %s for work %s, PR %s on branch %s (base: %s)", taskID, workID, prURL, branchName, baseBranch)
	}

	return buf.String()
}

func (a *baseAgent) BuildPlanPrompt(beadID string) string {
	data := struct {
		BeadID string
	}{
		BeadID: beadID,
	}

	var buf bytes.Buffer
	if err := a.templates.plan.Execute(&buf, data); err != nil {
		return fmt.Sprintf("Planning for issue %s", beadID)
	}

	return buf.String()
}

func (a *baseAgent) BuildLogAnalysisPrompt(params LogAnalysisParams) string {
	var buf bytes.Buffer
	if err := a.templates.logAnalysis.Execute(&buf, params); err != nil {
		return fmt.Sprintf("Log analysis task %s for work %s", params.TaskID, params.WorkID)
	}

	return buf.String()
}

// getBeadIDs extracts bead IDs from a slice of beads.
func getBeadIDs(beadList []beads.Bead) []string {
	ids := make([]string, len(beadList))
	for i, b := range beadList {
		ids[i] = b.ID
	}
	return ids
}

// claudeAgent adapts baseAgent for the Claude Code CLI.
type claudeAgent struct {
	baseAgent
}

func (a *claudeAgent) Binary() string {
	return claude.Binary
}

func (a *claudeAgent) BuildArgs(cfg *project.Config) []string {
	return claude.BuildArgs(cfg)
}

func (a *claudeAgent) TaskArgs(taskType string, cfg *project.Config) []string {
	// Use configured model for log_analysis tasks
	if taskType == "log_analysis" && cfg != nil {
		model := cfg.LogParser.GetModel()
		if model != "" {
			return []string{"--model", model}
		}
	}
	return nil
}

// piAgent adapts baseAgent for the pi CLI.
type piAgent struct {
	baseAgent
}

func (a *piAgent) Binary() string {
	return pi.Binary
}

func (a *piAgent) BuildArgs(cfg *project.Config) []string {
	return pi.BuildArgs(cfg)
}

func (a *piAgent) TaskArgs(taskType string, cfg *project.Config) []string {
	return nil
}

// NewAgent creates an Agent from project configuration.
// Returns a Claude agent by default if cfg is nil or unconfigured.
func NewAgent(cfg *project.Config) Agent {
	switch agentTypeFromConfig(cfg) {
	case agentPi:
		return &piAgent{baseAgent{templates: piTemplates}}
	default:
		return &claudeAgent{baseAgent{templates: claudeTemplates}}
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
