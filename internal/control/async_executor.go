package control

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/logging"
	"github.com/sargehq/sarge/internal/project"
)

// DefaultMaxConcurrentAsyncTasks is the default limit on concurrent async task execution.
const DefaultMaxConcurrentAsyncTasks = 5

// AsyncTaskExecutor manages concurrent execution of async tasks.
// It uses a semaphore to limit the number of concurrent goroutines.
type AsyncTaskExecutor struct {
	proj         *project.Project
	taskHandlers map[string]TaskHandler
	semaphore    chan struct{}
	wg           sync.WaitGroup
	mu           sync.Mutex
	running      map[string]struct{} // Track task IDs currently running
}

// NewAsyncTaskExecutor creates a new AsyncTaskExecutor with the specified concurrency limit.
func NewAsyncTaskExecutor(proj *project.Project, taskHandlers map[string]TaskHandler, maxConcurrent int) *AsyncTaskExecutor {
	if maxConcurrent <= 0 {
		maxConcurrent = DefaultMaxConcurrentAsyncTasks
	}
	return &AsyncTaskExecutor{
		proj:         proj,
		taskHandlers: taskHandlers,
		semaphore:    make(chan struct{}, maxConcurrent),
		running:      make(map[string]struct{}),
	}
}

// Submit submits an async task for execution in a goroutine.
// Returns true if the task was submitted, false if the task is already running.
// The task must already be marked as "executing" in the database before calling this.
func (e *AsyncTaskExecutor) Submit(ctx context.Context, task *db.ScheduledTask) bool {
	e.mu.Lock()
	if _, exists := e.running[task.ID]; exists {
		e.mu.Unlock()
		logging.Debug("Async task already running, skipping",
			"task_id", task.ID,
			"task_type", task.TaskType)
		return false
	}
	e.running[task.ID] = struct{}{}
	e.mu.Unlock()

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		defer func() {
			e.mu.Lock()
			delete(e.running, task.ID)
			e.mu.Unlock()
		}()

		// Acquire semaphore slot (blocks if at max concurrency)
		select {
		case e.semaphore <- struct{}{}:
			// Acquired slot
		case <-ctx.Done():
			logging.Debug("Context cancelled while waiting for semaphore",
				"task_id", task.ID,
				"task_type", task.TaskType)
			return
		}
		defer func() { <-e.semaphore }()

		// Check context again after acquiring semaphore
		if ctx.Err() != nil {
			return
		}

		logging.Info("Starting async task execution",
			"task_id", task.ID,
			"task_type", task.TaskType,
			"work_id", task.WorkID)

		fmt.Printf("[%s] [async] Starting %s for %s\n",
			time.Now().Format("15:04:05"), task.TaskType, task.WorkID)

		// Execute the handler
		var err error
		if handler, ok := e.taskHandlers[task.TaskType]; ok {
			err = handler(ctx, e.proj, task)
		} else {
			err = fmt.Errorf("unknown task type: %s", task.TaskType)
		}

		// Handle result
		if err != nil {
			fmt.Printf("[%s] [async] Task failed: %s - %s\n",
				time.Now().Format("15:04:05"), task.TaskType, err)
			HandleTaskError(ctx, e.proj, task, err.Error())
		} else {
			fmt.Printf("[%s] [async] Task completed: %s\n",
				time.Now().Format("15:04:05"), task.TaskType)
			if markErr := e.proj.DB.MarkTaskCompleted(ctx, task.ID); markErr != nil {
				logging.Warn("failed to mark async task as completed",
					"error", markErr,
					"task_id", task.ID)
			}
		}
	}()

	return true
}

// Wait blocks until all submitted tasks complete.
// Call this during shutdown to ensure clean termination.
func (e *AsyncTaskExecutor) Wait() {
	e.wg.Wait()
}

// RunningCount returns the number of currently running async tasks.
func (e *AsyncTaskExecutor) RunningCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.running)
}
