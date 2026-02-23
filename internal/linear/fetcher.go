package linear

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/sargehq/sarge/internal/beans"
)

// Fetcher orchestrates fetching Linear issues and importing them into Beans
type Fetcher struct {
	client     ClientInterface
	beansDir   string
	beansCache map[string]string // linearID -> beanID cache
}

// NewFetcher creates a new fetcher with the given API key and beans directory
func NewFetcher(apiKey string, beansDir string) (*Fetcher, error) {
	client, err := NewClient(apiKey)
	if err != nil {
		return nil, err
	}

	return &Fetcher{
		client:     client,
		beansDir:   beansDir,
		beansCache: make(map[string]string),
	}, nil
}

// FetchAndImport fetches a Linear issue and imports it into Beans
// Returns the created bean ID and any error
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
	if beanID, exists := f.beansCache[linearID]; exists {
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

	// Check if already imported by querying beans
	beanID, err := f.findExistingBean(ctx, linearID)
	if err != nil {
		result.Error = fmt.Errorf("failed to check for existing bean: %w", err)
		return result, result.Error
	}
	if beanID != "" {
		// If update mode is enabled, update the existing bean
		if opts != nil && opts.UpdateExisting {
			if err := f.updateExistingBean(ctx, beanID, issue, opts); err != nil {
				result.Error = fmt.Errorf("failed to update existing bean: %w", err)
				return result, result.Error
			}
			result.BeanID = beanID
			result.Success = true
			result.SkipReason = "updated existing bean"
			f.beansCache[linearID] = beanID
			return result, nil
		}

		result.BeanID = beanID
		result.Success = true
		result.SkipReason = "already imported"
		f.beansCache[linearID] = beanID
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

	// Map Linear issue to Beans creation options
	beanOpts := MapIssueToBeanCreate(issue)

	// Override type if specified in options
	if opts != nil && opts.TypeFilter != "" {
		beanOpts.Type = opts.TypeFilter
	}

	// Format description with Linear metadata
	beanOpts.Description = FormatBeanDescription(issue)

	// Dry run: skip actual creation
	if opts != nil && opts.DryRun {
		result.Success = true
		result.SkipReason = "dry run"
		return result, nil
	}

	// Create the bean
	createdBeanID, err := f.createBean(ctx, beanOpts)
	if err != nil {
		result.Error = fmt.Errorf("failed to create bean: %w", err)
		return result, result.Error
	}

	result.BeanID = createdBeanID
	result.Success = true
	f.beansCache[linearID] = createdBeanID

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

// findExistingBean checks if a bean already exists for the given Linear ID
func (f *Fetcher) findExistingBean(ctx context.Context, linearID string) (string, error) {
	// Use beans list --json to find by tag or body content
	args := []string{"list", "--json"}
	if f.beansDir != "" {
		args = append([]string{"--beans-path", f.beansDir}, args...)
	}
	cmd := exec.CommandContext(ctx, "beans", args...)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to list beans: %w", err)
	}

	var beansList []struct {
		ID   string   `json:"id"`
		Tags []string `json:"tags"`
		Body string   `json:"body"`
	}
	if err := json.Unmarshal(output, &beansList); err != nil {
		return "", fmt.Errorf("failed to parse beans list: %w", err)
	}

	linearTag := "linear:" + linearID
	for _, bean := range beansList {
		// Check tags first (most reliable - we store linear:ID as a tag)
		for _, tag := range bean.Tags {
			if tag == linearTag {
				return bean.ID, nil
			}
		}
		// Fallback: check if the bean's body contains the Linear ID
		if strings.Contains(bean.Body, linearID) ||
			(strings.Contains(bean.Body, "linear.app/") && strings.Contains(bean.Body, linearID)) {
			return bean.ID, nil
		}
	}

	return "", nil
}

// createBean creates a bean using the beans client and sets all metadata
func (f *Fetcher) createBean(ctx context.Context, opts *BeanCreateOptions) (string, error) {
	// Convert P0-P4 priority string to beans priority
	priority := mapPriorityToBeans(opts.Priority)

	createOpts := beans.CreateOptions{
		Title:    opts.Title,
		Body:     opts.Description,
		Type:     opts.Type,
		Priority: priority,
	}

	// Add labels including assignee and Linear ID as tags
	var tags []string
	tags = append(tags, opts.Labels...)
	if opts.Assignee != "" {
		tags = append(tags, "assignee:"+opts.Assignee)
	}
	if linearID, ok := opts.Metadata["linear_id"]; ok && linearID != "" {
		tags = append(tags, "linear:"+linearID)
	}
	if len(tags) > 0 {
		createOpts.Tags = tags
	}

	cli := beans.NewCLI(f.beansDir)
	beanID, err := cli.Create(ctx, createOpts)
	if err != nil {
		return "", err
	}

	// Set status if it's not the default "todo"
	if opts.Status != "" && opts.Status != beans.StatusTodo {
		updateOpts := beans.UpdateOptions{
			Status: opts.Status,
		}
		if err := cli.Update(ctx, beanID, updateOpts); err != nil {
			return beanID, fmt.Errorf("created bean but failed to set status: %w", err)
		}
	}

	return beanID, nil
}

// mapPriorityToBeans converts a P0-P4 string to a beans priority string.
func mapPriorityToBeans(p string) string {
	switch p {
	case "P0":
		return beans.PriorityCritical
	case "P1":
		return beans.PriorityHigh
	case "P2":
		return beans.PriorityNormal
	case "P3":
		return beans.PriorityLow
	case "P4":
		return beans.PriorityDeferred
	default:
		return beans.PriorityNormal
	}
}

// updateExistingBean updates an existing bean with fresh data from Linear
func (f *Fetcher) updateExistingBean(ctx context.Context, beanID string, issue *Issue, opts *ImportOptions) error {
	// Map Linear issue to creation options (reuse mapping logic)
	beanOpts := MapIssueToBeanCreate(issue)

	// Override type if specified in options
	if opts != nil && opts.TypeFilter != "" {
		beanOpts.Type = opts.TypeFilter
	}

	// Format body with Linear metadata
	body := FormatBeanDescription(issue)

	// Convert priority
	priority := mapPriorityToBeans(beanOpts.Priority)

	// Update the bean with all fields
	cli := beans.NewCLI(f.beansDir)
	updateOpts := beans.UpdateOptions{
		Title:    beanOpts.Title,
		Type:     beanOpts.Type,
		Body:     body,
		Priority: priority,
		Status:   beanOpts.Status,
	}
	if err := cli.Update(ctx, beanID, updateOpts); err != nil {
		return fmt.Errorf("failed to update bean fields: %w", err)
	}

	// Update tags (labels + assignee)
	var tags []string
	tags = append(tags, beanOpts.Labels...)
	if beanOpts.Assignee != "" {
		tags = append(tags, "assignee:"+beanOpts.Assignee)
	}
	if len(tags) > 0 {
		if err := cli.AddTags(ctx, beanID, tags); err != nil {
			return fmt.Errorf("failed to update tags: %w", err)
		}
	}

	return nil
}

// createDependencies creates dependency relationships for imported beans
func (f *Fetcher) createDependencies(ctx context.Context, beanID string, blockedByIDs []string, opts *ImportOptions) error {
	depth := 1
	if opts.MaxDepDepth > 0 && depth > opts.MaxDepDepth {
		return nil
	}

	cli := beans.NewCLI(f.beansDir)
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
