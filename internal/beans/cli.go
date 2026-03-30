package beans

//go:generate moq -stub -out beans_mock.go . CLI:BeansCLIMock Reader:BeansReaderMock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sargehq/sarge/internal/logging"
)

// CLI defines the interface for beans command operations.
// This abstraction enables testing without actual beans CLI calls.
// Each CLI instance is bound to a specific beans directory.
type CLI interface {
	// Create creates a new bean and returns its ID.
	Create(ctx context.Context, opts CreateOptions) (string, error)
	// Close closes a bean (sets status to completed).
	Close(ctx context.Context, beanID string) error
	// Delete permanently removes a bean.
	Delete(ctx context.Context, beanID string, force bool) error
	// Reopen reopens a completed/scrapped bean (sets status to todo).
	Reopen(ctx context.Context, beanID string) error
	// Update updates a bean's fields.
	Update(ctx context.Context, beanID string, opts UpdateOptions) error
	// AddComment appends text to the bean body.
	AddComment(ctx context.Context, beanID, comment string) error
	// AddTags adds tags to a bean.
	AddTags(ctx context.Context, beanID string, tags []string) error
	// AddDependency makes beanID blocked by dependsOnID.
	AddDependency(ctx context.Context, beanID, dependsOnID string) error
}

// Reader defines the interface for reading beans data.
// This abstraction enables testing without actual beans CLI/GraphQL access.
type Reader interface {
	// GetBean retrieves a single bean by ID with its relationships.
	GetBean(ctx context.Context, id string) (*BeanWithDeps, error)
	// GetBeansWithDeps retrieves beans and their relationships.
	GetBeansWithDeps(ctx context.Context, beanIDs []string) (*BeansWithDepsResult, error)
	// ListBeans lists all beans with optional status filter.
	ListBeans(ctx context.Context, status string) ([]Bean, error)
	// GetReadyBeans returns all todo beans where all blockers are satisfied.
	GetReadyBeans(ctx context.Context) ([]Bean, error)
	// GetTransitiveDependencies collects all transitive blockers for a bean.
	GetTransitiveDependencies(ctx context.Context, id string) ([]Bean, error)
	// GetBeanWithChildren retrieves a bean and all its child beans recursively.
	GetBeanWithChildren(ctx context.Context, id string) ([]Bean, error)
}

// CreateOptions specifies options for creating a bean.
type CreateOptions struct {
	Title    string
	Type     string // "task", "bug", "feature", "epic", "milestone"
	Priority string // "critical", "high", "normal", "low", "deferred"
	IsEpic   bool
	Body     string
	Parent   string   // Parent bean ID
	Tags     []string // Optional tags
}

// UpdateOptions specifies options for updating a bean.
type UpdateOptions struct {
	Title    string
	Type     string
	Body     string
	Priority string
	Status   string
}

// cliImpl implements CLI using the beans command-line tool.
type cliImpl struct {
	beansDir string
}

// Compile-time check that cliImpl implements CLI.
var _ CLI = (*cliImpl)(nil)

// NewCLI creates a new CLI instance bound to the specified beans directory.
func NewCLI(beansDir string) CLI {
	return &cliImpl{beansDir: beansDir}
}

// beansCommand creates an exec.Cmd for running beans with --beans-path set.
func beansCommand(ctx context.Context, beansDir string, args ...string) *exec.Cmd {
	if beansDir != "" {
		args = append([]string{"--beans-path", beansDir}, args...)
	}
	cmd := exec.CommandContext(ctx, "beans", args...)
	return cmd
}

// createResponse is the JSON structure returned by beans create --json.
type createResponse struct {
	Success bool `json:"success"`
	Bean    struct {
		ID string `json:"id"`
	} `json:"bean"`
	Message string `json:"message"`
}

// Create implements CLI.Create.
func (c *cliImpl) Create(ctx context.Context, opts CreateOptions) (string, error) {
	beanType := opts.Type
	if opts.IsEpic {
		beanType = "epic"
	}

	args := []string{"create", opts.Title, "--type", beanType, "--priority", opts.Priority, "--json"}
	if opts.Body != "" {
		args = append(args, "--body", opts.Body)
	}
	if opts.Parent != "" {
		args = append(args, "--parent", opts.Parent)
	}
	for _, tag := range opts.Tags {
		if tag != "" {
			args = append(args, "--tag", tag)
		}
	}

	logging.Debug("creating bean", "args", args, "beansDir", c.beansDir, "opts", opts)

	cmd := beansCommand(ctx, c.beansDir, args...)
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			logging.Error("beans create failed", "error", err, "stderr", string(exitErr.Stderr), "args", args)
			return "", fmt.Errorf("failed to create bean: %w\n%s", err, exitErr.Stderr)
		}
		logging.Error("beans create failed", "error", err, "args", args)
		return "", fmt.Errorf("failed to create bean: %w", err)
	}

	var resp createResponse
	if err := json.Unmarshal(output, &resp); err != nil {
		logging.Error("failed to parse beans create output", "output", string(output), "error", err)
		return "", fmt.Errorf("failed to parse create response: %w", err)
	}

	if !resp.Success || resp.Bean.ID == "" {
		return "", fmt.Errorf("beans create returned no ID: %s", string(output))
	}

	logging.Debug("created bean", "beanID", resp.Bean.ID)
	return resp.Bean.ID, nil
}

// Close implements CLI.Close.
func (c *cliImpl) Close(ctx context.Context, beanID string) error {
	cmd := beansCommand(ctx, c.beansDir, "update", beanID, "--status", StatusCompleted)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to close bean %s: %w\n%s", beanID, err, output)
	}
	return nil
}

// Delete implements CLI.Delete.
func (c *cliImpl) Delete(ctx context.Context, beanID string, force bool) error {
	args := []string{"delete", beanID}
	if force {
		args = append(args, "--force")
	}
	cmd := beansCommand(ctx, c.beansDir, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete bean %s: %w\n%s", beanID, err, output)
	}
	return nil
}

// Reopen implements CLI.Reopen.
func (c *cliImpl) Reopen(ctx context.Context, beanID string) error {
	cmd := beansCommand(ctx, c.beansDir, "update", beanID, "--status", StatusTodo)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to reopen bean %s: %w\n%s", beanID, err, output)
	}
	return nil
}

// Update implements CLI.Update.
func (c *cliImpl) Update(ctx context.Context, beanID string, opts UpdateOptions) error {
	args := []string{"update", beanID}
	if opts.Title != "" {
		args = append(args, "--title", opts.Title)
	}
	if opts.Type != "" {
		args = append(args, "--type", opts.Type)
	}
	if opts.Body != "" {
		args = append(args, "--body", opts.Body)
	}
	if opts.Priority != "" {
		args = append(args, "--priority", opts.Priority)
	}
	if opts.Status != "" {
		args = append(args, "--status", opts.Status)
	}

	cmd := beansCommand(ctx, c.beansDir, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to update bean %s: %w\n%s", beanID, err, output)
	}
	return nil
}

// AddComment implements CLI.AddComment by appending text to the bean body.
func (c *cliImpl) AddComment(ctx context.Context, beanID, comment string) error {
	cmd := beansCommand(ctx, c.beansDir, "update", beanID, "--body-append", "\n\n---\n\n"+comment)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add comment to bean %s: %w\n%s", beanID, err, output)
	}
	return nil
}

// AddTags implements CLI.AddTags.
func (c *cliImpl) AddTags(ctx context.Context, beanID string, tags []string) error {
	if len(tags) == 0 {
		return nil
	}

	args := []string{"update", beanID}
	for _, tag := range tags {
		args = append(args, "--tag", tag)
	}

	cmd := beansCommand(ctx, c.beansDir, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add tags to bean %s: %w\n%s", beanID, err, output)
	}
	return nil
}

// AddDependency implements CLI.AddDependency.
// Makes beanID blocked by dependsOnID.
func (c *cliImpl) AddDependency(ctx context.Context, beanID, dependsOnID string) error {
	cmd := beansCommand(ctx, c.beansDir, "update", beanID, "--blocked-by", dependsOnID)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add dependency %s -> %s: %w\n%s", beanID, dependsOnID, err, output)
	}
	return nil
}

// Init initializes beans in the specified directory.
// Init initializes a new beans repository in beansDir.
// prefix is the bean ID prefix (e.g. "a" becomes "a-" in .beans.yml).
// projectRoot is the project root where mise is configured, used to invoke
// the mise-installed beans binary via "mise exec -- beans init".
func Init(ctx context.Context, beansDir, prefix, projectRoot string) error {
	// Ensure the target directory exists before running beans init
	if err := os.MkdirAll(beansDir, 0o750); err != nil {
		return fmt.Errorf("failed to create beans directory %s: %w", beansDir, err)
	}

	// Use "mise exec" to invoke beans, since beans was just installed by mise
	// and may not be on the global PATH yet.
	cmd := exec.CommandContext(ctx, "mise", "exec", "--", "beans", "init")
	cmd.Dir = beansDir

	// Set MISE_PROJECT_DIR so mise loads the project root's .mise.toml
	// (since cmd.Dir is the beans subdirectory, not the project root)
	cmd.Env = append(os.Environ(), "MISE_PROJECT_DIR="+projectRoot)

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("beans init failed: %w\n%s", err, output)
	}

	// Update the prefix in .beans.yml (beans init auto-derives it from the
	// directory name, but we want it based on the repo name).
	configPath := filepath.Join(beansDir, ".beans.yml")
	configData, err := os.ReadFile(configPath) // #nosec G304 -- path is constructed internally
	if err != nil {
		return fmt.Errorf("failed to read beans config: %w", err)
	}

	// Replace the auto-derived prefix with our desired prefix
	updated := strings.Replace(
		string(configData),
		"prefix: .beans-",
		"prefix: "+prefix+"-",
		1,
	)
	if updated == string(configData) {
		// Try without the dot prefix (directory name may vary)
		// Fall back to a more general replacement
		lines := strings.Split(string(configData), "\n")
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "prefix:") {
				lines[i] = "    prefix: " + prefix + "-"
				break
			}
		}
		updated = strings.Join(lines, "\n")
	}

	if err := os.WriteFile(configPath, []byte(updated), 0o600); err != nil {
		return fmt.Errorf("failed to update beans config: %w", err)
	}

	return nil
}

// EditCommand returns an exec.Cmd for opening a bean's markdown file in $EDITOR.
// It queries the bean's path via --json and opens it in the editor.
func EditCommand(ctx context.Context, beanID, beansDir string) *exec.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	// Get bean path from beans show --json
	showCmd := beansCommand(ctx, beansDir, "show", beanID, "--json")
	output, err := showCmd.Output()
	if err != nil {
		// Fall back to showing the bean
		return beansCommand(ctx, beansDir, "show", beanID)
	}

	var beanInfo struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(output, &beanInfo); err != nil || beanInfo.Path == "" {
		return beansCommand(ctx, beansDir, "show", beanID)
	}

	// Construct full path: beansDir/beans/<path>
	fullPath := filepath.Join(beansDir, "beans", beanInfo.Path)
	return exec.CommandContext(ctx, editor, fullPath) //nolint:gosec // editor is user-configured $EDITOR
}
