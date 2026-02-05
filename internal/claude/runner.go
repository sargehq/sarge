package claude

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

//go:embed templates/estimate.tmpl
var estimateTemplateText string

//go:embed templates/task.tmpl
var taskTemplateText string

//go:embed templates/pr.tmpl
var prTemplateText string

//go:embed templates/review.tmpl
var reviewTemplateText string

//go:embed templates/update-pr-description.tmpl
var updatePRDescriptionTemplateText string

//go:embed templates/plan.tmpl
var planTemplateText string

//go:embed templates/log_analysis.tmpl
var logAnalysisTemplateText string

var (
	estimateTmpl            = template.Must(template.New("estimate").Parse(estimateTemplateText))
	taskTmpl                = template.Must(template.New("task").Parse(taskTemplateText))
	prTmpl                  = template.Must(template.New("pr").Parse(prTemplateText))
	reviewTmpl              = template.Must(template.New("review").Parse(reviewTemplateText))
	updatePRDescriptionTmpl = template.Must(template.New("update-pr-description").Parse(updatePRDescriptionTemplateText))
	planTmpl                = template.Must(template.New("plan").Parse(planTemplateText))
	logAnalysisTmpl         = template.Must(template.New("log_analysis").Parse(logAnalysisTemplateText))
)

// BuildTaskPrompt builds a prompt for a task with multiple beads.
func BuildTaskPrompt(taskID string, beadList []beads.Bead, branchName, baseBranch string) string {
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
	if err := taskTmpl.Execute(&buf, data); err != nil {
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
func BuildEstimatePrompt(taskID string, beadList []beads.Bead) string {
	data := struct {
		TaskID  string
		BeadIDs []string
	}{
		TaskID:  taskID,
		BeadIDs: getBeadIDs(beadList),
	}

	var buf bytes.Buffer
	if err := estimateTmpl.Execute(&buf, data); err != nil {
		// Fallback to simple string if template execution fails
		return fmt.Sprintf("Estimation task %s for beads: %v", taskID, getBeadIDs(beadList))
	}

	return buf.String()
}

// BuildPRPrompt builds a prompt for PR creation.
func BuildPRPrompt(taskID string, workID string, branchName string, baseBranch string) string {
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
	if err := prTmpl.Execute(&buf, data); err != nil {
		// Fallback to simple string if template execution fails
		return fmt.Sprintf("PR creation task %s for work %s on branch %s (base: %s)", taskID, workID, branchName, baseBranch)
	}

	return buf.String()
}

// BuildReviewPrompt builds a prompt for code review.
func BuildReviewPrompt(taskID string, workID string, branchName string, baseBranch string, rootIssueID string) string {
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
	if err := reviewTmpl.Execute(&buf, data); err != nil {
		// Fallback to simple string if template execution fails
		return fmt.Sprintf("Review task %s for work %s on branch %s (base: %s)", taskID, workID, branchName, baseBranch)
	}

	return buf.String()
}

// BuildUpdatePRDescriptionPrompt builds a prompt for updating a PR description.
func BuildUpdatePRDescriptionPrompt(taskID string, workID string, prURL string, branchName string, baseBranch string) string {
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
	if err := updatePRDescriptionTmpl.Execute(&buf, data); err != nil {
		// Fallback to simple string if template execution fails
		return fmt.Sprintf("Update PR description task %s for work %s, PR %s on branch %s (base: %s)", taskID, workID, prURL, branchName, baseBranch)
	}

	return buf.String()
}

// BuildPlanPrompt builds a prompt for planning an issue.
func BuildPlanPrompt(beadID string) string {
	data := struct {
		BeadID string
	}{
		BeadID: beadID,
	}

	var buf bytes.Buffer
	if err := planTmpl.Execute(&buf, data); err != nil {
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

// BuildLogAnalysisPrompt builds a prompt for Claude-based CI log analysis.
func BuildLogAnalysisPrompt(params LogAnalysisParams) string {
	var buf bytes.Buffer
	if err := logAnalysisTmpl.Execute(&buf, params); err != nil {
		// Fallback to simple string if template execution fails
		return fmt.Sprintf("Log analysis task %s for work %s", params.TaskID, params.WorkID)
	}

	return buf.String()
}

// RunPlanSession runs an interactive Claude session for planning an issue.
// This launches Claude with the plan prompt and connects stdin/stdout/stderr
// for interactive use. The config parameter controls Claude settings like --dangerously-skip-permissions.
func RunPlanSession(ctx context.Context, beadID string, workDir string, stdin io.Reader, stdout, stderr io.Writer, cfg *project.Config) error {
	prompt := BuildPlanPrompt(beadID)

	var args []string
	if cfg != nil && cfg.Claude.ShouldSkipPermissions() {
		args = append(args, "--dangerously-skip-permissions")
	}
	args = append(args, prompt)

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = workDir
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("claude exited with error: %w", err)
	}

	return nil
}
