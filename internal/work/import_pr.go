package work

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/sargehq/sarge/internal/beans"
	"github.com/sargehq/sarge/internal/github"
	"github.com/sargehq/sarge/internal/logging"
)

// SetupWorktreeFromPR fetches a PR's branch and creates a worktree for it.
// It returns the created worktree path and the PR metadata.
//
// Parameters:
//   - repoPath: Path to the main repository
//   - prURLOrNumber: PR URL or number
//   - repo: Repository in owner/repo format (only needed if prURLOrNumber is a number)
//   - workDir: Directory where the worktree should be created (worktree will be at workDir/tree)
//   - branchName: Name to use for the local branch (if empty, uses the PR's branch name)
//
// The function:
// 1. Fetches PR metadata to get branch information
// 2. Fetches the PR's head ref using GitHub's refs/pull/<n>/head
// 3. Creates a worktree at workDir/tree from the fetched branch
func (s *WorkService) SetupWorktreeFromPR(ctx context.Context, repoPath, prURLOrNumber, repo, workDir, branchName string) (*github.PRMetadata, string, error) {
	logging.Info("setting up worktree from PR",
		"repoPath", repoPath,
		"prURLOrNumber", prURLOrNumber,
		"repo", repo,
		"workDir", workDir,
		"branchName", branchName)

	// Fetch PR metadata
	metadata, err := s.GitHubClient.GetPRMetadata(ctx, prURLOrNumber, repo)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get PR metadata: %w", err)
	}

	// Determine the local branch name
	localBranch := branchName
	if localBranch == "" {
		localBranch = metadata.HeadRefName
	}

	// Fetch the PR's head ref
	logging.Debug("fetching PR ref", "prNumber", metadata.Number, "localBranch", localBranch)
	if err := s.Git.FetchPRRef(ctx, repoPath, metadata.Number, localBranch); err != nil {
		return metadata, "", fmt.Errorf("failed to fetch PR ref: %w", err)
	}

	// Create the worktree directory path
	worktreePath := filepath.Join(workDir, "tree")

	// Create worktree from the fetched branch
	logging.Debug("creating worktree", "worktreePath", worktreePath, "branch", localBranch)
	if err := s.Worktree.CreateFromExisting(ctx, repoPath, worktreePath, localBranch); err != nil {
		return metadata, "", fmt.Errorf("failed to create worktree: %w", err)
	}

	logging.Info("successfully set up worktree from PR",
		"prNumber", metadata.Number,
		"worktreePath", worktreePath,
		"branch", localBranch)

	return metadata, worktreePath, nil
}

// CreateBeanOptions contains options for creating a bean from a PR.
type CreateBeanOptions struct {
	// BeansDir is the directory containing the beans database.
	BeansDir string
	// SkipIfExists skips creation if a bean with the same PR URL already exists.
	SkipIfExists bool
	// OverrideTitle allows overriding the PR title.
	OverrideTitle string
	// OverrideType allows overriding the inferred type.
	OverrideType string
	// OverridePriority allows overriding the inferred priority.
	OverridePriority string
}

// CreateBeanResult contains the result of creating a bean from a PR.
type CreateBeanResult struct {
	BeanID     string
	Created    bool
	SkipReason string
}

// CreateBeanFromPR creates a bean from PR metadata.
// This allows users to optionally track imported PRs in the beans system.
func (s *WorkService) CreateBeanFromPR(ctx context.Context, metadata *github.PRMetadata, opts *CreateBeanOptions) (*CreateBeanResult, error) {
	logging.Info("creating bean from PR",
		"prNumber", metadata.Number,
		"prTitle", metadata.Title,
		"beansDir", opts.BeansDir)

	result := &CreateBeanResult{}

	// Check for existing bean if requested
	if opts.SkipIfExists {
		existingID, err := s.findBeanByExternalRef(ctx, metadata.URL)
		if err != nil {
			logging.Warn("failed to check for existing bean", "error", err)
			// Continue anyway - we'll try to create
		} else if existingID != "" {
			result.BeanID = existingID
			result.Created = false
			result.SkipReason = "bean already exists for this PR"
			logging.Info("found existing bean for PR", "beanID", existingID)
			return result, nil
		}
	}

	// Map PR to bean options
	beanOpts := mapPRToBeanCreate(metadata)

	// Apply overrides
	if opts.OverrideTitle != "" {
		beanOpts.title = opts.OverrideTitle
	}
	if opts.OverrideType != "" {
		beanOpts.issueType = opts.OverrideType
	}
	if opts.OverridePriority != "" {
		beanOpts.priority = opts.OverridePriority
	}

	// Format body with PR metadata
	beanOpts.description = formatBeanDescription(metadata)

	// Convert priority string (P0-P4) to beans priority
	priority := parsePriorityToBeans(beanOpts.priority)

	// Build tags from labels plus PR URL for deduplication
	tags := append(beanOpts.labels, "pr:"+metadata.URL)

	// Create the bean
	createOpts := beans.CreateOptions{
		Title:    beanOpts.title,
		Body:     beanOpts.description,
		Type:     beanOpts.issueType,
		Priority: priority,
		Tags:     tags,
	}

	cli := beans.NewCLI(opts.BeansDir)
	beanID, err := cli.Create(ctx, createOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create bean: %w", err)
	}

	result.BeanID = beanID
	result.Created = true

	logging.Info("successfully created bean from PR",
		"beanID", beanID,
		"prNumber", metadata.Number)

	return result, nil
}

// beanCreateOptions represents internal options for creating a bean from a PR.
type beanCreateOptions struct {
	title       string
	description string
	issueType   string   // task, bug, feature
	priority    string   // P0-P4
	status      string   // open, in_progress, closed
	labels      []string // label names
	metadata    map[string]string
}

// mapPRToBeanCreate converts PR metadata to bean creation options.
func mapPRToBeanCreate(pr *github.PRMetadata) *beanCreateOptions {
	opts := &beanCreateOptions{
		title:       pr.Title,
		description: pr.Body,
		issueType:   mapPRType(pr),
		priority:    mapPRPriority(pr),
		status:      mapPRStatus(pr),
		labels:      pr.Labels,
		metadata:    make(map[string]string),
	}

	// Store PR metadata
	opts.metadata["pr_url"] = pr.URL
	opts.metadata["pr_number"] = fmt.Sprintf("%d", pr.Number)
	opts.metadata["pr_branch"] = pr.HeadRefName
	opts.metadata["pr_base_branch"] = pr.BaseRefName
	opts.metadata["pr_author"] = pr.Author
	opts.metadata["pr_repo"] = pr.Repo

	return opts
}

// mapPRType infers a bean issue type from PR labels and title.
// Returns: "task", "bug", or "feature"
func mapPRType(pr *github.PRMetadata) string {
	// Check labels for type hints
	for _, label := range pr.Labels {
		labelLower := strings.ToLower(label)
		if strings.Contains(labelLower, "bug") || strings.Contains(labelLower, "fix") {
			return "bug"
		}
		if strings.Contains(labelLower, "feature") || strings.Contains(labelLower, "enhancement") {
			return "feature"
		}
	}

	// Check title for type hints
	titleLower := strings.ToLower(pr.Title)
	if strings.Contains(titleLower, "bug") || strings.Contains(titleLower, "fix") {
		return "bug"
	}
	if strings.Contains(titleLower, "feat") || strings.Contains(titleLower, "add") {
		return "feature"
	}

	// Default to task
	return "task"
}

// mapPRPriority infers priority from PR labels.
// Returns: "P0", "P1", "P2", "P3", or "P4"
func mapPRPriority(pr *github.PRMetadata) string {
	for _, label := range pr.Labels {
		labelLower := strings.ToLower(label)
		// Check for explicit priority labels
		if strings.Contains(labelLower, "critical") || strings.Contains(labelLower, "urgent") || strings.Contains(labelLower, "p0") {
			return "P0"
		}
		if strings.Contains(labelLower, "high") || strings.Contains(labelLower, "p1") {
			return "P1"
		}
		if strings.Contains(labelLower, "medium") || strings.Contains(labelLower, "p2") {
			return "P2"
		}
		if strings.Contains(labelLower, "low") || strings.Contains(labelLower, "p3") {
			return "P3"
		}
	}
	// Default to medium priority
	return "P2"
}

// mapPRStatus converts PR state to bean status.
func mapPRStatus(pr *github.PRMetadata) string {
	if pr.Merged {
		return "closed"
	}
	switch strings.ToUpper(pr.State) {
	case "OPEN":
		if pr.IsDraft {
			return "open"
		}
		return "in_progress"
	case "CLOSED":
		return "closed"
	case "MERGED":
		return "closed"
	default:
		return "open"
	}
}

// formatBeanDescription formats a bean description with PR metadata.
func formatBeanDescription(pr *github.PRMetadata) string {
	var builder strings.Builder

	// Add the original PR body
	if pr.Body != "" {
		builder.WriteString(pr.Body)
		builder.WriteString("\n\n")
	}

	// Add PR metadata section
	builder.WriteString("---\n")
	builder.WriteString("**Imported from GitHub PR**\n")
	fmt.Fprintf(&builder, "- PR: #%d\n", pr.Number)
	fmt.Fprintf(&builder, "- URL: %s\n", pr.URL)
	fmt.Fprintf(&builder, "- Branch: %s → %s\n", pr.HeadRefName, pr.BaseRefName)
	fmt.Fprintf(&builder, "- Author: %s\n", pr.Author)
	fmt.Fprintf(&builder, "- State: %s\n", pr.State)

	if pr.IsDraft {
		builder.WriteString("- Draft: yes\n")
	}
	if pr.Merged {
		fmt.Fprintf(&builder, "- Merged: %s\n", pr.MergedAt.Format("2006-01-02"))
	}
	if len(pr.Labels) > 0 {
		fmt.Fprintf(&builder, "- Labels: %s\n", strings.Join(pr.Labels, ", "))
	}

	return builder.String()
}

// findBeanByExternalRef checks if a bean already exists with the given PR URL as a tag.
func (s *WorkService) findBeanByExternalRef(ctx context.Context, externalRef string) (string, error) {
	beansList, err := s.BeansReader.ListBeans(ctx, "")
	if err != nil {
		return "", fmt.Errorf("failed to list beans: %w", err)
	}

	prTag := "pr:" + externalRef
	for _, bean := range beansList {
		for _, tag := range bean.Tags {
			if tag == prTag {
				return bean.ID, nil
			}
		}
	}

	return "", nil
}

// parsePriorityToBeans converts priority string (P0-P4) to beans priority string.
func parsePriorityToBeans(priority string) string {
	switch priority {
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
