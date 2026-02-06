package pi

import (
	"bytes"
	_ "embed"
	"fmt"
	"text/template"

	"github.com/sargehq/sarge/internal/agents/types"
	"github.com/sargehq/sarge/internal/beads"
	"github.com/sargehq/sarge/internal/project"
)

// Binary is the CLI binary name for the pi agent.
const Binary = "pi"

// Embedded template text.
//
//go:embed templates/estimate.tmpl
var estimateText string

//go:embed templates/task.tmpl
var taskText string

//go:embed templates/pr.tmpl
var prText string

//go:embed templates/review.tmpl
var reviewText string

//go:embed templates/update-pr-description.tmpl
var updatePRDescriptionText string

//go:embed templates/plan.tmpl
var planText string

//go:embed templates/log_analysis.tmpl
var logAnalysisText string

// Compiled templates.
var (
	Estimate            = template.Must(template.New("estimate").Parse(estimateText))
	Task                = template.Must(template.New("task").Parse(taskText))
	PR                  = template.Must(template.New("pr").Parse(prText))
	Review              = template.Must(template.New("review").Parse(reviewText))
	UpdatePRDescription = template.Must(template.New("update-pr-description").Parse(updatePRDescriptionText))
	Plan                = template.Must(template.New("plan").Parse(planText))
	LogAnalysis         = template.Must(template.New("log_analysis").Parse(logAnalysisText))
)

// Agent implements the agents.Agent interface for the pi CLI.
type Agent struct{}

func (a *Agent) Binary() string {
	return Binary
}

func (a *Agent) BuildArgs(cfg *project.Config) []string {
	return BuildArgs(cfg)
}

func (a *Agent) TaskArgs(taskType string, cfg *project.Config) []string {
	return nil
}

func (a *Agent) BuildTaskPrompt(taskID string, beadList []beads.Bead, branchName, baseBranch string) string {
	data := struct {
		TaskID     string
		BeadIDs    []string
		BranchName string
		BaseBranch string
	}{
		TaskID:     taskID,
		BeadIDs:    beadIDs(beadList),
		BranchName: branchName,
		BaseBranch: baseBranch,
	}

	var buf bytes.Buffer
	if err := Task.Execute(&buf, data); err != nil {
		return fmt.Sprintf("Task %s on branch %s for beads: %v", taskID, branchName, beadIDs(beadList))
	}
	return buf.String()
}

func (a *Agent) BuildEstimatePrompt(taskID string, beadList []beads.Bead) string {
	data := struct {
		TaskID  string
		BeadIDs []string
	}{
		TaskID:  taskID,
		BeadIDs: beadIDs(beadList),
	}

	var buf bytes.Buffer
	if err := Estimate.Execute(&buf, data); err != nil {
		return fmt.Sprintf("Estimation task %s for beads: %v", taskID, beadIDs(beadList))
	}
	return buf.String()
}

func (a *Agent) BuildPRPrompt(taskID string, workID string, branchName string, baseBranch string) string {
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
	if err := PR.Execute(&buf, data); err != nil {
		return fmt.Sprintf("PR creation task %s for work %s on branch %s (base: %s)", taskID, workID, branchName, baseBranch)
	}
	return buf.String()
}

func (a *Agent) BuildReviewPrompt(taskID string, workID string, branchName string, baseBranch string, rootIssueID string) string {
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
	if err := Review.Execute(&buf, data); err != nil {
		return fmt.Sprintf("Review task %s for work %s on branch %s (base: %s)", taskID, workID, branchName, baseBranch)
	}
	return buf.String()
}

func (a *Agent) BuildUpdatePRDescriptionPrompt(taskID string, workID string, prURL string, branchName string, baseBranch string) string {
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
	if err := UpdatePRDescription.Execute(&buf, data); err != nil {
		return fmt.Sprintf("Update PR description task %s for work %s, PR %s on branch %s (base: %s)", taskID, workID, prURL, branchName, baseBranch)
	}
	return buf.String()
}

func (a *Agent) BuildPlanPrompt(beadID string) string {
	data := struct {
		BeadID string
	}{
		BeadID: beadID,
	}

	var buf bytes.Buffer
	if err := Plan.Execute(&buf, data); err != nil {
		return fmt.Sprintf("Planning for issue %s", beadID)
	}
	return buf.String()
}

func (a *Agent) BuildLogAnalysisPrompt(params types.LogAnalysisParams) string {
	var buf bytes.Buffer
	if err := LogAnalysis.Execute(&buf, params); err != nil {
		return fmt.Sprintf("Log analysis task %s for work %s", params.TaskID, params.WorkID)
	}
	return buf.String()
}

// beadIDs extracts bead IDs from a slice of beads.
func beadIDs(beadList []beads.Bead) []string {
	ids := make([]string, len(beadList))
	for i, b := range beadList {
		ids[i] = b.ID
	}
	return ids
}

// BuildArgs returns pi-specific CLI arguments from project configuration.
func BuildArgs(cfg *project.Config) []string {
	if cfg == nil {
		return nil
	}
	var args []string
	if cfg.Pi.Provider != "" {
		args = append(args, "--provider", cfg.Pi.Provider)
	}
	if cfg.Pi.Model != "" {
		args = append(args, "--model", cfg.Pi.Model)
	}
	if cfg.Pi.Thinking != "" {
		args = append(args, "--thinking", cfg.Pi.Thinking)
	}
	return args
}
