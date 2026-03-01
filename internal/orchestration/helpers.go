package orchestration

import (
	"context"
	"fmt"
	"time"

	"github.com/sargehq/sarge/internal/db"
)

// SpinnerFrames for animated waiting display
var SpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// SpinnerWait displays an animated spinner with a message for the specified duration.
// The spinner updates every 100ms to create a smooth animation effect.
// Does not print a newline so the spinner can continue on the same line.
func SpinnerWait(msg string, duration time.Duration) {
	// Reduce polling intervals - control plane handles scheduler tasks globally
	// Agent monitoring uses database watcher
	// This polling is just a safety net for the main orchestrator loop
	maxDuration := 2 * time.Second
	if duration > maxDuration {
		duration = maxDuration
	}

	start := time.Now()
	frameIdx := 0
	for time.Since(start) < duration {
		fmt.Printf("\r%s %s", SpinnerFrames[frameIdx], msg)
		frameIdx = (frameIdx + 1) % len(SpinnerFrames)
		time.Sleep(100 * time.Millisecond)
	}
	// Don't print newline - let caller decide or let next SpinnerWait overwrite
}

// CountReviewIterations counts how many review iterations have been done for a work.
func CountReviewIterations(ctx context.Context, database *db.DB, workID string) int {
	tasks, err := database.GetWorkTasks(ctx, workID)
	if err != nil {
		return 0
	}

	count := 0
	for _, t := range tasks {
		if t.TaskType == "review" {
			count++
		}
	}
	return count
}
