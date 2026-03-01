package cmd

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"github.com/sargehq/sarge/internal/control"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/debug"
	"github.com/sargehq/sarge/internal/procmon"
	"github.com/sargehq/sarge/internal/project"
	"github.com/spf13/cobra"
)

// ControlPlaneTabName is the name of the control plane tab in zellij
const ControlPlaneTabName = control.ControlPlaneTabName

var controlCmd = &cobra.Command{
	Use:   "control",
	Short: "[Deprecated] Run the control plane as a standalone process",
	Long: `[Deprecated] The control plane now runs as an in-process goroutine inside the TUI.
This command is kept as a debug fallback for running it as a separate process.`,
	Hidden: true,
	RunE:   runControlPlane,
}

var controlRoot string

func init() {
	rootCmd.AddCommand(controlCmd)
	controlCmd.Flags().StringVar(&controlRoot, "root", "", "Project root directory")
}

func runControlPlane(cmd *cobra.Command, args []string) error {
	ctx := GetContext()

	proj, err := project.Find(ctx, controlRoot)
	if err != nil {
		return fmt.Errorf("not in a project directory: %w", err)
	}
	defer proj.Close()

	// Apply hooks.env to current process - inherited by child processes
	applyHooksEnv(proj.Config.Hooks.Env)

	// Set BEANS_PATH so beans commands work in any spawned processes
	_ = os.Setenv("BEANS_PATH", proj.BeansPath())

	// Register this control plane process for heartbeat monitoring
	procManager := procmon.NewManager(proj.DB, db.DefaultHeartbeatInterval)
	if err := procManager.RegisterControlPlane(ctx); err != nil {
		return fmt.Errorf("failed to register control plane: %w", err)
	}
	defer procManager.Stop()

	// Start pprof server if enabled in config
	if proj.Config.Debug.Pprof {
		pprofPort, err := debug.StartPprof()
		if err != nil {
			return fmt.Errorf("failed to start pprof: %w", err)
		}
		if err := procManager.SetPprofPort(ctx, &pprofPort); err != nil {
			return fmt.Errorf("failed to set pprof port: %w", err)
		}
	}

	fmt.Println("=== Control Plane Started ===")
	fmt.Printf("Project: %s\n", proj.Config.Project.Name)
	fmt.Println("Watching for scheduled tasks across all works...")

	// Start the control plane loop
	err = control.RunControlPlaneLoop(ctx, proj, procManager)
	if errors.Is(err, control.ErrBinaryChanged) {
		// Re-exec the new binary to pick up the update.
		// Resolve the path fresh — the binary on disk is the new one.
		exePath, execErr := os.Executable()
		if execErr != nil {
			return fmt.Errorf("failed to resolve executable for restart: %w", execErr)
		}
		fmt.Printf("Re-executing: %s\n", exePath)
		// Close project DB before exec so the new process gets a clean connection
		proj.Close()
		procManager.Stop()
		return syscall.Exec(exePath, os.Args, os.Environ()) //nolint:gosec // exePath is from os.Executable(), not user input
	}
	return err
}
