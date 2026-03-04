package control

import (
	"context"

	"github.com/sargehq/sarge/internal/project"
)

// ControlPlaneTabName is the name used for control plane identification.
const ControlPlaneTabName = "control"

// InitResult contains information about session initialization.
type InitResult struct {
	// SessionCreated is true if a new session was created.
	SessionCreated bool
	// SessionName is the name of the session.
	SessionName string
}

// EnsureControlPlane is a no-op in the single-process architecture.
// The control plane runs as an in-process goroutine started by the TUI.
// This function is kept for backward compatibility with callers that check
// whether the control plane is running.
func EnsureControlPlane(ctx context.Context, proj *project.Project) (*InitResult, error) {
	return &InitResult{
		SessionCreated: false,
		SessionName:    "in-process",
	}, nil
}
