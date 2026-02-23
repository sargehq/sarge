package beads

//go:generate moq -stub -out beads_mock.go . CLI:BeadsCLIMock Reader:BeadsReaderMock

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/sargehq/sarge/internal/logging"
)

// CLI defines the interface for bd command operations.
// This abstraction enables testing without actual bd CLI calls.
// Each CLI instance is bound to a specific beads directory.
//
// Note: Init and InstallHooks are package-level functions, not part of this
// interface, since they are setup operations that run before a CLI is created.
type CLI interface {
	// Create creates a new bead and returns its ID.
	Create(ctx context.Context, opts CreateOptions) (string, error)
	// Close closes a bead.
	Close(ctx context.Context, beadID string) error
	// Delete permanently removes a bead.
	Delete(ctx context.Context, beadID string, force bool) error
	// Reopen reopens a closed bead.
	Reopen(ctx context.Context, beadID string) error
	// Update updates a bead's fields.
	Update(ctx context.Context, beadID string, opts UpdateOptions) error
	// AddComment adds a comment to a bead.
	AddComment(ctx context.Context, beadID, comment string) error
	// AddLabels adds labels to a bead.
	AddLabels(ctx context.Context, beadID string, labels []string) error
	// SetExternalRef sets the external reference for a bead.
	SetExternalRef(ctx context.Context, beadID, externalRef string) error
	// AddDependency adds a dependency between two beads.
	AddDependency(ctx context.Context, beadID, dependsOnID string) error
}

// Reader defines the interface for reading beads from the database.
// This abstraction enables testing without actual database access.
type Reader interface {
	// GetBead retrieves a single bead by ID with its dependencies/dependents.
	GetBead(ctx context.Context, id string) (*BeadWithDeps, error)
	// GetBeadsWithDeps retrieves beads and their dependencies/dependents.
	GetBeadsWithDeps(ctx context.Context, beadIDs []string) (*BeadsWithDepsResult, error)
	// ListBeads lists all beads with optional status filter.
	ListBeads(ctx context.Context, status string) ([]Bead, error)
	// GetReadyBeads returns all open beads where all dependencies are satisfied.
	GetReadyBeads(ctx context.Context) ([]Bead, error)
	// GetTransitiveDependencies collects all transitive dependencies for a bead.
	GetTransitiveDependencies(ctx context.Context, id string) ([]Bead, error)
	// GetBeadWithChildren retrieves a bead and all its child beads recursively.
	GetBeadWithChildren(ctx context.Context, id string) ([]Bead, error)
}

// cliImpl implements CLI using the bd command-line tool.
type cliImpl struct {
	beadsDir string
}

// Compile-time check that cliImpl implements CLI.
var _ CLI = (*cliImpl)(nil)

// NewCLI creates a new CLI instance bound to the specified beads directory.
func NewCLI(beadsDir string) CLI {
	return &cliImpl{beadsDir: beadsDir}
}

// Create implements CLI.Create.
func (c *cliImpl) Create(ctx context.Context, opts CreateOptions) (string, error) {
	// Determine the type - if IsEpic is set, override type to "epic"
	beadType := opts.Type
	if opts.IsEpic {
		beadType = "epic"
	}
	args := []string{"create", "--title=" + opts.Title, "--type=" + beadType, fmt.Sprintf("--priority=%d", opts.Priority)}
	if opts.Description != "" {
		args = append(args, "--description="+opts.Description)
	}
	if opts.Parent != "" {
		args = append(args, "--parent="+opts.Parent)
	}
	if opts.ExternalRef != "" {
		args = append(args, "--external-ref="+opts.ExternalRef)
	}
	for _, label := range opts.Labels {
		if label != "" {
			args = append(args, "--label="+label)
		}
	}

	logging.Debug("creating bead", "args", args, "beadsDir", c.beadsDir, "opts", opts)

	cmd := bdCommand(ctx, c.beadsDir, args...)
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			logging.Error("bd create failed", "error", err, "stderr", string(exitErr.Stderr), "args", args)
			return "", fmt.Errorf("failed to create bead: %w\n%s", err, exitErr.Stderr)
		}
		logging.Error("bd create failed", "error", err, "args", args)
		return "", fmt.Errorf("failed to create bead: %w", err)
	}

	logging.Debug("bd create output", "output", string(output))

	// Parse the bead ID from output
	// bd create outputs multi-line text like "✓ Created issue: s-0o9\n   Title: ..."
	// We need to extract just the bead ID (format: prefix-xxx)
	beadID := ""
	outputStr := string(output)
	// Split by whitespace (including newlines) and find the bead ID
	parts := strings.Fields(outputStr)
	for _, p := range parts {
		// Bead IDs are typically short alphanumeric with a dash (e.g., s-0o9, ac-123, bd-456)
		if len(p) >= 3 && len(p) <= 20 && strings.Contains(p, "-") && !strings.HasPrefix(p, "-") {
			// Check it looks like a bead ID (letters/numbers and dashes only)
			isBeadID := true
			for _, ch := range p {
				if (ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') && (ch < '0' || ch > '9') && ch != '-' && ch != '.' {
					isBeadID = false
					break
				}
			}
			if isBeadID {
				beadID = p
				break
			}
		}
	}

	if beadID == "" {
		logging.Error("failed to parse bead ID from output", "output", string(output), "args", args)
		return "", fmt.Errorf("failed to get created bead ID from output: %s", output)
	}

	logging.Debug("created bead", "beadID", beadID)
	return beadID, nil
}

// Close implements CLI.Close.
func (c *cliImpl) Close(ctx context.Context, beadID string) error {
	cmd := bdCommand(ctx, c.beadsDir, "close", beadID)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to close bead %s: %w\n%s", beadID, err, output)
	}
	return nil
}

// Delete implements CLI.Delete.
func (c *cliImpl) Delete(ctx context.Context, beadID string, force bool) error {
	args := []string{"delete", beadID}
	if force {
		args = append(args, "--force")
	}
	cmd := bdCommand(ctx, c.beadsDir, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete bead %s: %w\n%s", beadID, err, output)
	}
	return nil
}

// Reopen implements CLI.Reopen.
func (c *cliImpl) Reopen(ctx context.Context, beadID string) error {
	cmd := bdCommand(ctx, c.beadsDir, "reopen", beadID)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to reopen bead %s: %w\n%s", beadID, err, output)
	}
	return nil
}

// Update implements CLI.Update.
func (c *cliImpl) Update(ctx context.Context, beadID string, opts UpdateOptions) error {
	args := []string{"update", beadID}
	if opts.Title != "" {
		args = append(args, "--title="+opts.Title)
	}
	if opts.Type != "" {
		args = append(args, "--type="+opts.Type)
	}
	if opts.Description != "" {
		args = append(args, "--description="+opts.Description)
	}
	if opts.Assignee != "" {
		args = append(args, "--assignee="+opts.Assignee)
	}
	if opts.Priority != nil {
		args = append(args, fmt.Sprintf("--priority=%d", *opts.Priority))
	}
	if opts.Status != "" {
		args = append(args, "--status="+opts.Status)
	}

	cmd := bdCommand(ctx, c.beadsDir, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to update bead %s: %w\n%s", beadID, err, output)
	}
	return nil
}

// AddComment implements CLI.AddComment.
func (c *cliImpl) AddComment(ctx context.Context, beadID, comment string) error {
	cmd := bdCommand(ctx, c.beadsDir, "comments", "add", beadID, comment)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add comment to bead %s: %w\n%s", beadID, err, output)
	}
	return nil
}

// AddLabels implements CLI.AddLabels.
func (c *cliImpl) AddLabels(ctx context.Context, beadID string, labels []string) error {
	if len(labels) == 0 {
		return nil
	}

	args := []string{"update", beadID}
	for _, label := range labels {
		args = append(args, "--add-label="+label)
	}

	cmd := bdCommand(ctx, c.beadsDir, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add labels to bead %s: %w\n%s", beadID, err, output)
	}
	return nil
}

// SetExternalRef implements CLI.SetExternalRef.
func (c *cliImpl) SetExternalRef(ctx context.Context, beadID, externalRef string) error {
	if externalRef == "" {
		return nil
	}

	args := []string{"update", beadID, "--external-ref=" + externalRef}

	cmd := bdCommand(ctx, c.beadsDir, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set external ref for bead %s: %w\n%s", beadID, err, output)
	}
	return nil
}

// AddDependency implements CLI.AddDependency.
func (c *cliImpl) AddDependency(ctx context.Context, beadID, dependsOnID string) error {
	cmd := bdCommand(ctx, c.beadsDir, "dep", "add", beadID, dependsOnID)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add dependency %s -> %s: %w\n%s", beadID, dependsOnID, err, output)
	}
	return nil
}


