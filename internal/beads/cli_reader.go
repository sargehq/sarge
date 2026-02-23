package beads

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sargehq/sarge/internal/beads/cachemanager"
	"github.com/sargehq/sarge/internal/logging"
)

// bdShowIssue is the JSON structure returned by `bd show --json`.
type bdShowIssue struct {
	ID                 string         `json:"id"`
	Title              string         `json:"title"`
	Description        string         `json:"description"`
	Design             string         `json:"design"`
	AcceptanceCriteria string         `json:"acceptance_criteria"`
	Notes              string         `json:"notes"`
	Status             string         `json:"status"`
	Priority           int            `json:"priority"`
	IssueType          string         `json:"issue_type"`
	Assignee           string         `json:"assignee"`
	EstimatedMinutes   int            `json:"estimated_minutes"`
	CreatedAt          time.Time      `json:"created_at"`
	CreatedBy          string         `json:"created_by"`
	Owner              string         `json:"owner"`
	UpdatedAt          time.Time      `json:"updated_at"`
	ClosedAt           *time.Time     `json:"closed_at"`
	CloseReason        string         `json:"close_reason"`
	ExternalRef        string         `json:"external_ref"`
	Parent             string         `json:"parent"`
	Dependencies       []bdShowDep    `json:"dependencies"`
	Dependents         []bdShowDep    `json:"dependents"`
}

// bdShowDep is a dependency/dependent entry in `bd show --json` output.
type bdShowDep struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Status         string `json:"status"`
	DependencyType string `json:"dependency_type"`
}

// bdListIssue is the JSON structure returned by `bd list --json`.
type bdListIssue struct {
	ID               string    `json:"id"`
	Title            string    `json:"title"`
	Description      string    `json:"description"`
	Design           string    `json:"design"`
	Notes            string    `json:"notes"`
	Status           string    `json:"status"`
	Priority         int       `json:"priority"`
	IssueType        string    `json:"issue_type"`
	Assignee         string    `json:"assignee"`
	EstimatedMinutes int       `json:"estimated_minutes"`
	CreatedAt        time.Time `json:"created_at"`
	CreatedBy        string    `json:"created_by"`
	Owner            string    `json:"owner"`
	UpdatedAt        time.Time `json:"updated_at"`
	ClosedAt         *time.Time `json:"closed_at"`
	CloseReason      string    `json:"close_reason"`
	ExternalRef      string    `json:"external_ref"`
}

// CLIReader implements the Reader interface using the bd CLI.
// It caches results in memory and can be flushed when the underlying data changes.
type CLIReader struct {
	beadsDir string

	mu    sync.RWMutex
	cache cachemanager.CacheManager[string, *BeadsWithDepsResult]

	// listCache stores the most recent list results keyed by status filter.
	listMu    sync.RWMutex
	listCache map[string]*listCacheEntry
}

type listCacheEntry struct {
	beads     []Bead
	fetchedAt time.Time
}

const listCacheTTL = 30 * time.Second

// CLIReaderConfig holds configuration for the CLIReader.
type CLIReaderConfig struct {
	BeadsDir         string
	CacheExpiration  time.Duration
	CacheCleanupTime time.Duration
}

// DefaultCLIReaderConfig returns default configuration.
func DefaultCLIReaderConfig(beadsDir string) CLIReaderConfig {
	return CLIReaderConfig{
		BeadsDir:         beadsDir,
		CacheExpiration:  10 * time.Minute,
		CacheCleanupTime: 30 * time.Minute,
	}
}

// NewCLIReader creates a new CLI-based reader.
func NewCLIReader(_ context.Context, cfg CLIReaderConfig) (*CLIReader, error) {
	cache := cachemanager.NewInMemoryCacheManager[string, *BeadsWithDepsResult](
		"beads-cli",
		cfg.CacheExpiration,
		cfg.CacheCleanupTime,
	)

	return &CLIReader{
		beadsDir:  cfg.BeadsDir,
		cache:     cache,
		listCache: make(map[string]*listCacheEntry),
	}, nil
}

// Close is a no-op for the CLI reader.
func (r *CLIReader) Close() error {
	return nil
}

// FlushCache clears all cached data.
func (r *CLIReader) FlushCache(ctx context.Context) error {
	r.listMu.Lock()
	r.listCache = make(map[string]*listCacheEntry)
	r.listMu.Unlock()

	if r.cache != nil {
		return r.cache.Flush(ctx)
	}
	return nil
}

// runBD executes a bd command and returns stdout bytes.
func (r *CLIReader) runBD(ctx context.Context, args ...string) ([]byte, error) {
	cmd := bdCommand(ctx, r.beadsDir, args...)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if ok := isExitError(err, &exitErr); ok {
			return nil, fmt.Errorf("bd %s failed: %w\n%s", strings.Join(args, " "), err, exitErr.Stderr)
		}
		return nil, fmt.Errorf("bd %s failed: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

// isExitError is a helper to check for exec.ExitError.
func isExitError(err error, target **exec.ExitError) bool {
	if e, ok := err.(*exec.ExitError); ok {
		*target = e
		return true
	}
	return false
}

// bdShow calls `bd show <ids...> --json` and returns parsed issues.
func (r *CLIReader) bdShow(ctx context.Context, ids ...string) ([]bdShowIssue, error) {
	args := append(ids, "--json")
	args = append([]string{"show"}, args...)
	out, err := r.runBD(ctx, args...)
	if err != nil {
		return nil, err
	}

	var issues []bdShowIssue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parsing bd show output: %w", err)
	}
	return issues, nil
}

// bdList calls `bd list --json` with optional filters and returns parsed issues.
func (r *CLIReader) bdList(ctx context.Context, args ...string) ([]bdListIssue, error) {
	cmdArgs := append([]string{"list", "--json"}, args...)
	out, err := r.runBD(ctx, cmdArgs...)
	if err != nil {
		return nil, err
	}

	var issues []bdListIssue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parsing bd list output: %w", err)
	}
	return issues, nil
}

// beadFromBDShow converts a bdShowIssue to a Bead.
func beadFromBDShow(issue bdShowIssue) Bead {
	b := Bead{
		ID:                 issue.ID,
		Title:              issue.Title,
		Description:        issue.Description,
		Design:             issue.Design,
		AcceptanceCriteria: issue.AcceptanceCriteria,
		Notes:              issue.Notes,
		Status:             issue.Status,
		Priority:           issue.Priority,
		Type:               issue.IssueType,
		Assignee:           issue.Assignee,
		EstimatedMinutes:   issue.EstimatedMinutes,
		CreatedAt:          issue.CreatedAt,
		CreatedBy:          issue.CreatedBy,
		Owner:              issue.Owner,
		UpdatedAt:          issue.UpdatedAt,
		CloseReason:        issue.CloseReason,
		ExternalRef:        issue.ExternalRef,
		IsEpic:             issue.IssueType == "epic",
	}
	if issue.ClosedAt != nil {
		b.ClosedAt = *issue.ClosedAt
	}
	return b
}

// beadFromBDList converts a bdListIssue to a Bead.
func beadFromBDList(issue bdListIssue) Bead {
	b := Bead{
		ID:               issue.ID,
		Title:            issue.Title,
		Description:      issue.Description,
		Design:           issue.Design,
		Notes:            issue.Notes,
		Status:           issue.Status,
		Priority:         issue.Priority,
		Type:             issue.IssueType,
		Assignee:         issue.Assignee,
		EstimatedMinutes: issue.EstimatedMinutes,
		CreatedAt:        issue.CreatedAt,
		CreatedBy:        issue.CreatedBy,
		Owner:            issue.Owner,
		UpdatedAt:        issue.UpdatedAt,
		CloseReason:      issue.CloseReason,
		ExternalRef:      issue.ExternalRef,
		IsEpic:           issue.IssueType == "epic",
	}
	if issue.ClosedAt != nil {
		b.ClosedAt = *issue.ClosedAt
	}
	return b
}

// GetBead retrieves a single bead by ID with its dependencies/dependents.
func (r *CLIReader) GetBead(ctx context.Context, id string) (*BeadWithDeps, error) {
	result, err := r.GetBeadsWithDeps(ctx, []string{id})
	if err != nil {
		return nil, err
	}
	return result.GetBead(id), nil
}

// GetBeadsWithDeps retrieves beads and their dependencies/dependents.
func (r *CLIReader) GetBeadsWithDeps(ctx context.Context, beadIDs []string) (*BeadsWithDepsResult, error) {
	if len(beadIDs) == 0 {
		return &BeadsWithDepsResult{
			Beads:        make(map[string]Bead),
			Dependencies: make(map[string][]Dependency),
			Dependents:   make(map[string][]Dependent),
		}, nil
	}

	// Build cache key from sorted IDs
	sortedIDs := make([]string, len(beadIDs))
	copy(sortedIDs, beadIDs)
	sort.Strings(sortedIDs)
	cacheKey := strings.Join(sortedIDs, ",")

	if r.cache != nil {
		if cached, found := r.cache.Get(ctx, cacheKey); found {
			return cached, nil
		}
	}

	// Fetch via bd show (supports multiple IDs)
	issues, err := r.bdShow(ctx, beadIDs...)
	if err != nil {
		return nil, fmt.Errorf("fetching beads: %w", err)
	}

	beadsMap := make(map[string]Bead, len(issues))
	depsMap := make(map[string][]Dependency)
	dependentsMap := make(map[string][]Dependent)

	for _, issue := range issues {
		beadsMap[issue.ID] = beadFromBDShow(issue)

		for _, dep := range issue.Dependencies {
			depsMap[issue.ID] = append(depsMap[issue.ID], Dependency{
				IssueID:     issue.ID,
				DependsOnID: dep.ID,
				Type:        dep.DependencyType,
				Status:      dep.Status,
				Title:       dep.Title,
			})
		}

		for _, dep := range issue.Dependents {
			dependentsMap[issue.ID] = append(dependentsMap[issue.ID], Dependent{
				IssueID:     dep.ID,
				DependsOnID: issue.ID,
				Type:        dep.DependencyType,
				Status:      dep.Status,
				Title:       dep.Title,
			})
		}
	}

	result := &BeadsWithDepsResult{
		Beads:        beadsMap,
		Dependencies: depsMap,
		Dependents:   dependentsMap,
	}

	if r.cache != nil {
		r.cache.Set(ctx, cacheKey, result, cachemanager.DefaultExpiration)
	}

	return result, nil
}

// ListBeads lists all beads with optional status filter.
func (r *CLIReader) ListBeads(ctx context.Context, status string) ([]Bead, error) {
	// Check list cache
	r.listMu.RLock()
	if entry, ok := r.listCache[status]; ok && time.Since(entry.fetchedAt) < listCacheTTL {
		r.listMu.RUnlock()
		return entry.beads, nil
	}
	r.listMu.RUnlock()

	var args []string
	if status != "" {
		args = append(args, "--status", status)
	}

	issues, err := r.bdList(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("listing beads: %w", err)
	}

	beads := make([]Bead, 0, len(issues))
	for _, issue := range issues {
		beads = append(beads, beadFromBDList(issue))
	}

	// Update list cache
	r.listMu.Lock()
	r.listCache[status] = &listCacheEntry{beads: beads, fetchedAt: time.Now()}
	r.listMu.Unlock()

	return beads, nil
}

// GetReadyBeads returns all open beads where all blocking dependencies are satisfied.
func (r *CLIReader) GetReadyBeads(ctx context.Context) ([]Bead, error) {
	issues, err := r.bdList(ctx, "--ready")
	if err != nil {
		return nil, fmt.Errorf("listing ready beads: %w", err)
	}

	beads := make([]Bead, 0, len(issues))
	for _, issue := range issues {
		beads = append(beads, beadFromBDList(issue))
	}
	return beads, nil
}

// GetTransitiveDependencies collects all transitive dependencies for a bead.
// Returns beads in dependency order (dependencies before dependents).
func (r *CLIReader) GetTransitiveDependencies(ctx context.Context, id string) ([]Bead, error) {
	visited := make(map[string]bool)
	var orderedIDs []string

	var collect func(beadID string) error
	collect = func(beadID string) error {
		if visited[beadID] {
			return nil
		}
		visited[beadID] = true

		result, err := r.GetBeadsWithDeps(ctx, []string{beadID})
		if err != nil {
			return err
		}

		for _, dep := range result.Dependencies[beadID] {
			if dep.Type == "blocked_by" && !visited[dep.DependsOnID] {
				if err := collect(dep.DependsOnID); err != nil {
					return err
				}
			}
		}

		orderedIDs = append(orderedIDs, beadID)
		return nil
	}

	if err := collect(id); err != nil {
		return nil, err
	}

	result, err := r.GetBeadsWithDeps(ctx, orderedIDs)
	if err != nil {
		return nil, err
	}

	beads := make([]Bead, 0, len(orderedIDs))
	for _, beadID := range orderedIDs {
		if bead, ok := result.Beads[beadID]; ok {
			beads = append(beads, bead)
		}
	}
	return beads, nil
}

// GetBeadWithChildren retrieves a bead and all its child beads recursively.
func (r *CLIReader) GetBeadWithChildren(ctx context.Context, id string) ([]Bead, error) {
	visited := make(map[string]bool)
	var orderedIDs []string

	var collect func(beadID string) error
	collect = func(beadID string) error {
		if visited[beadID] {
			return nil
		}
		visited[beadID] = true
		orderedIDs = append(orderedIDs, beadID)

		result, err := r.GetBeadsWithDeps(ctx, []string{beadID})
		if err != nil {
			return err
		}

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

	result, err := r.GetBeadsWithDeps(ctx, orderedIDs)
	if err != nil {
		return nil, err
	}

	beads := make([]Bead, 0, len(orderedIDs))
	for _, beadID := range orderedIDs {
		if bead, ok := result.Beads[beadID]; ok {
			beads = append(beads, bead)
		}
	}
	return beads, nil
}

// CloseEligibleParents closes any parent beads where all children are complete.
func (r *CLIReader) CloseEligibleParents(ctx context.Context, beadsDir string) error {
	allBeads, err := r.ListBeads(ctx, "")
	if err != nil {
		return fmt.Errorf("listing beads: %w", err)
	}

	if len(allBeads) == 0 {
		return nil
	}

	allIDs := make([]string, 0, len(allBeads))
	for _, b := range allBeads {
		allIDs = append(allIDs, b.ID)
	}

	result, err := r.GetBeadsWithDeps(ctx, allIDs)
	if err != nil {
		return fmt.Errorf("fetching beads with deps: %w", err)
	}

	closed := make(map[string]bool)
	cli := NewCLI(beadsDir)

	for {
		var toClose []string

		for id, bead := range result.Beads {
			if bead.Status == StatusClosed || closed[id] {
				continue
			}

			dependents := result.Dependents[id]
			var children []string
			for _, dep := range dependents {
				if dep.Type == "parent-child" {
					children = append(children, dep.IssueID)
				}
			}

			if len(children) == 0 {
				continue
			}

			allChildrenClosed := true
			for _, childID := range children {
				child, ok := result.Beads[childID]
				if !ok || (child.Status != StatusClosed && !closed[childID]) {
					allChildrenClosed = false
					break
				}
			}

			if allChildrenClosed {
				toClose = append(toClose, id)
			}
		}

		if len(toClose) == 0 {
			break
		}

		for _, id := range toClose {
			if err := cli.Close(ctx, id); err != nil {
				logging.Warn("failed to close parent bead", "id", id, "error", err)
				continue
			}
			closed[id] = true
			logging.Debug("closed parent bead", "id", id)
		}
	}

	return nil
}

// Compile-time check that CLIReader implements Reader.
var _ Reader = (*CLIReader)(nil)
