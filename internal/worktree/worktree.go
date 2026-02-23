package worktree

//go:generate moq -stub -out worktree_mock.go . Operations:WorktreeOperationsMock

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Worktree represents a git worktree.
type Worktree struct {
	Path   string // Absolute path to the worktree
	HEAD   string // Current HEAD commit SHA
	Branch string // Branch name (empty if detached)
}

// Operations defines the interface for worktree operations.
// This abstraction enables testing without actual git commands.
type Operations interface {
	// Create creates a new worktree at worktreePath from repoPath with a new branch.
	Create(ctx context.Context, repoPath, worktreePath, branch, baseBranch string) error
	// CreateFromExisting creates a worktree at worktreePath for an existing branch.
	CreateFromExisting(ctx context.Context, repoPath, worktreePath, branch string) error
	// RemoveForce forcefully removes a worktree even if it has uncommitted changes.
	RemoveForce(ctx context.Context, repoPath, worktreePath string) error
	// List returns all worktrees for the given repository.
	List(ctx context.Context, repoPath string) ([]Worktree, error)
	// ExistsPath checks if the worktree path exists on disk.
	ExistsPath(worktreePath string) bool
}

// CLIOperations implements Operations using the git CLI.
type CLIOperations struct{}

// Compile-time check that CLIOperations implements Operations.
var _ Operations = (*CLIOperations)(nil)

// NewOperations creates a new Operations implementation using the git CLI.
func NewOperations() Operations {
	return &CLIOperations{}
}

// Create implements Operations.Create.
func (c *CLIOperations) Create(ctx context.Context, repoPath, worktreePath, branch, baseBranch string) error {
	args := []string{"-C", repoPath, "worktree", "add", worktreePath, "-b", branch}
	if baseBranch != "" {
		args = append(args, baseBranch)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	setMiseTrustedPaths(cmd, worktreePath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create worktree: %w\n%s", err, output)
	}
	return nil
}

// CreateFromExisting implements Operations.CreateFromExisting.
func (c *CLIOperations) CreateFromExisting(ctx context.Context, repoPath, worktreePath, branch string) error {
	args := []string{"-C", repoPath, "worktree", "add", worktreePath, branch}
	cmd := exec.CommandContext(ctx, "git", args...)
	setMiseTrustedPaths(cmd, worktreePath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create worktree from existing branch: %w\n%s", err, output)
	}
	return nil
}

// setMiseTrustedPaths configures the MISE_TRUSTED_CONFIG_PATHS environment variable
// on a command so that mise trusts config files in the worktree path. This prevents
// git post-checkout hooks from failing when they invoke mise-managed tools (like beans)
// in the newly created worktree before `mise trust` has been run.
func setMiseTrustedPaths(cmd *exec.Cmd, worktreePath string) {
	// Trust the worktree directory and its parent (the project root).
	// The parent covers the project-level .mise.toml, and the worktree
	// itself covers the repo's mise.toml that gets checked out.
	projectRoot := filepath.Dir(filepath.Dir(worktreePath)) // worktreePath is <root>/<work-id>/tree
	trustPaths := projectRoot

	// Preserve any existing trusted paths
	if existing := os.Getenv("MISE_TRUSTED_CONFIG_PATHS"); existing != "" {
		trustPaths = existing + ":" + trustPaths
	}

	cmd.Env = append(os.Environ(), "MISE_TRUSTED_CONFIG_PATHS="+trustPaths)
}

// RemoveForce implements Operations.RemoveForce.
func (c *CLIOperations) RemoveForce(ctx context.Context, repoPath, worktreePath string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "worktree", "remove", "--force", worktreePath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to force remove worktree: %w\n%s", err, output)
	}
	return nil
}

// List implements Operations.List.
func (c *CLIOperations) List(ctx context.Context, repoPath string) ([]Worktree, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "worktree", "list", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}

	return parseWorktreeList(string(output))
}

// parseWorktreeList parses the porcelain output of git worktree list.
// Format:
// worktree /path/to/worktree
// HEAD <sha>
// branch refs/heads/<name>
// (blank line)
func parseWorktreeList(output string) ([]Worktree, error) {
	var worktrees []Worktree
	var current Worktree

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			// End of worktree entry
			if current.Path != "" {
				worktrees = append(worktrees, current)
			}
			current = Worktree{}
			continue
		}

		if strings.HasPrefix(line, "worktree ") {
			current.Path = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "HEAD ") {
			current.HEAD = strings.TrimPrefix(line, "HEAD ")
		} else if strings.HasPrefix(line, "branch ") {
			ref := strings.TrimPrefix(line, "branch ")
			// Strip refs/heads/ prefix to get branch name
			current.Branch = strings.TrimPrefix(ref, "refs/heads/")
		}
		// Ignore "detached" and other entries
	}

	// Don't forget the last entry if output doesn't end with blank line
	if current.Path != "" {
		worktrees = append(worktrees, current)
	}

	return worktrees, scanner.Err()
}

// ExistsPath implements Operations.ExistsPath.
func (c *CLIOperations) ExistsPath(worktreePath string) bool {
	info, err := os.Stat(worktreePath)
	if err != nil {
		return false
	}
	return info.IsDir()
}
