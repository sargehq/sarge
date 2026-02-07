package agents

//go:generate moq -stub -out agents_mock.go . Agent:AgentMock

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/sargehq/sarge/internal/beads/pubsub"
	"github.com/sargehq/sarge/internal/db"
	trackingwatcher "github.com/sargehq/sarge/internal/tracking/watcher"
)

// monitorAgent handles the main event loop for monitoring agent execution.
// It watches for agent exit, task completion in database, signals, and context cancellation.
func monitorAgent(ctx context.Context, database *db.DB, taskID string, agentCmd *exec.Cmd, startTime time.Time, projectRoot string) error {
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

	trackingDBPath := filepath.Join(projectRoot, ".co", "tracking.db")
	watcher, err := trackingwatcher.New(trackingwatcher.DefaultConfig(trackingDBPath))
	if err == nil {
		if err := watcher.Start(); err == nil {
			defer watcher.Stop()
			// Subscribe to watcher events
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
			return fmt.Errorf("task_status_changed") // Special error to indicate normal completion
		}
		return nil
	}

	for {
		select {
		case err := <-done:
			// Agent exited on its own - no termination needed
			return handleAgentExit(ctx, database, taskID, err, startTime)

		case event, ok := <-watcherSub:
			if !ok {
				// Watcher closed, continue without it
				watcherSub = nil
				continue
			}
			// Database changed event
			if event.Payload.Type == trackingwatcher.DBChanged {
				if err := checkTaskStatus(); err != nil {
					if err.Error() == "task_status_changed" {
						return nil // Normal completion
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
			// Polling fallback
			if err := checkTaskStatus(); err != nil {
				if err.Error() == "task_status_changed" {
					return nil // Normal completion
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

// terminateGracefully sends SIGTERM and waits for exit, force killing after 5 seconds.
func terminateGracefully(cmd *exec.Cmd, done <-chan error) {
	cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-done:
		// Exited gracefully
	case <-time.After(5 * time.Second):
		fmt.Println("Agent didn't exit gracefully, force killing...")
		cmd.Process.Kill()
		<-done
	}
}

// handleAgentExit processes the agent's exit and returns the appropriate result.
func handleAgentExit(ctx context.Context, database *db.DB, taskID string, exitErr error, startTime time.Time) error {
	elapsed := time.Since(startTime)

	if exitErr != nil {
		// Check if it was killed by us due to completion
		task, dbErr := database.GetTask(ctx, taskID)
		if dbErr == nil && task != nil && (task.Status == db.StatusCompleted || task.Status == db.StatusFailed) {
			fmt.Printf("\n=== Task %s %s (took %s) ===\n", taskID, task.Status, elapsed.Round(time.Second))
			return nil
		}
		// Actual error
		if dbErr := database.FailTask(ctx, taskID, fmt.Sprintf("agent exited with error: %v", exitErr)); dbErr != nil {
			fmt.Printf("Warning: failed to mark task as failed: %v\n", dbErr)
		}
		return fmt.Errorf("agent exited with error: %w", exitErr)
	}

	// Agent exited successfully - check task status
	task, err := database.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to get task %s: %w", taskID, err)
	}
	if task != nil && task.Status == db.StatusCompleted {
		fmt.Printf("\n=== Task %s completed (took %s) ===\n", taskID, elapsed.Round(time.Second))
	} else if task != nil && task.Status == db.StatusFailed {
		fmt.Printf("\n=== Task %s failed (took %s) ===\n", taskID, elapsed.Round(time.Second))
	} else {
		fmt.Printf("\n=== Agent exited for task %s (took %s) ===\n", taskID, elapsed.Round(time.Second))
	}
	return nil
}
