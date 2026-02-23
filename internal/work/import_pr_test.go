package work

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sargehq/sarge/internal/beans"
	"github.com/sargehq/sarge/internal/git"
	"github.com/sargehq/sarge/internal/github"
	"github.com/sargehq/sarge/internal/worktree"
	"github.com/stretchr/testify/require"
)

// newTestWorkService creates a WorkService with mocked dependencies for testing.
func newTestWorkService(client github.ClientInterface, gitOps git.Operations, worktreeOps worktree.Operations) *WorkService {
	return NewWorkServiceWithDeps(WorkServiceDeps{
		GitHubClient: client,
		Git:          gitOps,
		Worktree:     worktreeOps,
	})
}

func TestSetupWorktreeFromPR_Success(t *testing.T) {
	ctx := context.Background()

	metadata := &github.PRMetadata{
		Number:      123,
		URL:         "https://github.com/owner/repo/pull/123",
		Title:       "Test PR",
		HeadRefName: "feature-branch",
		BaseRefName: "main",
	}

	client := &github.GitHubClientMock{
		GetPRMetadataFunc: func(ctx context.Context, prURLOrNumber string, repo string) (*github.PRMetadata, error) {
			return metadata, nil
		},
	}

	fetchPRRefCalled := false
	gitOps := &git.GitOperationsMock{
		FetchPRRefFunc: func(ctx context.Context, repoPath string, prNumber int, localBranch string) error {
			fetchPRRefCalled = true
			require.Equal(t, 123, prNumber)
			require.Equal(t, "feature-branch", localBranch)
			return nil
		},
	}

	createFromExistingCalled := false
	worktreeOps := &worktree.WorktreeOperationsMock{
		CreateFromExistingFunc: func(ctx context.Context, repoPath, worktreePath, branch string) error {
			createFromExistingCalled = true
			require.Equal(t, "feature-branch", branch)
			require.Equal(t, "/work/dir/tree", worktreePath)
			return nil
		},
	}

	svc := newTestWorkService(client, gitOps, worktreeOps)

	resultMetadata, worktreePath, err := svc.SetupWorktreeFromPR(ctx, "/repo/path", "https://github.com/owner/repo/pull/123", "", "/work/dir", "")

	require.NoError(t, err)
	require.True(t, fetchPRRefCalled, "FetchPRRef was not called")
	require.True(t, createFromExistingCalled, "CreateFromExisting was not called")
	require.Equal(t, 123, resultMetadata.Number)
	require.Equal(t, "/work/dir/tree", worktreePath)
}

func TestSetupWorktreeFromPR_CustomBranchName(t *testing.T) {
	ctx := context.Background()

	metadata := &github.PRMetadata{
		Number:      123,
		URL:         "https://github.com/owner/repo/pull/123",
		Title:       "Test PR",
		HeadRefName: "feature-branch",
		BaseRefName: "main",
	}

	client := &github.GitHubClientMock{
		GetPRMetadataFunc: func(ctx context.Context, prURLOrNumber string, repo string) (*github.PRMetadata, error) {
			return metadata, nil
		},
	}

	gitOps := &git.GitOperationsMock{
		FetchPRRefFunc: func(ctx context.Context, repoPath string, prNumber int, localBranch string) error {
			require.Equal(t, "custom-branch", localBranch)
			return nil
		},
	}

	worktreeOps := &worktree.WorktreeOperationsMock{
		CreateFromExistingFunc: func(ctx context.Context, repoPath, worktreePath, branch string) error {
			require.Equal(t, "custom-branch", branch)
			return nil
		},
	}

	svc := newTestWorkService(client, gitOps, worktreeOps)

	_, _, err := svc.SetupWorktreeFromPR(ctx, "/repo/path", "https://github.com/owner/repo/pull/123", "", "/work/dir", "custom-branch")

	require.NoError(t, err)
}

func TestSetupWorktreeFromPR_MetadataError(t *testing.T) {
	ctx := context.Background()

	client := &github.GitHubClientMock{
		GetPRMetadataFunc: func(ctx context.Context, prURLOrNumber string, repo string) (*github.PRMetadata, error) {
			return nil, errors.New("API error")
		},
	}

	gitOps := &git.GitOperationsMock{}
	worktreeOps := &worktree.WorktreeOperationsMock{}

	svc := newTestWorkService(client, gitOps, worktreeOps)

	_, _, err := svc.SetupWorktreeFromPR(ctx, "/repo/path", "https://github.com/owner/repo/pull/123", "", "/work/dir", "")

	require.Error(t, err)
	require.Equal(t, "failed to get PR metadata: API error", err.Error())
}

func TestSetupWorktreeFromPR_FetchPRRefError(t *testing.T) {
	ctx := context.Background()

	metadata := &github.PRMetadata{
		Number:      123,
		HeadRefName: "feature-branch",
	}

	client := &github.GitHubClientMock{
		GetPRMetadataFunc: func(ctx context.Context, prURLOrNumber string, repo string) (*github.PRMetadata, error) {
			return metadata, nil
		},
	}

	gitOps := &git.GitOperationsMock{
		FetchPRRefFunc: func(ctx context.Context, repoPath string, prNumber int, localBranch string) error {
			return errors.New("fetch failed")
		},
	}

	worktreeOps := &worktree.WorktreeOperationsMock{}

	svc := newTestWorkService(client, gitOps, worktreeOps)

	resultMetadata, _, err := svc.SetupWorktreeFromPR(ctx, "/repo/path", "https://github.com/owner/repo/pull/123", "", "/work/dir", "")

	require.Error(t, err)
	// Metadata should still be returned on fetch failure
	require.NotNil(t, resultMetadata, "metadata should be returned even on fetch failure")
}

func TestSetupWorktreeFromPR_WorktreeCreateError(t *testing.T) {
	ctx := context.Background()

	metadata := &github.PRMetadata{
		Number:      123,
		HeadRefName: "feature-branch",
	}

	client := &github.GitHubClientMock{
		GetPRMetadataFunc: func(ctx context.Context, prURLOrNumber string, repo string) (*github.PRMetadata, error) {
			return metadata, nil
		},
	}

	gitOps := &git.GitOperationsMock{
		FetchPRRefFunc: func(ctx context.Context, repoPath string, prNumber int, localBranch string) error {
			return nil
		},
	}

	worktreeOps := &worktree.WorktreeOperationsMock{
		CreateFromExistingFunc: func(ctx context.Context, repoPath, worktreePath, branch string) error {
			return errors.New("worktree create failed")
		},
	}

	svc := newTestWorkService(client, gitOps, worktreeOps)

	resultMetadata, _, err := svc.SetupWorktreeFromPR(ctx, "/repo/path", "https://github.com/owner/repo/pull/123", "", "/work/dir", "")

	require.Error(t, err)
	// Metadata should still be returned on worktree create failure
	require.NotNil(t, resultMetadata, "metadata should be returned even on worktree create failure")
}

func TestMapPRToBeanCreate(t *testing.T) {
	pr := &github.PRMetadata{
		Number:      123,
		URL:         "https://github.com/owner/repo/pull/123",
		Title:       "Add new feature",
		Body:        "This PR adds a new feature",
		HeadRefName: "feature-branch",
		BaseRefName: "main",
		Author:      "testuser",
		Labels:      []string{"feature", "enhancement"},
		State:       "OPEN",
		Repo:        "owner/repo",
	}

	opts := mapPRToBeanCreate(pr)

	require.Equal(t, "Add new feature", opts.title)
	require.Equal(t, "This PR adds a new feature", opts.description)

	// Should detect feature type from labels
	require.Equal(t, "feature", opts.issueType)

	// Should have default P2 priority
	require.Equal(t, "P2", opts.priority)

	// Labels should be passed through
	require.Len(t, opts.labels, 2)

	// Metadata should contain PR info
	require.Equal(t, "https://github.com/owner/repo/pull/123", opts.metadata["pr_url"])
	require.Equal(t, "123", opts.metadata["pr_number"])
	require.Equal(t, "feature-branch", opts.metadata["pr_branch"])
	require.Equal(t, "testuser", opts.metadata["pr_author"])
}

func TestMapPRType(t *testing.T) {
	tests := []struct {
		name     string
		pr       *github.PRMetadata
		expected string
	}{
		{
			name: "Bug from label",
			pr: &github.PRMetadata{
				Title:  "Some change",
				Labels: []string{"bug"},
			},
			expected: "bug",
		},
		{
			name: "Bug from fix label",
			pr: &github.PRMetadata{
				Title:  "Some change",
				Labels: []string{"bugfix"},
			},
			expected: "bug",
		},
		{
			name: "Feature from label",
			pr: &github.PRMetadata{
				Title:  "Some change",
				Labels: []string{"feature"},
			},
			expected: "feature",
		},
		{
			name: "Feature from enhancement label",
			pr: &github.PRMetadata{
				Title:  "Some change",
				Labels: []string{"enhancement"},
			},
			expected: "feature",
		},
		{
			name: "Bug from title",
			pr: &github.PRMetadata{
				Title:  "Fix broken login",
				Labels: []string{},
			},
			expected: "bug",
		},
		{
			name: "Feature from title with feat",
			pr: &github.PRMetadata{
				Title:  "feat: Add new button",
				Labels: []string{},
			},
			expected: "feature",
		},
		{
			name: "Feature from title with add",
			pr: &github.PRMetadata{
				Title:  "Add user authentication",
				Labels: []string{},
			},
			expected: "feature",
		},
		{
			name: "Default to task",
			pr: &github.PRMetadata{
				Title:  "Update documentation",
				Labels: []string{},
			},
			expected: "task",
		},
		{
			name: "Label takes precedence over title",
			pr: &github.PRMetadata{
				Title:  "Fix: Add new feature", // Title suggests bug
				Labels: []string{"feature"},    // Label says feature
			},
			expected: "feature",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapPRType(tt.pr)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestMapPRPriority(t *testing.T) {
	tests := []struct {
		name     string
		pr       *github.PRMetadata
		expected string
	}{
		{
			name: "Critical from label",
			pr: &github.PRMetadata{
				Labels: []string{"critical"},
			},
			expected: "P0",
		},
		{
			name: "Urgent from label",
			pr: &github.PRMetadata{
				Labels: []string{"urgent"},
			},
			expected: "P0",
		},
		{
			name: "P0 from label",
			pr: &github.PRMetadata{
				Labels: []string{"p0"},
			},
			expected: "P0",
		},
		{
			name: "High priority from label",
			pr: &github.PRMetadata{
				Labels: []string{"high-priority"},
			},
			expected: "P1",
		},
		{
			name: "P1 from label",
			pr: &github.PRMetadata{
				Labels: []string{"priority-p1"},
			},
			expected: "P1",
		},
		{
			name: "Medium priority from label",
			pr: &github.PRMetadata{
				Labels: []string{"medium"},
			},
			expected: "P2",
		},
		{
			name: "P2 from label",
			pr: &github.PRMetadata{
				Labels: []string{"p2"},
			},
			expected: "P2",
		},
		{
			name: "Low priority from label",
			pr: &github.PRMetadata{
				Labels: []string{"low"},
			},
			expected: "P3",
		},
		{
			name: "P3 from label",
			pr: &github.PRMetadata{
				Labels: []string{"p3"},
			},
			expected: "P3",
		},
		{
			name: "Default to P2",
			pr: &github.PRMetadata{
				Labels: []string{"documentation"},
			},
			expected: "P2",
		},
		{
			name: "Empty labels default to P2",
			pr: &github.PRMetadata{
				Labels: []string{},
			},
			expected: "P2",
		},
		{
			name: "First matching priority wins",
			pr: &github.PRMetadata{
				Labels: []string{"low", "critical"}, // critical comes second but matches first in loop
			},
			expected: "P3", // low is checked before critical in the loop order
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapPRPriority(tt.pr)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestMapPRStatus(t *testing.T) {
	tests := []struct {
		name     string
		pr       *github.PRMetadata
		expected string
	}{
		{
			name: "Merged PR",
			pr: &github.PRMetadata{
				State:  "MERGED",
				Merged: true,
			},
			expected: "closed",
		},
		{
			name: "Open draft PR",
			pr: &github.PRMetadata{
				State:   "OPEN",
				IsDraft: true,
				Merged:  false,
			},
			expected: "open",
		},
		{
			name: "Open PR not draft",
			pr: &github.PRMetadata{
				State:   "OPEN",
				IsDraft: false,
				Merged:  false,
			},
			expected: "in_progress",
		},
		{
			name: "Closed PR",
			pr: &github.PRMetadata{
				State:  "CLOSED",
				Merged: false,
			},
			expected: "closed",
		},
		{
			name: "Merged state",
			pr: &github.PRMetadata{
				State:  "MERGED",
				Merged: false, // Even if Merged is false, MERGED state maps to closed
			},
			expected: "closed",
		},
		{
			name: "Unknown state defaults to open",
			pr: &github.PRMetadata{
				State:  "UNKNOWN",
				Merged: false,
			},
			expected: "open",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapPRStatus(tt.pr)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatBeanDescription(t *testing.T) {
	mergedAt := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name     string
		pr       *github.PRMetadata
		contains []string
	}{
		{
			name: "Basic PR",
			pr: &github.PRMetadata{
				Number:      123,
				URL:         "https://github.com/owner/repo/pull/123",
				Body:        "Original description",
				HeadRefName: "feature-branch",
				BaseRefName: "main",
				Author:      "testuser",
				State:       "OPEN",
			},
			contains: []string{
				"Original description",
				"**Imported from GitHub PR**",
				"PR: #123",
				"URL: https://github.com/owner/repo/pull/123",
				"Branch: feature-branch → main",
				"Author: testuser",
				"State: OPEN",
			},
		},
		{
			name: "Draft PR",
			pr: &github.PRMetadata{
				Number:      456,
				URL:         "https://github.com/owner/repo/pull/456",
				HeadRefName: "draft-branch",
				BaseRefName: "main",
				Author:      "otheruser",
				State:       "OPEN",
				IsDraft:     true,
			},
			contains: []string{
				"Draft: yes",
			},
		},
		{
			name: "Merged PR",
			pr: &github.PRMetadata{
				Number:      789,
				URL:         "https://github.com/owner/repo/pull/789",
				HeadRefName: "merged-branch",
				BaseRefName: "main",
				Author:      "mergeduser",
				State:       "MERGED",
				Merged:      true,
				MergedAt:    mergedAt,
			},
			contains: []string{
				"Merged: 2024-01-15",
			},
		},
		{
			name: "PR with labels",
			pr: &github.PRMetadata{
				Number:      101,
				URL:         "https://github.com/owner/repo/pull/101",
				HeadRefName: "labeled-branch",
				BaseRefName: "main",
				Author:      "labeluser",
				State:       "OPEN",
				Labels:      []string{"bug", "urgent", "needs-review"},
			},
			contains: []string{
				"Labels: bug, urgent, needs-review",
			},
		},
		{
			name: "Empty body",
			pr: &github.PRMetadata{
				Number:      200,
				URL:         "https://github.com/owner/repo/pull/200",
				Body:        "",
				HeadRefName: "no-body-branch",
				BaseRefName: "main",
				Author:      "noBodyUser",
				State:       "OPEN",
			},
			contains: []string{
				"**Imported from GitHub PR**",
				"PR: #200",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatBeanDescription(tt.pr)

			for _, expected := range tt.contains {
				require.True(t, strings.Contains(result, expected),
					"expected description to contain %q, got:\n%s", expected, result)
			}
		})
	}
}

func TestParsePriority(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"P0", beans.PriorityCritical},
		{"P1", beans.PriorityHigh},
		{"P2", beans.PriorityNormal},
		{"P3", beans.PriorityLow},
		{"P4", beans.PriorityDeferred},
		{"p0", beans.PriorityNormal},        // lowercase doesn't match, defaults to normal
		{"", beans.PriorityNormal},           // empty defaults to normal
		{"P5", beans.PriorityNormal},         // unknown P-level defaults to normal
		{"invalid", beans.PriorityNormal},
		{"P", beans.PriorityNormal},          // just P defaults to normal
		{"Priority1", beans.PriorityNormal},  // doesn't start with P followed by digit
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parsePriorityToBeans(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestCreateBeanOptions(t *testing.T) {
	opts := &CreateBeanOptions{
		BeansDir:         "/path/to/beans",
		SkipIfExists:     true,
		OverrideTitle:    "Custom Title",
		OverrideType:     "bug",
		OverridePriority: "P1",
	}

	require.Equal(t, "/path/to/beans", opts.BeansDir)
	require.True(t, opts.SkipIfExists)
	require.Equal(t, "Custom Title", opts.OverrideTitle)
	require.Equal(t, "bug", opts.OverrideType)
	require.Equal(t, "P1", opts.OverridePriority)
}

func TestCreateBeanResult(t *testing.T) {
	result := &CreateBeanResult{
		BeanID:     "bean-123",
		Created:    true,
		SkipReason: "",
	}

	require.Equal(t, "bean-123", result.BeanID)
	require.True(t, result.Created)

	// Test skip result
	skipResult := &CreateBeanResult{
		BeanID:     "existing-bean",
		Created:    false,
		SkipReason: "bean already exists for this PR",
	}

	require.False(t, skipResult.Created, "Created should be false for skipped bean")
	require.NotEmpty(t, skipResult.SkipReason, "SkipReason should be set for skipped bean")
}
