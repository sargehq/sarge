package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/sargehq/sarge/internal/beans/pubsub"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/project"
	trackingwatcher "github.com/sargehq/sarge/internal/tracking/watcher"
)

// MonitorAgent handles the main event loop for monitoring agent execution.
// It watches for agent exit, task completion in database, signals, and context cancellation.
// workDir is used to derive the project root (assumes workDir is <project>/<work-id>/tree/).
func MonitorAgent(ctx context.Context, database *db.DB, taskID string, agentCmd *exec.Cmd, startTime time.Time, workDir string) error {
	projectRoot := filepath.Dir(filepath.Dir(workDir))

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	// Wait for agent to complete
	done := make(chan error, 1)
	go func() {
		done <- agentCmd.Wait()
	}()

	// Try to set up database watcher for event-driven monitoring
	var watcherSub <-chan pubsub.Event[trackingwatcher.WatcherEvent]
	var ticker *time.Ticker

	trackingDBPath := filepath.Join(projectRoot, project.ConfigDir, "tracking.db")
	watcher, err := trackingwatcher.New(trackingwatcher.DefaultConfig(trackingDBPath))
	if err == nil {
		if err := watcher.Start(); err == nil {
			defer watcher.Stop()
			watcherSub = watcher.Broker().Subscribe(ctx)
			fmt.Printf("Using database watcher for task monitoring\n")
		}
	}

	// Fall back to polling if watcher setup failed
	if watcherSub == nil {
		ticker = time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		fmt.Printf("Using polling for task monitoring (2s interval)\n")
	}

	// Helper function to check task status
	checkTaskStatus := func() error {
		task, err := database.GetTask(ctx, taskID)
		if err != nil {
			fmt.Printf("Warning: failed to check task status: %v\n", err)
			return nil // continue monitoring
		}
		if task == nil {
			fmt.Printf("\nTask %s no longer exists, terminating agent...\n", taskID)
			terminateGracefully(agentCmd, done)
			return fmt.Errorf("task %s was deleted", taskID)
		}
		if task.Status == db.StatusCompleted || task.Status == db.StatusFailed {
			fmt.Printf("\nTask marked as %s in database, terminating agent...\n", task.Status)
			terminateGracefully(agentCmd, done)
			elapsed := time.Since(startTime)
			fmt.Printf("\n=== Task %s %s (took %s) ===\n", taskID, task.Status, elapsed.Round(time.Second))
			return fmt.Errorf("task_status_changed")
		}
		return nil
	}

	for {
		select {
		case err := <-done:
			return handleAgentExit(ctx, database, taskID, err, startTime)

		case event, ok := <-watcherSub:
			if !ok {
				watcherSub = nil
				continue
			}
			if event.Payload.Type == trackingwatcher.DBChanged {
				if err := checkTaskStatus(); err != nil {
					if err.Error() == "task_status_changed" {
						return nil
					}
					return err
				}
			}

		case <-func() <-chan time.Time {
			if ticker != nil {
				return ticker.C
			}
			return nil
		}():
			if err := checkTaskStatus(); err != nil {
				if err.Error() == "task_status_changed" {
					return nil
				}
				return err
			}

		case sig := <-sigChan:
			fmt.Printf("\nReceived signal %v, forwarding to agent...\n", sig)
			if sysSig, ok := sig.(syscall.Signal); ok {
				agentCmd.Process.Signal(sysSig)
			} else {
				agentCmd.Process.Signal(syscall.SIGTERM)
			}
			terminateGracefully(agentCmd, done)
			return fmt.Errorf("interrupted by signal %v", sig)

		case <-ctx.Done():
			fmt.Println("\nContext cancelled, terminating agent...")
			terminateGracefully(agentCmd, done)
			return ctx.Err()
		}
	}
}

func terminateGracefully(cmd *exec.Cmd, done <-chan error) {
	cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		fmt.Println("Agent didn't exit gracefully, force killing...")
		cmd.Process.Kill()
		<-done
	}
}

func handleAgentExit(ctx context.Context, database *db.DB, taskID string, exitErr error, startTime time.Time) error {
	elapsed := time.Since(startTime)

	if exitErr != nil {
		task, dbErr := database.GetTask(ctx, taskID)
		if dbErr == nil && task != nil && (task.Status == db.StatusCompleted || task.Status == db.StatusFailed) {
			fmt.Printf("\n=== Task %s %s (took %s) ===\n", taskID, task.Status, elapsed.Round(time.Second))
			return nil
		}
		if dbErr := database.FailTask(ctx, taskID, fmt.Sprintf("agent exited with error: %v", exitErr)); dbErr != nil {
			fmt.Printf("Warning: failed to mark task as failed: %v\n", dbErr)
		}
		return fmt.Errorf("agent exited with error: %w", exitErr)
	}

	task, err := database.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to get task %s: %w", taskID, err)
	}
	if task != nil && task.Status == db.StatusCompleted {
		fmt.Printf("\n=== Task %s completed (took %s) ===\n", taskID, elapsed.Round(time.Second))
	} else if task != nil && task.Status == db.StatusFailed {
		fmt.Printf("\n=== Task %s failed (took %s) ===\n", taskID, elapsed.Round(time.Second))
	} else if task != nil && task.Status == db.StatusProcessing {
		// Agent exited cleanly but never called sarge complete — mark as failed
		fmt.Printf("\n=== Agent exited without completing task %s (took %s), marking as failed ===\n", taskID, elapsed.Round(time.Second))
		if dbErr := database.FailTask(ctx, taskID, "agent exited without completing task"); dbErr != nil {
			fmt.Printf("Warning: failed to mark task as failed: %v\n", dbErr)
		}
	} else {
		fmt.Printf("\n=== Agent exited for task %s (took %s) ===\n", taskID, elapsed.Round(time.Second))
	}
	return nil
}
