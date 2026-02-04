package control

import (
	"context"
	"fmt"

	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/logging"
	"github.com/sargehq/sarge/internal/project"
	"github.com/sargehq/sarge/internal/zellij"
)

// ControlPlaneTabName is the name of the control plane tab in zellij
const ControlPlaneTabName = "control"

// InitResult contains information about session initialization
type InitResult struct {
	// SessionCreated is true if a new zellij session was created
	SessionCreated bool
	// SessionName is the name of the zellij session (e.g., "co-myproject")
	SessionName string
}

// EnsureControlPlane ensures the zellij session and control plane are running.
// Creates the session if needed, spawns control plane if missing, restarts if dead.
// Returns information about whether a new session was created.
func EnsureControlPlane(ctx context.Context, proj *project.Project) (*InitResult, error) {
	projectName := proj.Config.Project.Name
	sessionName := project.SessionNameForProject(projectName)
	zc := zellij.New()

	result := &InitResult{
		SessionName: sessionName,
	}

	// Ensure session exists with control plane as the initial tab
	sessionCreated, err := zc.EnsureSessionWithCommand(ctx, sessionName, ControlPlaneTabName, proj.Root, "sarge", []string{"control", "--root", proj.Root})
	if err != nil {
		return nil, fmt.Errorf("failed to ensure zellij session: %w", err)
	}
	result.SessionCreated = sessionCreated

	if sessionCreated {
		logging.Debug("New zellij session created with control plane", "sessionName", sessionName)
		return result, nil
	}

	// Session existed - check if control plane tab exists
	zellijSession := zc.Session(sessionName)
	tabExists, err := zellijSession.TabExists(ctx, ControlPlaneTabName)
	if err != nil {
		return nil, fmt.Errorf("failed to check tab existence: %w", err)
	}
	if !tabExists {
		// No tab - spawn control plane
		if err := spawnControlPlane(ctx, proj); err != nil {
			return nil, err
		}
		return result, nil
	}

	// Tab exists - check if control plane has a recent heartbeat
	alive, err := proj.DB.IsControlPlaneAlive(ctx, db.DefaultStalenessThreshold)
	if err != nil {
		return nil, fmt.Errorf("failed to check control plane status: %w", err)
	}
	if alive {
		// Control plane is alive
		return result, nil
	}

	// Tab exists but process is dead - restart
	logging.Debug("Control plane tab exists but process is dead - restarting...")

	// Try to close the dead tab first
	_ = zellijSession.TerminateAndCloseTab(ctx, ControlPlaneTabName)

	// Spawn a new one
	if err := spawnControlPlane(ctx, proj); err != nil {
		return nil, err
	}
	return result, nil
}

// spawnControlPlane spawns the control plane in a zellij tab.
// The session must already exist.
func spawnControlPlane(ctx context.Context, proj *project.Project) error {
	projectName := proj.Config.Project.Name
	projectRoot := proj.Root
	sessionName := project.SessionNameForProject(projectName)
	zc := zellij.New()

	logging.Debug("spawnControlPlane started", "sessionName", sessionName, "projectRoot", projectRoot)

	// Check if control plane tab already exists
	session := zc.Session(sessionName)
	tabExists, _ := session.TabExists(ctx, ControlPlaneTabName)
	logging.Debug("spawnControlPlane TabExists check", "tabExists", tabExists)
	if tabExists {
		return nil
	}

	// Create control plane tab with command using layout
	// This avoids race conditions from creating a tab then executing a command
	logging.Debug("spawnControlPlane creating tab with command", "tabName", ControlPlaneTabName)
	if err := session.CreateTabWithCommand(ctx, ControlPlaneTabName, projectRoot, "sarge", []string{"control", "--root", projectRoot}, "control"); err != nil {
		logging.Error("spawnControlPlane CreateTabWithCommand failed", "error", err)
		return fmt.Errorf("failed to create control plane tab: %w", err)
	}
	logging.Debug("spawnControlPlane completed successfully")

	return nil
}
