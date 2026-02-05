package beads

//go:generate moq -stub -out beads_mock.go . CLI:BeadsCLIMock Reader:BeadsReaderMock

import (
	"context"
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
	return Create(ctx, c.beadsDir, opts)
}

// Close implements CLI.Close.
func (c *cliImpl) Close(ctx context.Context, beadID string) error {
	return Close(ctx, beadID, c.beadsDir)
}

// Delete implements CLI.Delete.
func (c *cliImpl) Delete(ctx context.Context, beadID string, force bool) error {
	return Delete(ctx, beadID, c.beadsDir, force)
}

// Reopen implements CLI.Reopen.
func (c *cliImpl) Reopen(ctx context.Context, beadID string) error {
	return Reopen(ctx, beadID, c.beadsDir)
}

// Update implements CLI.Update.
func (c *cliImpl) Update(ctx context.Context, beadID string, opts UpdateOptions) error {
	return Update(ctx, beadID, c.beadsDir, opts)
}

// AddComment implements CLI.AddComment.
func (c *cliImpl) AddComment(ctx context.Context, beadID, comment string) error {
	return AddComment(ctx, beadID, comment, c.beadsDir)
}

// AddLabels implements CLI.AddLabels.
func (c *cliImpl) AddLabels(ctx context.Context, beadID string, labels []string) error {
	return AddLabels(ctx, beadID, c.beadsDir, labels)
}

// SetExternalRef implements CLI.SetExternalRef.
func (c *cliImpl) SetExternalRef(ctx context.Context, beadID, externalRef string) error {
	return SetExternalRef(ctx, beadID, externalRef, c.beadsDir)
}

// AddDependency implements CLI.AddDependency.
func (c *cliImpl) AddDependency(ctx context.Context, beadID, dependsOnID string) error {
	return AddDependency(ctx, beadID, dependsOnID, c.beadsDir)
}

// Compile-time check that Client implements Reader.
var _ Reader = (*Client)(nil)
