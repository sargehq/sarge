package agents

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
	"time"

	"github.com/sargehq/sarge/internal/agents/types"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/project"
)

// TemplateIndex maps template array positions.
const (
	TmplImplement = iota
	TmplEstimate
	TmplPR
	TmplReview
	TmplUpdatePRDescription
	TmplPlan
	TmplLogAnalysis
)

// BaseAgent contains the shared logic for all agent implementations.
// It is designed to be embedded by concrete agent types (e.g. ClaudeAgent, PiAgent).
type BaseAgent struct {
	BinaryName string
	Templates  []*template.Template
	BaseArgsFn func(cfg *project.Config) []string
	TaskArgsFn func(taskType string, cfg *project.Config) []string
}

// BuildPrompt renders a prompt template for the given task parameters.
func (a *BaseAgent) BuildPrompt(params types.TaskParams) (string, error) {
	tmpl, data, err := a.resolveTemplate(params)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("template execution failed for %s: %w", params.Type, err)
	}
	return buf.String(), nil
}

func (a *BaseAgent) resolveTemplate(params types.TaskParams) (*template.Template, any, error) {
	switch params.Type {
	case types.TaskTypeImplement:
		return a.Templates[TmplImplement], struct {
			TaskID     string
			BeadIDs    []string
			BranchName string
			BaseBranch string
		}{
			TaskID:     params.TaskID,
			BeadIDs:    params.BeadIDs,
			BranchName: params.BranchName,
			BaseBranch: params.BaseBranch,
		}, nil

	case types.TaskTypeEstimate:
		return a.Templates[TmplEstimate], struct {
			TaskID  string
			BeadIDs []string
		}{
			TaskID:  params.TaskID,
			BeadIDs: params.BeadIDs,
		}, nil

	case types.TaskTypePR:
		return a.Templates[TmplPR], struct {
			TaskID     string
			WorkID     string
			BranchName string
			BaseBranch string
		}{
			TaskID:     params.TaskID,
			WorkID:     params.WorkID,
			BranchName: params.BranchName,
			BaseBranch: params.BaseBranch,
		}, nil

	case types.TaskTypeReview:
		return a.Templates[TmplReview], struct {
			TaskID      string
			WorkID      string
			BranchName  string
			BaseBranch  string
			RootIssueID string
		}{
			TaskID:      params.TaskID,
			WorkID:      params.WorkID,
			BranchName:  params.BranchName,
			BaseBranch:  params.BaseBranch,
			RootIssueID: params.RootIssueID,
		}, nil

	case types.TaskTypeUpdatePRDescription:
		return a.Templates[TmplUpdatePRDescription], struct {
			TaskID     string
			WorkID     string
			PRURL      string
			BranchName string
			BaseBranch string
		}{
			TaskID:     params.TaskID,
			WorkID:     params.WorkID,
			PRURL:      params.PRURL,
			BranchName: params.BranchName,
			BaseBranch: params.BaseBranch,
		}, nil

	case types.TaskTypePlan:
		return a.Templates[TmplPlan], struct {
			BeadID string
		}{
			BeadID: params.BeadID,
		}, nil

	case types.TaskTypeLogAnalysis:
		return a.Templates[TmplLogAnalysis], struct {
			TaskID        string
			WorkID        string
			BranchName    string
			RootIssueID   string
			WorkflowName  string
			JobName       string
			LogFilePath   string
			ExistingBeads []types.BeadSummary
		}{
			TaskID:        params.TaskID,
			WorkID:        params.WorkID,
			BranchName:    params.BranchName,
			RootIssueID:   params.RootIssueID,
			WorkflowName:  params.WorkflowName,
			JobName:       params.JobName,
			LogFilePath:   params.LogFilePath,
			ExistingBeads: params.ExistingBeads,
		}, nil

	default:
		return nil, nil, fmt.Errorf("unknown task type: %s", params.Type)
	}
}

// Run builds a prompt from params and executes the agent directly in the current terminal (fork/exec).
func (a *BaseAgent) Run(ctx context.Context, database *db.DB, taskID string, params types.TaskParams, workDir string, cfg *project.Config) error {
	prompt, err := a.BuildPrompt(params)
	if err != nil {
		return fmt.Errorf("failed to build prompt: %w", err)
	}

	// Get task to verify it exists
	task, err := database.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to get task %s: %w", taskID, err)
	}
	if task == nil {
		return fmt.Errorf("task %s not found", taskID)
	}

	// Mark task as processing
	if err := database.StartTask(ctx, taskID, workDir); err != nil {
		return fmt.Errorf("failed to start task: %w", err)
	}

	startTime := time.Now()
	fmt.Printf("\n=== Starting %s for task %s at %s ===\n", a.BinaryName, taskID, startTime.Format("15:04:05"))

	// Set up agent command with prompt as argument
	var agentArgs []string
	agentArgs = append(agentArgs, a.BaseArgsFn(cfg)...)
	agentArgs = append(agentArgs, a.TaskArgsFn(task.TaskType, cfg)...)
	agentArgs = append(agentArgs, prompt)
	agentCmd := exec.CommandContext(ctx, a.BinaryName, agentArgs...)
	agentCmd.Dir = workDir
	agentCmd.Stdin = os.Stdin
	agentCmd.Stdout = os.Stdout
	agentCmd.Stderr = os.Stderr

	// Start agent
	if err := agentCmd.Start(); err != nil {
		if dbErr := database.FailTask(ctx, taskID, fmt.Sprintf("failed to start %s: %v", a.BinaryName, err)); dbErr != nil {
			fmt.Printf("Warning: failed to mark task as failed: %v\n", dbErr)
		}
		return fmt.Errorf("failed to start %s: %w", a.BinaryName, err)
	}

	// Run the main monitoring loop
	// Derive project root from workDir (assumes workDir is <project>/<work-id>/tree/)
	projectRoot := filepath.Dir(filepath.Dir(workDir))
	return monitorAgent(ctx, database, taskID, agentCmd, startTime, projectRoot)
}

// RunInteractive builds a prompt from params and runs the agent interactively,
// connecting stdin/stdout/stderr directly.
func (a *BaseAgent) RunInteractive(ctx context.Context, params types.TaskParams, workDir string, stdin io.Reader, stdout, stderr io.Writer, cfg *project.Config) error {
	prompt, err := a.BuildPrompt(params)
	if err != nil {
		return fmt.Errorf("failed to build prompt: %w", err)
	}

	var args []string
	args = append(args, a.BaseArgsFn(cfg)...)
	args = append(args, prompt)

	cmd := exec.CommandContext(ctx, a.BinaryName, args...)
	cmd.Dir = workDir
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s exited with error: %w", a.BinaryName, err)
	}

	return nil
}
