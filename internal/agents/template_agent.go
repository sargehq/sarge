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

// templateIndex maps template array positions.
const (
	tmplImplement = iota
	tmplEstimate
	tmplPR
	tmplReview
	tmplUpdatePRDescription
	tmplPlan
	tmplLogAnalysis
)

// templateAgent is the single concrete Agent implementation.
// It is parameterized by a set of templates and binary config functions
// provided by the claude/ and pi/ subpackages.
type templateAgent struct {
	binaryName string
	templates  [7]*template.Template
	baseArgs   func(cfg *project.Config) []string
	taskArgs   func(taskType string, cfg *project.Config) []string
}

func (a *templateAgent) buildPrompt(params types.TaskParams) (string, error) {
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

func (a *templateAgent) resolveTemplate(params types.TaskParams) (*template.Template, any, error) {
	switch params.Type {
	case types.TaskTypeImplement:
		return a.templates[tmplImplement], struct {
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
		return a.templates[tmplEstimate], struct {
			TaskID  string
			BeadIDs []string
		}{
			TaskID:  params.TaskID,
			BeadIDs: params.BeadIDs,
		}, nil

	case types.TaskTypePR:
		return a.templates[tmplPR], struct {
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
		return a.templates[tmplReview], struct {
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
		return a.templates[tmplUpdatePRDescription], struct {
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
		return a.templates[tmplPlan], struct {
			BeadID string
		}{
			BeadID: params.BeadID,
		}, nil

	case types.TaskTypeLogAnalysis:
		return a.templates[tmplLogAnalysis], struct {
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
func (a *templateAgent) Run(ctx context.Context, database *db.DB, taskID string, params types.TaskParams, workDir string, cfg *project.Config) error {
	prompt, err := a.buildPrompt(params)
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
	fmt.Printf("\n=== Starting %s for task %s at %s ===\n", a.binaryName, taskID, startTime.Format("15:04:05"))

	// Set up agent command with prompt as argument
	var agentArgs []string
	agentArgs = append(agentArgs, a.baseArgs(cfg)...)
	agentArgs = append(agentArgs, a.taskArgs(task.TaskType, cfg)...)
	agentArgs = append(agentArgs, prompt)
	agentCmd := exec.CommandContext(ctx, a.binaryName, agentArgs...)
	agentCmd.Dir = workDir
	agentCmd.Stdin = os.Stdin
	agentCmd.Stdout = os.Stdout
	agentCmd.Stderr = os.Stderr

	// Start agent
	if err := agentCmd.Start(); err != nil {
		if dbErr := database.FailTask(ctx, taskID, fmt.Sprintf("failed to start %s: %v", a.binaryName, err)); dbErr != nil {
			fmt.Printf("Warning: failed to mark task as failed: %v\n", dbErr)
		}
		return fmt.Errorf("failed to start %s: %w", a.binaryName, err)
	}

	// Run the main monitoring loop
	// Derive project root from workDir (assumes workDir is <project>/<work-id>/tree/)
	projectRoot := filepath.Dir(filepath.Dir(workDir))
	return monitorAgent(ctx, database, taskID, agentCmd, startTime, projectRoot)
}

// RunInteractive builds a prompt from params and runs the agent interactively,
// connecting stdin/stdout/stderr directly.
func (a *templateAgent) RunInteractive(ctx context.Context, params types.TaskParams, workDir string, stdin io.Reader, stdout, stderr io.Writer, cfg *project.Config) error {
	prompt, err := a.buildPrompt(params)
	if err != nil {
		return fmt.Errorf("failed to build prompt: %w", err)
	}

	var args []string
	args = append(args, a.baseArgs(cfg)...)
	args = append(args, prompt)

	cmd := exec.CommandContext(ctx, a.binaryName, args...)
	cmd.Dir = workDir
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s exited with error: %w", a.binaryName, err)
	}

	return nil
}
