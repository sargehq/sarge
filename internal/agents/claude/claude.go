package claude

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"text/template"
	"time"

	"github.com/sargehq/sarge/internal/agents/runner"
	"github.com/sargehq/sarge/internal/agents/types"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/project"
)

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

// Template index positions.
const (
	tmplImplement = iota
	tmplEstimate
	tmplPR
	tmplReview
	tmplUpdatePRDescription
	tmplPlan
	tmplLogAnalysis
)

func templates() []*template.Template {
	return []*template.Template{
		template.Must(template.New("implement").Parse(taskText)),
		template.Must(template.New("estimate").Parse(estimateText)),
		template.Must(template.New("pr").Parse(prText)),
		template.Must(template.New("review").Parse(reviewText)),
		template.Must(template.New("update-pr-description").Parse(updatePRDescriptionText)),
		template.Must(template.New("plan").Parse(planText)),
		template.Must(template.New("log_analysis").Parse(logAnalysisText)),
	}
}

const binary = "claude"

func baseArgs(cfg *project.Config) []string {
	if cfg == nil {
		return nil
	}
	var args []string
	if cfg.Claude.ShouldSkipPermissions() {
		args = append(args, "--dangerously-skip-permissions")
	}
	return args
}

func taskArgs(taskType string, cfg *project.Config) []string {
	if taskType == "log_analysis" && cfg != nil {
		model := cfg.LogParser.GetModel()
		if model != "" {
			return []string{"--model", model}
		}
	}
	return nil
}

// Agent implements the agents.Agent interface for the Claude coding agent.
type Agent struct {
	binaryName string
	templates  []*template.Template
	baseArgs   func(cfg *project.Config) []string
	taskArgs   func(taskType string, cfg *project.Config) []string
}

// New creates a new Claude Agent.
func New() *Agent {
	return &Agent{
		binaryName: binary,
		templates:  templates(),
		baseArgs:   baseArgs,
		taskArgs:   taskArgs,
	}
}

func (a *Agent) BuildPrompt(params types.TaskParams) (string, error) {
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

func (a *Agent) resolveTemplate(params types.TaskParams) (*template.Template, any, error) {
	switch params.Type {
	case types.TaskTypeImplement:
		return a.templates[tmplImplement], struct {
			TaskID     string
			BeanIDs    []string
			BranchName string
			BaseBranch string
		}{
			TaskID:     params.TaskID,
			BeanIDs:    params.BeanIDs,
			BranchName: params.BranchName,
			BaseBranch: params.BaseBranch,
		}, nil

	case types.TaskTypeEstimate:
		return a.templates[tmplEstimate], struct {
			TaskID  string
			BeanIDs []string
		}{
			TaskID:  params.TaskID,
			BeanIDs: params.BeanIDs,
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
			BeanID string
		}{
			BeanID: params.BeanID,
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
			ExistingBeans []types.BeanSummary
		}{
			TaskID:        params.TaskID,
			WorkID:        params.WorkID,
			BranchName:    params.BranchName,
			RootIssueID:   params.RootIssueID,
			WorkflowName:  params.WorkflowName,
			JobName:       params.JobName,
			LogFilePath:   params.LogFilePath,
			ExistingBeans: params.ExistingBeans,
		}, nil

	default:
		return nil, nil, fmt.Errorf("unknown task type: %s", params.Type)
	}
}

// Run builds a prompt from params and executes the agent in the current terminal.
func (a *Agent) Run(ctx context.Context, database *db.DB, taskID string, params types.TaskParams, workDir string, cfg *project.Config) error {
	prompt, err := a.BuildPrompt(params)
	if err != nil {
		return fmt.Errorf("failed to build prompt: %w", err)
	}

	task, err := database.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to get task %s: %w", taskID, err)
	}
	if task == nil {
		return fmt.Errorf("task %s not found", taskID)
	}

	if err := database.StartTask(ctx, taskID, workDir); err != nil {
		return fmt.Errorf("failed to start task: %w", err)
	}

	startTime := time.Now()
	fmt.Printf("\n=== Starting %s for task %s at %s ===\n", a.binaryName, taskID, startTime.Format("15:04:05"))

	var agentArgs []string
	agentArgs = append(agentArgs, a.baseArgs(cfg)...)
	agentArgs = append(agentArgs, a.taskArgs(task.TaskType, cfg)...)
	agentArgs = append(agentArgs, prompt)
	agentCmd := exec.CommandContext(ctx, a.binaryName, agentArgs...)
	agentCmd.Dir = workDir
	agentCmd.Stdin = os.Stdin
	agentCmd.Stdout = os.Stdout
	agentCmd.Stderr = os.Stderr

	if err := agentCmd.Start(); err != nil {
		if dbErr := database.FailTask(ctx, taskID, fmt.Sprintf("failed to start %s: %v", a.binaryName, err)); dbErr != nil {
			fmt.Printf("Warning: failed to mark task as failed: %v\n", dbErr)
		}
		return fmt.Errorf("failed to start %s: %w", a.binaryName, err)
	}

	return runner.MonitorAgent(ctx, database, taskID, agentCmd, startTime, workDir)
}

// RunInteractive builds a prompt and runs the agent interactively.
func (a *Agent) RunInteractive(ctx context.Context, params types.TaskParams, workDir string, stdin io.Reader, stdout, stderr io.Writer, cfg *project.Config) error {
	prompt, err := a.BuildPrompt(params)
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
