package beads

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/sargehq/sarge/internal/mise"
)

// bdCommand creates an exec.Cmd for running bd with BEADS_DIR set.
// The beadsDir parameter should be the path to the .beads directory.
func bdCommand(ctx context.Context, beadsDir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "bd", args...)
	if beadsDir != "" {
		cmd.Env = append(os.Environ(), "BEADS_DIR="+beadsDir)
	}
	return cmd
}

// Init initializes beads in the specified directory.
// beadsDir should be the path where .beads/ should be created (e.g., /path/to/.beads).
// prefix is the issue ID prefix (e.g., "myproject" for myproject-1, myproject-2).
func Init(_ context.Context, beadsDir, prefix string) error {
	parentDir := filepath.Dir(beadsDir)
	if _, err := mise.Exec(parentDir, "bd", "init", "--prefix", prefix); err != nil {
		return fmt.Errorf("bd init failed: %w", err)
	}
	return nil
}

// InstallHooks installs beads hooks in the specified directory.
func InstallHooks(_ context.Context, repoDir string) error {
	if _, err := mise.Exec(repoDir, "bd", "hooks", "install"); err != nil {
		return fmt.Errorf("bd hooks install failed: %w", err)
	}
	return nil
}

// Reinit regenerates the beads database from existing JSONL files.
func Reinit(_ context.Context, repoDir string) error {
	if _, err := mise.Exec(repoDir, "bd", "init"); err != nil {
		return fmt.Errorf("bd init failed: %w", err)
	}
	return nil
}

// CreateOptions specifies options for creating a bead.
type CreateOptions struct {
	Title       string
	Type        string   // "task", "bug", "feature"
	Priority    int
	IsEpic      bool
	Description string
	Parent      string   // Parent bead ID for hierarchical child
	Labels      []string // Optional labels for the bead
	ExternalRef string   // Optional external reference (e.g., GitHub comment ID)
}

// UpdateOptions specifies options for updating a bead.
type UpdateOptions struct {
	Title       string
	Type        string
	Description string
	Assignee    string
	Priority    *int // nil means don't update
	Status      string
}

// EditCommand returns an exec.Cmd for opening a bead in an editor.
// This is meant to be used with tea.ExecProcess for interactive editing.
func EditCommand(ctx context.Context, beadID, beadsDir string) *exec.Cmd {
	return bdCommand(ctx, beadsDir, "edit", beadID)
}

// Dependency represents a dependency relationship between beads.
type Dependency struct {
	IssueID     string
	DependsOnID string
	Type        string // "blocks", "blocked_by", "parent-child", "relates-to"
	Status      string // status of the depended-on issue
	Title       string // title of the depended-on issue
}

// Dependent represents a bead that depends on another bead.
type Dependent struct {
	IssueID     string // the issue that depends on us
	DependsOnID string
	Type        string
	Status      string // status of the dependent issue
	Title       string // title of the dependent issue
}

// BeadWithDeps bundles a bead with its dependencies and dependents.
type BeadWithDeps struct {
	*Bead
	Dependencies []Dependency
	Dependents   []Dependent
}

// BeadsWithDepsResult holds the result of GetBeadsWithDeps.
type BeadsWithDepsResult struct {
	Beads        map[string]Bead
	Dependencies map[string][]Dependency
	Dependents   map[string][]Dependent
}

// GetBead returns a single BeadWithDeps from the result, or nil if not found.
func (r *BeadsWithDepsResult) GetBead(id string) *BeadWithDeps {
	bead, ok := r.Beads[id]
	if !ok {
		return nil
	}
	return &BeadWithDeps{
		Bead:         &bead,
		Dependencies: r.Dependencies[id],
		Dependents:   r.Dependents[id],
	}
}
