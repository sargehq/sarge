package linear

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/sargehq/sarge/internal/beads"
)

// Fetcher orchestrates fetching Linear issues and importing them into Beads
type Fetcher struct {
	client     ClientInterface
	beadsDir   string
	beadsCache map[string]string // linearID -> beanID cache
}

// NewFetcher creates a new fetcher with the given API key and beads directory
func NewFetcher(apiKey string, beadsDir string) (*Fetcher, error) {
	client, err := NewClient(apiKey)
	if err != nil {
		return nil, err
	}

	return &Fetcher{
		client:     client,
		beadsDir:   beadsDir,
		beadsCache: make(map[string]string),
	}, nil
}

// FetchAndImport fetches a Linear issue and imports it into Beads
// Returns the created bead ID and any error
func (f *Fetcher) FetchAndImport(ctx context.Context, linearIDOrURL string, opts *ImportOptions) (*ImportResult, error) {
	result := &ImportResult{
		Success: false,
	}

	// Parse and normalize the Linear ID
	linearID, err := ParseIssueIDOrURL(linearIDOrURL)
	if err != nil {
		result.Error = fmt.Errorf("invalid Linear ID or URL: %w", err)
		return result, result.Error
	}
	result.LinearID = linearID

	// Check if already imported (cached)
	if beanID, exists := f.beadsCache[linearID]; exists {
		result.BeanID = beanID
		result.Success = true
		result.SkipReason = "already imported (cached)"
		return result, nil
	}

	// Fetch the issue from Linear
	issue, err := f.client.GetIssue(ctx, linearID)
	if err != nil {
		result.Error = fmt.Errorf("failed to fetch issue from Linear: %w", err)
		return result, result.Error
	}
	result.LinearURL = issue.URL

	// Check if already imported by querying beads
	beanID, err := f.findExistingBead(ctx, linearID)
	if err != nil {
		result.Error = fmt.Errorf("failed to check for existing bead: %w", err)
		return result, result.Error
	}
	if beanID != "" {
		// If update mode is enabled, update the existing bead
		if opts != nil && opts.UpdateExisting {
			if err := f.updateExistingBead(ctx, beanID, issue, opts); err != nil {
				result.Error = fmt.Errorf("failed to update existing bead: %w", err)
				return result, result.Error
			}
			result.BeanID = beanID
			result.Success = true
			result.SkipReason = "updated existing bead"
			f.beadsCache[linearID] = beanID
			return result, nil
		}

		result.BeanID = beanID
		result.Success = true
		result.SkipReason = "already imported"
		f.beadsCache[linearID] = beanID
		return result, nil
	}

	// Apply filters if specified
	if opts != nil {
		if opts.StatusFilter != "" && MapStatus(issue.State) != opts.StatusFilter {
			result.SkipReason = fmt.Sprintf("filtered out by status (wanted: %s, got: %s)", opts.StatusFilter, MapStatus(issue.State))
			return result, nil
		}
		if opts.PriorityFilter != "" && MapPriority(issue.Priority) != opts.PriorityFilter {
			result.SkipReason = fmt.Sprintf("filtered out by priority (wanted: %s, got: %s)", opts.PriorityFilter, MapPriority(issue.Priority))
			return result, nil
		}
		if opts.AssigneeFilter != "" && (issue.Assignee == nil || issue.Assignee.Email != opts.AssigneeFilter) {
			result.SkipReason = "filtered out by assignee"
			return result, nil
		}
	}

	// Map Linear issue to Beads creation options
	beadOpts := MapIssueToBeadCreate(issue)

	// Override type if specified in options
	if opts != nil && opts.TypeFilter != "" {
		beadOpts.Type = opts.TypeFilter
	}

	// Format description with Linear metadata
	beadOpts.Description = FormatBeadDescription(issue)

	// Dry run: skip actual creation
	if opts != nil && opts.DryRun {
		result.Success = true
		result.SkipReason = "dry run"
		return result, nil
	}

	// Create the bead
	createdBeanID, err := f.createBead(ctx, beadOpts)
	if err != nil {
		result.Error = fmt.Errorf("failed to create bead: %w", err)
		return result, result.Error
	}

	result.BeanID = createdBeanID
	result.Success = true
	f.beadsCache[linearID] = createdBeanID

	// Handle dependencies if requested
	if opts != nil && opts.CreateDeps && len(issue.BlockedBy) > 0 {
		if err := f.createDependencies(ctx, createdBeanID, issue.BlockedBy, opts); err != nil {
			// Log but don't fail - the main import succeeded
			result.Error = fmt.Errorf("warning: failed to create dependencies: %w", err)
		}
	}

	return result, nil
}

// FetchBatch fetches and imports multiple Linear issues
func (f *Fetcher) FetchBatch(ctx context.Context, linearIDsOrURLs []string, opts *ImportOptions) ([]*ImportResult, error) {
	results := make([]*ImportResult, 0, len(linearIDsOrURLs))

	for _, idOrURL := range linearIDsOrURLs {
		result, err := f.FetchAndImport(ctx, idOrURL, opts)
		if err != nil {
			// Continue with other imports even if one fails
			result.Error = err
		}
		results = append(results, result)

		// Check for context cancellation
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}
	}

	return results, nil
}

// findExistingBead checks if a bead already exists for the given Linear ID
func (f *Fetcher) findExistingBead(ctx context.Context, linearID string) (string, error) {
	// First try to find by external_ref using bd list --external-ref
	// This is the most reliable method since we now set external_ref
	cmd := exec.CommandContext(ctx, "bd", "list", "--json")
	if f.beadsDir != "" {
		cmd.Dir = f.beadsDir
	}
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to list beads: %w", err)
	}

	var beadsList []struct {
		ID          string `json:"id"`
		ExternalRef string `json:"external_ref"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(output, &beadsList); err != nil {
		return "", fmt.Errorf("failed to parse beads list: %w", err)
	}

	for _, bead := range beadsList {
		// Check external_ref first (most reliable)
		if bead.ExternalRef == linearID {
			return bead.ID, nil
		}
		// Fallback: check if the bead's description contains the Linear ID
		if strings.Contains(bead.Description, linearID) ||
			strings.Contains(bead.Description, "linear.app/") && strings.Contains(bead.Description, linearID) {
			return bead.ID, nil
		}
	}

	return "", nil
}

// createBead creates a bead using the beads client and sets all metadata
func (f *Fetcher) createBead(ctx context.Context, opts *BeadCreateOptions) (string, error) {
	// Convert priority string (P0-P4) to int (0-4)
	priority := 2 // default to medium
	if len(opts.Priority) >= 2 && opts.Priority[0] == 'P' {
		switch opts.Priority[1] {
		case '0':
			priority = 0
		case '1':
			priority = 1
		case '2':
			priority = 2
		case '3':
			priority = 3
		case '4':
			priority = 4
		}
	}

	createOpts := beads.CreateOptions{
		Title:       opts.Title,
		Description: opts.Description,
		Type:        opts.Type,
		Priority:    priority,
	}

	cli := beads.NewCLI(f.beadsDir)
	beanID, err := cli.Create(ctx, createOpts)
	if err != nil {
		return "", err
	}

	// Set assignee if specified
	if opts.Assignee != "" {
		updateOpts := beads.UpdateOptions{
			Assignee: opts.Assignee,
		}
		if err := cli.Update(ctx, beanID, updateOpts); err != nil {
			return beanID, fmt.Errorf("created bead but failed to set assignee: %w", err)
		}
	}

	// Add labels if specified
	if len(opts.Labels) > 0 {
		if err := cli.AddLabels(ctx, beanID, opts.Labels); err != nil {
			return beanID, fmt.Errorf("created bead but failed to add labels: %w", err)
		}
	}

	// Set Linear ID as external reference
	if linearID, ok := opts.Metadata["linear_id"]; ok && linearID != "" {
		if err := cli.SetExternalRef(ctx, beanID, linearID); err != nil {
			return beanID, fmt.Errorf("created bead but failed to set external ref: %w", err)
		}
	}

	// Set status if it's not the default "open"
	if opts.Status != "" && opts.Status != "open" {
		updateOpts := beads.UpdateOptions{
			Status: opts.Status,
		}
		if err := cli.Update(ctx, beanID, updateOpts); err != nil {
			return beanID, fmt.Errorf("created bead but failed to set status: %w", err)
		}
	}

	return beanID, nil
}

// updateExistingBead updates an existing bead with fresh data from Linear
func (f *Fetcher) updateExistingBead(ctx context.Context, beanID string, issue *Issue, opts *ImportOptions) error {
	// Map Linear issue to Beads creation options (reuse mapping logic)
	beadOpts := MapIssueToBeadCreate(issue)

	// Override type if specified in options
	if opts != nil && opts.TypeFilter != "" {
		beadOpts.Type = opts.TypeFilter
	}

	// Format description with Linear metadata
	beadOpts.Description = FormatBeadDescription(issue)

	// Convert priority string (P0-P4) to int (0-4)
	var priority *int
	if beadOpts.Priority != "" {
		p := 2 // default to medium
		if len(beadOpts.Priority) >= 2 && beadOpts.Priority[0] == 'P' {
			switch beadOpts.Priority[1] {
			case '0':
				p = 0
			case '1':
				p = 1
			case '2':
				p = 2
			case '3':
				p = 3
			case '4':
				p = 4
			}
		}
		priority = &p
	}

	// Update the bead with all fields
	cli := beads.NewCLI(f.beadsDir)
	updateOpts := beads.UpdateOptions{
		Title:       beadOpts.Title,
		Type:        beadOpts.Type,
		Description: beadOpts.Description,
		Assignee:    beadOpts.Assignee,
		Priority:    priority,
		Status:      beadOpts.Status,
	}
	if err := cli.Update(ctx, beanID, updateOpts); err != nil {
		return fmt.Errorf("failed to update bead fields: %w", err)
	}

	// Update labels (replace all existing labels)
	if len(beadOpts.Labels) > 0 {
		if err := cli.AddLabels(ctx, beanID, beadOpts.Labels); err != nil {
			return fmt.Errorf("failed to update labels: %w", err)
		}
	}

	return nil
}

// createDependencies creates dependency relationships for imported beads
func (f *Fetcher) createDependencies(ctx context.Context, beanID string, blockedByIDs []string, opts *ImportOptions) error {
	depth := 1
	if opts.MaxDepDepth > 0 && depth > opts.MaxDepDepth {
		return nil
	}

	cli := beads.NewCLI(f.beadsDir)
	for _, blockedByID := range blockedByIDs {
		// Fetch and import the blocking issue if not already imported
		result, err := f.FetchAndImport(ctx, blockedByID, &ImportOptions{
			DryRun:      opts.DryRun,
			CreateDeps:  true,
			MaxDepDepth: opts.MaxDepDepth,
		})
		if err != nil {
			return fmt.Errorf("failed to import blocking issue %s: %w", blockedByID, err)
		}

		if result.BeanID == "" {
			continue
		}

		// Create the dependency relationship
		// beanID depends on result.BeanID
		if err := cli.AddDependency(ctx, beanID, result.BeanID); err != nil {
			return fmt.Errorf("failed to add dependency %s -> %s: %w", beanID, result.BeanID, err)
		}
	}

	return nil
}
