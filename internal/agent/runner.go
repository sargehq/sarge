package agent

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io"
	"os/exec"
	"text/template"

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

// Claude templates
//
//go:embed templates/estimate.tmpl
var claudeEstimateText string

//go:embed templates/task.tmpl
var claudeTaskText string

//go:embed templates/pr.tmpl
var claudePRText string

//go:embed templates/review.tmpl
var claudeReviewText string

//go:embed templates/update-pr-description.tmpl
var claudeUpdatePRDescriptionText string

//go:embed templates/plan.tmpl
var claudePlanText string

//go:embed templates/log_analysis.tmpl
var claudeLogAnalysisText string

// Pi templates
//
//go:embed templates/pi/estimate.tmpl
var piEstimateText string

//go:embed templates/pi/task.tmpl
var piTaskText string

//go:embed templates/pi/pr.tmpl
var piPRText string

//go:embed templates/pi/review.tmpl
var piReviewText string

//go:embed templates/pi/update-pr-description.tmpl
var piUpdatePRDescriptionText string

//go:embed templates/pi/plan.tmpl
var piPlanText string

//go:embed templates/pi/log_analysis.tmpl
var piLogAnalysisText string

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
	estimate:            template.Must(template.New("estimate").Parse(claudeEstimateText)),
	task:                template.Must(template.New("task").Parse(claudeTaskText)),
	pr:                  template.Must(template.New("pr").Parse(claudePRText)),
	review:              template.Must(template.New("review").Parse(claudeReviewText)),
	updatePRDescription: template.Must(template.New("update-pr-description").Parse(claudeUpdatePRDescriptionText)),
	plan:                template.Must(template.New("plan").Parse(claudePlanText)),
	logAnalysis:         template.Must(template.New("log_analysis").Parse(claudeLogAnalysisText)),
}

var piTemplates = &templateSet{
	estimate:            template.Must(template.New("estimate").Parse(piEstimateText)),
	task:                template.Must(template.New("task").Parse(piTaskText)),
	pr:                  template.Must(template.New("pr").Parse(piPRText)),
	review:              template.Must(template.New("review").Parse(piReviewText)),
	updatePRDescription: template.Must(template.New("update-pr-description").Parse(piUpdatePRDescriptionText)),
	plan:                template.Must(template.New("plan").Parse(piPlanText)),
	logAnalysis:         template.Must(template.New("log_analysis").Parse(piLogAnalysisText)),
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
		// Fallback to simple string if template execution fails
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
		// Fallback to simple string if template execution fails
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
		// Fallback to simple string if template execution fails
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
		// Fallback to simple string if template execution fails
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
		// Fallback to simple string if template execution fails
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
		// Fallback to simple string if template execution fails
		return fmt.Sprintf("Planning for issue %s", beadID)
	}

	return buf.String()
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

// BuildLogAnalysisPrompt builds a prompt for agent-based CI log analysis.
func BuildLogAnalysisPrompt(params LogAnalysisParams, agentType AgentType) string {
	var buf bytes.Buffer
	if err := templatesFor(agentType).logAnalysis.Execute(&buf, params); err != nil {
		// Fallback to simple string if template execution fails
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
	if agentType == AgentClaude && cfg != nil && cfg.Claude.ShouldSkipPermissions() {
		args = append(args, "--dangerously-skip-permissions")
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

// AgentBinary returns the CLI binary name for the given agent type.
func AgentBinary(agentType AgentType) string {
	switch agentType {
	case AgentPi:
		return "pi"
	default:
		return "claude"
	}
}

// AgentTypeFromConfig returns the agent type from project configuration.
// Defaults to AgentClaude if not configured.
func AgentTypeFromConfig(cfg *project.Config) AgentType {
	if cfg == nil {
		return AgentClaude
	}
	return AgentType(cfg.Agent.Type)
}
