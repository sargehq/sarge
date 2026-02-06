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

// AgentType represents which coding agent to use.
type AgentType string

const (
	// AgentClaude uses Claude Code as the coding agent.
	AgentClaude AgentType = "claude"
	// AgentPi uses the pi coding agent.
	AgentPi AgentType = "pi"
)

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

// templatesFor returns the template set for the given agent type.
// Defaults to Claude templates if the agent type is unknown.
func templatesFor(agentType AgentType) *templateSet {
	switch agentType {
	case AgentPi:
		return piTemplates
	default:
		return claudeTemplates
	}
}

// AgentTypeFromConfig returns the agent type from project configuration.
// Defaults to AgentClaude if not configured.
func AgentTypeFromConfig(cfg *project.Config) AgentType {
	if cfg == nil || cfg.Agent.Type == "" {
		return AgentClaude
	}
	return AgentType(cfg.Agent.Type)
}

// AgentBinary returns the CLI binary name for the given agent type.
func AgentBinary(agentType AgentType) string {
	switch agentType {
	case AgentPi:
		return pi.Binary
	default:
		return claude.Binary
	}
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

// BuildTaskPrompt builds a prompt for a task with multiple beads.
func BuildTaskPrompt(taskID string, beadList []beads.Bead, branchName, baseBranch string, agentType AgentType) string {
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
	if err := templatesFor(agentType).task.Execute(&buf, data); err != nil {
		return fmt.Sprintf("Task %s on branch %s for beads: %v", taskID, branchName, getBeadIDs(beadList))
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

// BuildEstimatePrompt builds a prompt for complexity estimation of beads.
func BuildEstimatePrompt(taskID string, beadList []beads.Bead, agentType AgentType) string {
	data := struct {
		TaskID  string
		BeadIDs []string
	}{
		TaskID:  taskID,
		BeadIDs: getBeadIDs(beadList),
	}

	var buf bytes.Buffer
	if err := templatesFor(agentType).estimate.Execute(&buf, data); err != nil {
		return fmt.Sprintf("Estimation task %s for beads: %v", taskID, getBeadIDs(beadList))
	}

	return buf.String()
}

// BuildPRPrompt builds a prompt for PR creation.
func BuildPRPrompt(taskID string, workID string, branchName string, baseBranch string, agentType AgentType) string {
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
	if err := templatesFor(agentType).pr.Execute(&buf, data); err != nil {
		return fmt.Sprintf("PR creation task %s for work %s on branch %s (base: %s)", taskID, workID, branchName, baseBranch)
	}

	return buf.String()
}

// BuildReviewPrompt builds a prompt for code review.
func BuildReviewPrompt(taskID string, workID string, branchName string, baseBranch string, rootIssueID string, agentType AgentType) string {
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
	if err := templatesFor(agentType).review.Execute(&buf, data); err != nil {
		return fmt.Sprintf("Review task %s for work %s on branch %s (base: %s)", taskID, workID, branchName, baseBranch)
	}

	return buf.String()
}

// BuildUpdatePRDescriptionPrompt builds a prompt for updating a PR description.
func BuildUpdatePRDescriptionPrompt(taskID string, workID string, prURL string, branchName string, baseBranch string, agentType AgentType) string {
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
	if err := templatesFor(agentType).updatePRDescription.Execute(&buf, data); err != nil {
		return fmt.Sprintf("Update PR description task %s for work %s, PR %s on branch %s (base: %s)", taskID, workID, prURL, branchName, baseBranch)
	}

	return buf.String()
}

// BuildPlanPrompt builds a prompt for planning an issue.
func BuildPlanPrompt(beadID string, agentType AgentType) string {
	data := struct {
		BeadID string
	}{
		BeadID: beadID,
	}

	var buf bytes.Buffer
	if err := templatesFor(agentType).plan.Execute(&buf, data); err != nil {
		return fmt.Sprintf("Planning for issue %s", beadID)
	}

	return buf.String()
}

// BuildLogAnalysisPrompt builds a prompt for agent-based CI log analysis.
func BuildLogAnalysisPrompt(params LogAnalysisParams, agentType AgentType) string {
	var buf bytes.Buffer
	if err := templatesFor(agentType).logAnalysis.Execute(&buf, params); err != nil {
		return fmt.Sprintf("Log analysis task %s for work %s", params.TaskID, params.WorkID)
	}

	return buf.String()
}

// RunPlanSession runs an interactive agent session for planning an issue.
// This launches the agent with the plan prompt and connects stdin/stdout/stderr
// for interactive use. The config parameter controls agent settings.
func RunPlanSession(ctx context.Context, beadID string, workDir string, stdin io.Reader, stdout, stderr io.Writer, cfg *project.Config) error {
	agentType := AgentTypeFromConfig(cfg)
	prompt := BuildPlanPrompt(beadID, agentType)

	agentBin := AgentBinary(agentType)
	var args []string
	switch agentType {
	case AgentPi:
		args = append(args, pi.BuildArgs(cfg)...)
	default:
		args = append(args, claude.BuildArgs(cfg)...)
	}
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
