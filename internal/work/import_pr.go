package work

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/sargehq/sarge/internal/beads"
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

// CreateBeadOptions contains options for creating a bead from a PR.
type CreateBeadOptions struct {
	// BeadsDir is the directory containing the beads database.
	BeadsDir string
	// SkipIfExists skips creation if a bead with the same PR URL already exists.
	SkipIfExists bool
	// OverrideTitle allows overriding the PR title.
	OverrideTitle string
	// OverrideType allows overriding the inferred type.
	OverrideType string
	// OverridePriority allows overriding the inferred priority.
	OverridePriority string
}

// CreateBeadResult contains the result of creating a bead from a PR.
type CreateBeadResult struct {
	BeanID     string
	Created    bool
	SkipReason string
}

// CreateBeadFromPR creates a bead from PR metadata.
// This allows users to optionally track imported PRs in the beads system.
func (s *WorkService) CreateBeadFromPR(ctx context.Context, metadata *github.PRMetadata, opts *CreateBeadOptions) (*CreateBeadResult, error) {
	logging.Info("creating bead from PR",
		"prNumber", metadata.Number,
		"prTitle", metadata.Title,
		"beadsDir", opts.BeadsDir)

	result := &CreateBeadResult{}

	// Check for existing bead if requested
	if opts.SkipIfExists {
		existingID, err := s.findBeadByExternalRef(ctx, metadata.URL)
		if err != nil {
			logging.Warn("failed to check for existing bead", "error", err)
			// Continue anyway - we'll try to create
		} else if existingID != "" {
			result.BeanID = existingID
			result.Created = false
			result.SkipReason = "bead already exists for this PR"
			logging.Info("found existing bead for PR", "beanID", existingID)
			return result, nil
		}
	}

	// Map PR to bead options
	beadOpts := mapPRToBeadCreate(metadata)

	// Apply overrides
	if opts.OverrideTitle != "" {
		beadOpts.title = opts.OverrideTitle
	}
	if opts.OverrideType != "" {
		beadOpts.issueType = opts.OverrideType
	}
	if opts.OverridePriority != "" {
		beadOpts.priority = opts.OverridePriority
	}

	// Format description with PR metadata
	beadOpts.description = formatBeadDescription(metadata)

	// Convert priority string (P0-P4) to int (0-4)
	priority := parsePriority(beadOpts.priority)

	// Create the bead
	createOpts := beads.CreateOptions{
		Title:       beadOpts.title,
		Description: beadOpts.description,
		Type:        beadOpts.issueType,
		Priority:    priority,
	}

	cli := beads.NewCLI(opts.BeadsDir)
	beanID, err := cli.Create(ctx, createOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create bead: %w", err)
	}

	// Set external reference to PR URL for deduplication
	if err := cli.SetExternalRef(ctx, beanID, metadata.URL); err != nil {
		logging.Warn("failed to set external ref on bead", "error", err, "beanID", beanID)
		// Continue - bead was created successfully
	}

	// Add labels if present
	if len(beadOpts.labels) > 0 {
		if err := cli.AddLabels(ctx, beanID, beadOpts.labels); err != nil {
			logging.Warn("failed to add labels to bead", "error", err, "beanID", beanID)
			// Continue - bead was created successfully
		}
	}

	result.BeanID = beanID
	result.Created = true

	logging.Info("successfully created bead from PR",
		"beanID", beanID,
		"prNumber", metadata.Number)

	return result, nil
}

// beadCreateOptions represents internal options for creating a bead from a PR.
type beadCreateOptions struct {
	title       string
	description string
	issueType   string   // task, bug, feature
	priority    string   // P0-P4
	status      string   // open, in_progress, closed
	labels      []string // label names
	metadata    map[string]string
}

// mapPRToBeadCreate converts PR metadata to bead creation options.
func mapPRToBeadCreate(pr *github.PRMetadata) *beadCreateOptions {
	opts := &beadCreateOptions{
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

// mapPRType infers a bead issue type from PR labels and title.
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

// mapPRStatus converts PR state to bead status.
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

// formatBeadDescription formats a bead description with PR metadata.
func formatBeadDescription(pr *github.PRMetadata) string {
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

// findBeadByExternalRef checks if a bead already exists with the given external ref.
func (s *WorkService) findBeadByExternalRef(ctx context.Context, externalRef string) (string, error) {
	beadsList, err := s.BeadsReader.ListBeads(ctx, "")
	if err != nil {
		return "", fmt.Errorf("failed to list beads: %w", err)
	}

	for _, bead := range beadsList {
		if bead.ExternalRef == externalRef {
			return bead.ID, nil
		}
	}

	return "", nil
}

// parsePriority converts priority string (P0-P4) to int (0-4).
func parsePriority(priority string) int {
	if len(priority) >= 2 && priority[0] == 'P' {
		switch priority[1] {
		case '0':
			return 0
		case '1':
			return 1
		case '2':
			return 2
		case '3':
			return 3
		case '4':
			return 4
		}
	}
	return 2 // default to medium
}
