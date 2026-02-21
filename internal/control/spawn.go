package control

import (
	"context"
	"fmt"

	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/logging"
	"github.com/sargehq/sarge/internal/project"
	"github.com/sargehq/sarge/internal/zellij"
	"github.com/sargehq/sarge/internal/zmx"
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

// EnsureControlPlane ensures the control plane is running.
// For zellij: creates session if needed, spawns control plane tab if missing, restarts if dead.
// For zmx: creates control plane session if needed, restarts if dead.
// Returns information about whether a new session was created.
func EnsureControlPlane(ctx context.Context, proj *project.Project) (*InitResult, error) {
	if proj.Config.Multiplexer.IsZmx() {
		return ensureControlPlaneZmx(ctx, proj)
	}
	return ensureControlPlaneZellij(ctx, proj)
}

// ensureControlPlaneZmx ensures the control plane is running as a zmx session.
func ensureControlPlaneZmx(ctx context.Context, proj *project.Project) (*InitResult, error) {
	return ensureControlPlaneZmxWith(ctx, proj, zmx.New())
}

// ensureControlPlaneZmxWith is the implementation of ensureControlPlaneZmx that accepts
// an injected zmx client for testability.
func ensureControlPlaneZmxWith(ctx context.Context, proj *project.Project, zmxClient zmx.Client) (*InitResult, error) {
	projectName := proj.Config.Project.Name
	zmxName := zmx.SessionName(projectName, ControlPlaneTabName)

	logging.Debug("ensureControlPlaneZmxWith called",
		"projectName", projectName, "zmxName", zmxName, "projectRoot", proj.Root)

	result := &InitResult{
		SessionName: zmxName,
	}

	// Check if control plane session exists
	exists, err := zmxClient.SessionExists(ctx, zmxName)
	if err != nil {
		return nil, fmt.Errorf("failed to check zmx session: %w", err)
	}

	logging.Debug("ensureControlPlaneZmxWith session check", "zmxName", zmxName, "exists", exists)

	if !exists {
		// Create control plane session
		logging.Debug("Creating zmx control plane session", "zmxName", zmxName, "root", proj.Root)
		if err := zmxClient.RunSession(ctx, zmxName, "sarge", []string{"control", "--root", proj.Root}, proj.Root); err != nil {
			logging.Error("Failed to create zmx control plane session", "zmxName", zmxName, "error", err)
			return nil, fmt.Errorf("failed to create zmx control plane session: %w", err)
		}
		logging.Debug("zmx control plane session created successfully", "zmxName", zmxName)
		result.SessionCreated = true
		return result, nil
	}

	// Session exists - check if control plane has a recent heartbeat
	alive, err := proj.DB.IsControlPlaneAlive(ctx, db.DefaultStalenessThreshold)
	if err != nil {
		return nil, fmt.Errorf("failed to check control plane status: %w", err)
	}
	logging.Debug("ensureControlPlaneZmxWith heartbeat check", "zmxName", zmxName, "alive", alive)
	if alive {
		return result, nil
	}

	// Session exists but process is dead - kill and restart
	logging.Debug("Control plane zmx session exists but process is dead - restarting...", "zmxName", zmxName)
	_ = zmxClient.KillSession(ctx, zmxName)

	if err := zmxClient.RunSession(ctx, zmxName, "sarge", []string{"control", "--root", proj.Root}, proj.Root); err != nil {
		logging.Error("Failed to restart zmx control plane session", "zmxName", zmxName, "error", err)
		return nil, fmt.Errorf("failed to restart zmx control plane session: %w", err)
	}
	logging.Debug("zmx control plane session restarted successfully", "zmxName", zmxName)
	result.SessionCreated = true
	return result, nil
}

// ensureControlPlaneZellij ensures the control plane is running in a zellij session.
func ensureControlPlaneZellij(ctx context.Context, proj *project.Project) (*InitResult, error) {
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
