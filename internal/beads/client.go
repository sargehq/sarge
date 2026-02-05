package beads

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/sargehq/sarge/internal/beads/cachemanager"
	"github.com/sargehq/sarge/internal/beads/queries"
	"github.com/sargehq/sarge/internal/logging"
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
// bd init runs in the parent directory and creates .beads/ there.
// Uses mise exec to run bd commands since mise-managed tools may not be in PATH.
func Init(_ context.Context, beadsDir, prefix string) error {
	// bd init creates .beads/ in the current working directory, not via BEADS_DIR
	parentDir := filepath.Dir(beadsDir)
	if _, err := mise.Exec(parentDir, "bd", "init", "--prefix", prefix); err != nil {
		return fmt.Errorf("bd init failed: %w", err)
	}
	return nil
}

// InstallHooks installs beads hooks in the specified directory.
// repoDir should be the path to the git repository (e.g., /path/to/repo).
// Uses mise exec to run bd commands since mise-managed tools may not be in PATH.
func InstallHooks(_ context.Context, repoDir string) error {
	if _, err := mise.Exec(repoDir, "bd", "hooks", "install"); err != nil {
		return fmt.Errorf("bd hooks install failed: %w", err)
	}
	return nil
}

// Reinit regenerates the beads database from existing JSONL files.
// repoDir should be the path to the repository containing .beads/ directory.
// This is used when the .beads directory already exists and we just need to rebuild the database.
// Uses mise exec to run bd commands since mise-managed tools may not be in PATH.
func Reinit(_ context.Context, repoDir string) error {
	if _, err := mise.Exec(repoDir, "bd", "init"); err != nil {
		return fmt.Errorf("bd init failed: %w", err)
	}
	return nil
}

// CloseEligibleParents closes any parent beads where all children are complete.
// Uses cached bead data to find eligible parents, then closes them via bd CLI.
// Works for any parent-child relationship, not just epics.
func (c *Client) CloseEligibleParents(ctx context.Context, beadsDir string) error {
	// Get all bead IDs
	allIDs, err := c.queries.GetAllIssueIDs(ctx)
	if err != nil {
		return fmt.Errorf("fetching all bead IDs: %w", err)
	}

	if len(allIDs) == 0 {
		return nil
	}

	// Get all beads with dependencies
	result, err := c.GetBeadsWithDeps(ctx, allIDs)
	if err != nil {
		return fmt.Errorf("fetching beads with deps: %w", err)
	}

	// Find parents that are eligible for closing (all children closed)
	closed := make(map[string]bool)
	for {
		var toClose []string

		for id, bead := range result.Beads {
			// Skip already closed beads
			if bead.Status == "closed" || closed[id] {
				continue
			}

			// Check if this bead has children (is a parent)
			dependents := result.Dependents[id]
			var children []string
			for _, dep := range dependents {
				if dep.Type == "parent-child" {
					children = append(children, dep.IssueID)
				}
			}

			if len(children) == 0 {
				continue // Not a parent
			}

			// Check if all children are closed
			allChildrenClosed := true
			for _, childID := range children {
				child, ok := result.Beads[childID]
				if !ok || (child.Status != "closed" && !closed[childID]) {
					allChildrenClosed = false
					break
				}
			}

			if allChildrenClosed {
				toClose = append(toClose, id)
			}
		}

		if len(toClose) == 0 {
			break // No more parents to close
		}

		// Close eligible parents
		for _, id := range toClose {
			if err := Close(ctx, id, beadsDir); err != nil {
				logging.Warn("failed to close parent bead", "id", id, "error", err)
				continue
			}
			closed[id] = true
			logging.Debug("closed parent bead", "id", id)
		}
	}

	return nil
}

// CreateOptions specifies options for creating a bead.
type CreateOptions struct {
	Title        string
	Type         string   // "task", "bug", "feature"
	Priority     int
	IsEpic       bool
	Description  string
	Parent       string   // Parent bead ID for hierarchical child
	Labels       []string // Optional labels for the bead
	ExternalRef  string   // Optional external reference (e.g., GitHub comment ID)
}

// Create creates a new bead and returns its ID.
func Create(ctx context.Context, beadsDir string, opts CreateOptions) (string, error) {
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

	logging.Debug("creating bead", "args", args, "beadsDir", beadsDir, "opts", opts)

	cmd := bdCommand(ctx, beadsDir, args...)
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
			for _, c := range p {
				if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '-' && c != '.' {
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

// Close closes a bead.
func Close(ctx context.Context, beadID, beadsDir string) error {
	cmd := bdCommand(ctx, beadsDir, "close", beadID)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to close bead %s: %w\n%s", beadID, err, output)
	}
	return nil
}

// Delete permanently removes a bead.
func Delete(ctx context.Context, beadID, beadsDir string, force bool) error {
	args := []string{"delete", beadID}
	if force {
		args = append(args, "--force")
	}
	cmd := bdCommand(ctx, beadsDir, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete bead %s: %w\n%s", beadID, err, output)
	}
	return nil
}

// AddComment adds a comment to a bead.
func AddComment(ctx context.Context, beadID, comment, beadsDir string) error {
	cmd := bdCommand(ctx, beadsDir, "comments", "add", beadID, comment)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add comment to bead %s: %w\n%s", beadID, err, output)
	}
	return nil
}

// Reopen reopens a closed bead.
func Reopen(ctx context.Context, beadID, beadsDir string) error {
	cmd := bdCommand(ctx, beadsDir, "reopen", beadID)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to reopen bead %s: %w\n%s", beadID, err, output)
	}
	return nil
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

// Update updates a bead's fields.
func Update(ctx context.Context, beadID, beadsDir string, opts UpdateOptions) error {
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

	cmd := bdCommand(ctx, beadsDir, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to update bead %s: %w\n%s", beadID, err, output)
	}
	return nil
}

// AddLabels adds labels to a bead.
func AddLabels(ctx context.Context, beadID, beadsDir string, labels []string) error {
	if len(labels) == 0 {
		return nil
	}

	args := []string{"update", beadID}
	for _, label := range labels {
		args = append(args, "--add-label="+label)
	}

	cmd := bdCommand(ctx, beadsDir, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add labels to bead %s: %w\n%s", beadID, err, output)
	}
	return nil
}

// SetExternalRef sets the external reference for a bead.
func SetExternalRef(ctx context.Context, beadID, externalRef, beadsDir string) error {
	if externalRef == "" {
		return nil
	}

	args := []string{"update", beadID, "--external-ref=" + externalRef}

	cmd := bdCommand(ctx, beadsDir, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set external ref for bead %s: %w\n%s", beadID, err, output)
	}
	return nil
}

// AddDependency adds a dependency between two beads.
// The bead identified by beadID will depend on the bead identified by dependsOnID.
func AddDependency(ctx context.Context, beadID, dependsOnID, beadsDir string) error {
	cmd := bdCommand(ctx, beadsDir, "dep", "add", beadID, dependsOnID)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add dependency %s -> %s: %w\n%s", beadID, dependsOnID, err, output)
	}
	return nil
}

// EditCommand returns an exec.Cmd for opening a bead in an editor.
// This is meant to be used with tea.ExecProcess for interactive editing.
func EditCommand(ctx context.Context, beadID, beadsDir string) *exec.Cmd {
	return bdCommand(ctx, beadsDir, "edit", beadID)
}

// Client provides database access to beads with caching.
type Client struct {
	db           *sql.DB
	queries      *queries.Queries
	cache        cachemanager.CacheManager[string, *BeadsWithDepsResult]
	dbPath       string
	cacheEnabled bool
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

// ClientConfig holds configuration for the Client.
type ClientConfig struct {
	DBPath           string
	CacheEnabled     bool
	CacheExpiration  time.Duration
	CacheCleanupTime time.Duration
}

// DefaultClientConfig returns default configuration for the Client.
func DefaultClientConfig(dbPath string) ClientConfig {
	return ClientConfig{
		DBPath:           dbPath,
		CacheEnabled:     true,
		CacheExpiration:  10 * time.Minute,
		CacheCleanupTime: 30 * time.Minute,
	}
}

// NewClient creates a new beads database client.
func NewClient(ctx context.Context, cfg ClientConfig) (*Client, error) {
	// Open database in read-only mode
	db, err := sql.Open("sqlite3", "file:"+cfg.DBPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("opening beads database: %w", err)
	}

	// Test connection
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging beads database: %w", err)
	}

	var cache cachemanager.CacheManager[string, *BeadsWithDepsResult]
	if cfg.CacheEnabled {
		cache = cachemanager.NewInMemoryCacheManager[string, *BeadsWithDepsResult](
			"beads-issues",
			cfg.CacheExpiration,
			cfg.CacheCleanupTime,
		)
	}

	return &Client{
		db:           db,
		queries:      queries.New(db),
		cache:        cache,
		dbPath:       cfg.DBPath,
		cacheEnabled: cfg.CacheEnabled,
	}, nil
}

// Close closes the database connection.
func (c *Client) Close() error {
	return c.db.Close()
}

// FlushCache flushes the cache.
func (c *Client) FlushCache(ctx context.Context) error {
	if c.cache == nil {
		return nil
	}
	return c.cache.Flush(ctx)
}

// GetBeadsWithDeps retrieves beads and their dependencies/dependents.
// Results are cached based on sorted bead IDs.
func (c *Client) GetBeadsWithDeps(ctx context.Context, beadIDs []string) (*BeadsWithDepsResult, error) {
	if len(beadIDs) == 0 {
		return &BeadsWithDepsResult{
			Beads:        make(map[string]Bead),
			Dependencies: make(map[string][]Dependency),
			Dependents:   make(map[string][]Dependent),
		}, nil
	}

	// Create cache key from sorted bead IDs
	sortedIDs := make([]string, len(beadIDs))
	copy(sortedIDs, beadIDs)
	sort.Strings(sortedIDs)
	cacheKey := strings.Join(sortedIDs, ",")

	// Check cache
	if c.cacheEnabled && c.cache != nil {
		if cached, found := c.cache.Get(ctx, cacheKey); found {
			return cached, nil
		}
	}

	// Fetch issues from database
	issues, err := c.queries.GetIssuesByIDs(ctx, beadIDs)
	if err != nil {
		return nil, fmt.Errorf("fetching beads: %w", err)
	}

	// Build beads map
	beadsMap := make(map[string]Bead, len(issues))
	for _, issue := range issues {
		beadsMap[issue.ID] = BeadFromIssue(issue)
	}

	// Fetch dependencies
	deps, err := c.queries.GetDependenciesForIssues(ctx, beadIDs)
	if err != nil {
		return nil, fmt.Errorf("fetching dependencies: %w", err)
	}

	// Build dependencies map with clean types
	depsMap := make(map[string][]Dependency)
	for _, dep := range deps {
		depsMap[dep.IssueID] = append(depsMap[dep.IssueID], Dependency{
			IssueID:     dep.IssueID,
			DependsOnID: dep.DependsOnID,
			Type:        dep.Type,
			Status:      dep.Status,
			Title:       dep.Title,
		})
	}

	// Fetch dependents
	dependents, err := c.queries.GetDependentsForIssues(ctx, beadIDs)
	if err != nil {
		return nil, fmt.Errorf("fetching dependents: %w", err)
	}

	// Build dependents map with clean types
	dependentsMap := make(map[string][]Dependent)
	for _, dep := range dependents {
		dependentsMap[dep.DependsOnID] = append(dependentsMap[dep.DependsOnID], Dependent{
			IssueID:     dep.IssueID,
			DependsOnID: dep.DependsOnID,
			Type:        dep.Type,
			Status:      dep.Status,
			Title:       dep.Title,
		})
	}

	result := &BeadsWithDepsResult{
		Beads:        beadsMap,
		Dependencies: depsMap,
		Dependents:   dependentsMap,
	}

	// Cache result
	if c.cacheEnabled && c.cache != nil {
		c.cache.Set(ctx, cacheKey, result, cachemanager.DefaultExpiration)
	}

	return result, nil
}

// GetBead retrieves a single bead by ID with its dependencies/dependents.
// Returns nil if the bead is not found.
func (c *Client) GetBead(ctx context.Context, id string) (*BeadWithDeps, error) {
	result, err := c.GetBeadsWithDeps(ctx, []string{id})
	if err != nil {
		return nil, err
	}

	return result.GetBead(id), nil
}

// ListBeads lists all beads with optional status filter.
// Pass empty string for status to get all beads.
func (c *Client) ListBeads(ctx context.Context, status string) ([]Bead, error) {
	var ids []string
	var err error

	if status == "" {
		ids, err = c.queries.GetAllIssueIDs(ctx)
	} else {
		ids, err = c.queries.GetIssueIDsByStatus(ctx, status)
	}
	if err != nil {
		return nil, fmt.Errorf("fetching bead IDs: %w", err)
	}

	if len(ids) == 0 {
		return []Bead{}, nil
	}

	result, err := c.GetBeadsWithDeps(ctx, ids)
	if err != nil {
		return nil, err
	}

	// Convert map to slice
	beads := make([]Bead, 0, len(result.Beads))
	for _, bead := range result.Beads {
		beads = append(beads, bead)
	}

	return beads, nil
}

// GetReadyBeads returns all open beads where all dependencies are satisfied.
func (c *Client) GetReadyBeads(ctx context.Context) ([]Bead, error) {
	// Get all open beads
	ids, err := c.queries.GetIssueIDsByStatus(ctx, StatusOpen)
	if err != nil {
		return nil, fmt.Errorf("fetching open bead IDs: %w", err)
	}

	if len(ids) == 0 {
		return []Bead{}, nil
	}

	result, err := c.GetBeadsWithDeps(ctx, ids)
	if err != nil {
		return nil, err
	}

	// Filter to beads where all dependencies are closed
	var ready []Bead
	for _, id := range ids {
		bead, ok := result.Beads[id]
		if !ok {
			continue
		}

		// Check if all blocking dependencies are satisfied
		isReady := true
		for _, dep := range result.Dependencies[id] {
			if dep.Type == "blocks" && dep.Status != StatusClosed {
				isReady = false
				break
			}
		}

		if isReady {
			ready = append(ready, bead)
		}
	}

	return ready, nil
}

// GetTransitiveDependencies collects all transitive dependencies for a bead.
// Returns beads in dependency order (dependencies before dependents).
func (c *Client) GetTransitiveDependencies(ctx context.Context, id string) ([]Bead, error) {
	visited := make(map[string]bool)
	var orderedIDs []string

	// Recursive helper to collect dependency IDs
	var collect func(beadID string) error
	collect = func(beadID string) error {
		if visited[beadID] {
			return nil
		}
		visited[beadID] = true

		// Get this bead to find its dependencies
		result, err := c.GetBeadsWithDeps(ctx, []string{beadID})
		if err != nil {
			return err
		}

		// Recursively collect all blocked_by dependencies first
		for _, dep := range result.Dependencies[beadID] {
			if dep.Type == "blocked_by" && !visited[dep.DependsOnID] {
				if err := collect(dep.DependsOnID); err != nil {
					return err
				}
			}
		}

		// Add this bead after its dependencies
		orderedIDs = append(orderedIDs, beadID)
		return nil
	}

	if err := collect(id); err != nil {
		return nil, err
	}

	// Fetch all collected beads in one call
	result, err := c.GetBeadsWithDeps(ctx, orderedIDs)
	if err != nil {
		return nil, err
	}

	// Return in dependency order
	beads := make([]Bead, 0, len(orderedIDs))
	for _, beadID := range orderedIDs {
		if bead, ok := result.Beads[beadID]; ok {
			beads = append(beads, bead)
		}
	}

	return beads, nil
}

// GetBeadWithChildren retrieves a bead and all its child beads recursively.
// This is useful for epic beads that have sub-beads (parent-child relationship).
func (c *Client) GetBeadWithChildren(ctx context.Context, id string) ([]Bead, error) {
	visited := make(map[string]bool)
	var orderedIDs []string

	// Recursive helper to collect bead and children IDs
	var collect func(beadID string) error
	collect = func(beadID string) error {
		if visited[beadID] {
			return nil
		}
		visited[beadID] = true

		// Add this bead first
		orderedIDs = append(orderedIDs, beadID)

		// Get this bead to find its children
		result, err := c.GetBeadsWithDeps(ctx, []string{beadID})
		if err != nil {
			return err
		}

		// Recursively collect all parent-child dependents
		for _, dep := range result.Dependents[beadID] {
			if dep.Type == "parent-child" && !visited[dep.IssueID] {
				if err := collect(dep.IssueID); err != nil {
					return err
				}
			}
		}

		return nil
	}

	if err := collect(id); err != nil {
		return nil, err
	}

	// Fetch all collected beads in one call
	result, err := c.GetBeadsWithDeps(ctx, orderedIDs)
	if err != nil {
		return nil, err
	}

	// Return in order (parent before children)
	beads := make([]Bead, 0, len(orderedIDs))
	for _, beadID := range orderedIDs {
		if bead, ok := result.Beads[beadID]; ok {
			beads = append(beads, bead)
		}
	}

	return beads, nil
}
