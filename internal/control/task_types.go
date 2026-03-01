package control

import "github.com/sargehq/sarge/internal/db"

// TaskExecutionMode determines how a task handler is executed.
type TaskExecutionMode int

const (
	// TaskModeSync executes task handlers synchronously in the main loop.
	// Use for fast operations that complete in milliseconds.
	TaskModeSync TaskExecutionMode = iota

	// TaskModeAsync executes task handlers in a goroutine.
	// Use for long-running operations that may block for seconds or minutes.
	TaskModeAsync
)

// taskExecutionModes maps task types to their execution mode.
// By default, tasks run synchronously for simplicity and predictability.
// Only long-running tasks that block the event loop should be async.
var taskExecutionModes = map[string]TaskExecutionMode{
	// Sync tasks (fast, < 1 second typically)
	db.TaskTypeCreateWorktree:      TaskModeSync, // Creates git worktree, fast
	db.TaskTypeDestroyWorktree:     TaskModeSync, // Removes worktree directory, fast
	db.TaskTypeGitPush:             TaskModeSync, // Git push, usually fast
	db.TaskTypeGitHubComment:       TaskModeSync, // Posts a GitHub comment, fast
	db.TaskTypeGitHubResolveThread: TaskModeSync, // Resolves a GitHub thread, fast
	db.TaskTypeCommentResolution:   TaskModeSync, // Posts resolution comment, fast
	db.TaskTypeImportPR:            TaskModeSync, // Imports PR metadata, fast

	// Async tasks (long-running, can block for seconds to minutes)
	db.TaskTypeWatchWorkflowRun: TaskModeAsync, // Blocks until CI completes (minutes)
	db.TaskTypePRFeedback:       TaskModeAsync, // Fetches PR data from GitHub API (can be slow)
}

// GetTaskExecutionMode returns the execution mode for a task type.
// Unknown task types default to sync execution for safety.
func GetTaskExecutionMode(taskType string) TaskExecutionMode {
	if mode, ok := taskExecutionModes[taskType]; ok {
		return mode
	}
	return TaskModeSync // Default to sync for unknown task types
}

// IsAsyncTask returns true if the task type should be executed asynchronously.
func IsAsyncTask(taskType string) bool {
	return GetTaskExecutionMode(taskType) == TaskModeAsync
}
