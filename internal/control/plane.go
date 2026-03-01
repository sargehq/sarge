//go:generate moq -stub -out control_mock_test.go -pkg control_test . WorkDestroyer

package control

import (
	"context"
	"io"

	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/feedback"
	"github.com/sargehq/sarge/internal/git"
	"github.com/sargehq/sarge/internal/github"
	"github.com/sargehq/sarge/internal/mise"
	"github.com/sargehq/sarge/internal/project"
	"github.com/sargehq/sarge/internal/work"
	"github.com/sargehq/sarge/internal/worktree"
	"github.com/sargehq/sarge/internal/zellij"
)

// WorkDestroyer defines the interface for destroying work units.
// This abstraction enables testing without actual file system operations.
type WorkDestroyer interface {
	DestroyWork(ctx context.Context, workID string, w io.Writer) error
}

// DefaultWorkDestroyer implements WorkDestroyer using the work package.
type DefaultWorkDestroyer struct {
	workService *work.WorkService
}

// NewWorkDestroyer creates a new DefaultWorkDestroyer with a WorkService.
func NewWorkDestroyer(proj *project.Project) *DefaultWorkDestroyer {
	return &DefaultWorkDestroyer{
		workService: work.NewWorkService(proj),
	}
}

// DestroyWork implements WorkDestroyer.
func (d *DefaultWorkDestroyer) DestroyWork(ctx context.Context, workID string, w io.Writer) error {
	return d.workService.DestroyWork(ctx, workID, w)
}

// ControlPlane manages the execution of scheduled tasks with injectable dependencies.
// It allows for testing without actual CLI tools, services, or file system operations.
type ControlPlane struct {
	Git               git.Operations
	Worktree          worktree.Operations
	Zellij            zellij.SessionManager
	Mise              func(dir string) mise.Operations
	FeedbackProcessor feedback.Processor
	WorkDestroyer     WorkDestroyer
	GitHubClient      github.ClientInterface
}

// NewControlPlane creates a new ControlPlane with default production dependencies.
func NewControlPlane(proj *project.Project) *ControlPlane {
	return &ControlPlane{
		Git:               git.NewOperations(),
		Worktree:          worktree.NewOperations(),
		Zellij:            zellij.New(),
		Mise:              mise.NewOperations,
		FeedbackProcessor: feedback.NewProcessor(),
		WorkDestroyer:     NewWorkDestroyer(proj),
		GitHubClient:      github.NewClient(),
	}
}

// NewControlPlaneWithDeps creates a new ControlPlane with provided dependencies for testing.
func NewControlPlaneWithDeps(
	gitOps git.Operations,
	wtOps worktree.Operations,
	zellijMgr zellij.SessionManager,
	miseOps func(dir string) mise.Operations,
	feedbackProc feedback.Processor,
	workDestroyer WorkDestroyer,
	githubClient github.ClientInterface,
) *ControlPlane {
	return &ControlPlane{
		Git:               gitOps,
		Worktree:          wtOps,
		Zellij:            zellijMgr,
		Mise:              miseOps,
		FeedbackProcessor: feedbackProc,
		WorkDestroyer:     workDestroyer,
		GitHubClient:      githubClient,
	}
}

// GetTaskHandlers returns the task handler map for the control plane.
func (cp *ControlPlane) GetTaskHandlers() map[string]TaskHandler {
	return map[string]TaskHandler{
		db.TaskTypeCreateWorktree:      cp.HandleCreateWorktreeTask,
		db.TaskTypePRFeedback:          cp.HandlePRFeedbackTask,
		db.TaskTypeGitPush:             cp.HandleGitPushTask,
		db.TaskTypeDestroyWorktree:     cp.HandleDestroyWorktreeTask,
		db.TaskTypeWatchWorkflowRun:    cp.HandleWatchWorkflowRunTask,
		// These handlers don't need ControlPlane dependencies - keep as standalone functions
		db.TaskTypeImportPR:            HandleImportPRTask,
		db.TaskTypeCommentResolution:   HandleCommentResolutionTask,
		db.TaskTypeGitHubComment:       HandleGitHubCommentTask,
		db.TaskTypeGitHubResolveThread: HandleGitHubResolveThreadTask,
	}
}
