package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sargehq/sarge/internal/beans"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/git"
	"github.com/sargehq/sarge/internal/logging"
	"github.com/sargehq/sarge/internal/mise"
)

const (
	// ConfigDir is the directory name for project configuration.
	ConfigDir = ".co"
	// ConfigFile is the name of the project config file.
	ConfigFile = "config.toml"
	// TrackingDB is the name of the tracking database file.
	TrackingDB = "tracking.db"
	// MainDir is the directory name for the main repository.
	MainDir = "main"

	// RepoTypeLocal indicates a symlinked local repository.
	RepoTypeLocal = "local"
	// RepoTypeGitHub indicates a cloned GitHub repository.
	RepoTypeGitHub = "github"
)

// SessionNameForProject returns the zellij session name for a specific project.
// This is used consistently across the codebase for session management.
func SessionNameForProject(projectName string) string {
	return fmt.Sprintf("sarge-%s", projectName)
}

// FormatTabName formats a tab name with an optional friendly name.
// If friendlyName is not empty, formats as "prefix-workID (friendlyName)", otherwise just "prefix-workID".
// This is used for zellij tab titles where the friendly name is nice for display.
func FormatTabName(prefix, workID, friendlyName string) string {
	baseName := fmt.Sprintf("%s-%s", prefix, workID)
	if friendlyName != "" {
		return fmt.Sprintf("%s (%s)", baseName, friendlyName)
	}
	return baseName
}

// FormatTabNameShort formats a tab name without the friendly name.
// This is used for zmx session names where the full name becomes a Unix socket
// path and must stay under the 104-byte macOS sun_path limit.
func FormatTabNameShort(prefix, workID string) string {
	return fmt.Sprintf("%s-%s", prefix, workID)
}

// Project represents an orchestrator project.
type Project struct {
	Root   string        // Project directory path
	Config *Config       // Parsed config.toml
	DB     *db.DB        // Tracking database (lazy loaded)
	Beans  *beans.Client // Beans client (for issue tracking)
}

// Find finds a project from a flag value or current directory.
// If flagValue is non-empty, uses that path; otherwise uses cwd.
func Find(ctx context.Context, flagValue string) (*Project, error) {
	if flagValue != "" {
		return find(ctx, flagValue)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return find(ctx, cwd)
}

// find walks up from startDir looking for a .co/ directory.
// Returns the project if found, or an error if not found.
func find(ctx context.Context, startDir string) (*Project, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path: %w", err)
	}

	for {
		configPath := filepath.Join(dir, ConfigDir, ConfigFile)
		if _, err := os.Stat(configPath); err == nil {
			return load(ctx, dir)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			return nil, fmt.Errorf("no project found (no %s directory)", ConfigDir)
		}
		dir = parent
	}
}

// load loads a project from the given root directory.
func load(ctx context.Context, root string) (*Project, error) {
	configPath := filepath.Join(root, ConfigDir, ConfigFile)
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config from %s: %w", configPath, err)
	}

	proj := &Project{
		Root:   root,
		Config: cfg,
	}

	// Open the database automatically
	dbPath := filepath.Join(root, ConfigDir, TrackingDB)
	database, err := db.OpenPath(ctx, dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open tracking database: %w", err)
	}
	proj.DB = database

	// Open the beans client automatically
	// Use the configured beans path (relative to project root)
	beansDir := filepath.Join(root, cfg.Beans.Path)
	beansClient, err := beans.NewClient(ctx, beans.ClientConfig{BeansDir: beansDir})
	if err != nil {
		database.Close() // Clean up the already-opened DB
		return nil, fmt.Errorf("failed to open beans client: %w", err)
	}
	proj.Beans = beansClient

	// Initialize logging to .co/debug.log
	if err := logging.Init(root); err != nil {
		// Log initialization failure is non-fatal, but log it if we can
		logging.Warn("failed to initialize logging", "error", err)
	}

	return proj, nil
}

// Create initializes a new project at the given directory with default tool selections.
// repoSource can be a local path (symlinked) or GitHub URL (cloned).
func Create(ctx context.Context, dir, repoSource string) (*Project, error) {
	return CreateWithSelections(ctx, dir, repoSource, "claude", mise.DefaultToolSelections())
}

// CreateWithSelections initializes a new project at the given directory with specific tool selections.
// agentType is stored in project config ("claude" or "pi").
// toolSelections controls which tools are added to .mise.toml (agent may or may not be included).
// repoSource can be a local path (symlinked) or GitHub URL (cloned).
func CreateWithSelections(ctx context.Context, dir, repoSource string, agentType string, toolSelections mise.ToolSelections) (*Project, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path: %w", err)
	}

	// Check if project already exists
	configDir := filepath.Join(absDir, ConfigDir)
	if _, err := os.Stat(configDir); err == nil {
		return nil, fmt.Errorf("project already exists at %s", absDir)
	}

	// 1. Create project directory structure
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create project directory: %w", err)
	}

	mainPath := filepath.Join(absDir, MainDir)

	// 2. Clone or symlink the repository
	repoType, err := cloneRepo(ctx, repoSource, mainPath)
	if err != nil {
		os.RemoveAll(absDir)
		return nil, err
	}

	// 3. Generate mise config and run mise install
	setupMise(absDir, mainPath, toolSelections)

	// 4. Create config (before beans init, so config exists)
	cfg := &Config{
		Project: ProjectConfig{
			Name:      filepath.Base(absDir),
			CreatedAt: time.Now(),
		},
		Repo: RepoConfig{
			Type:   repoType,
			Source: repoSource,
			Path:   MainDir,
		},
		Agent: AgentConfig{
			Type: agentType,
		},
		Multiplexer: MultiplexerConfig{
			Type: toolSelections.MultiplexerType,
		},
		// Beans path will be set after setupBeans
	}

	// Save config with comprehensive documentation
	configPath := filepath.Join(configDir, ConfigFile)
	if err := cfg.SaveDocumentedConfig(configPath); err != nil {
		os.RemoveAll(absDir)
		return nil, err
	}

	// 5. Initialize beans (after mise, so beans CLI is available)
	beansPath, err := setupBeans(ctx, repoSource, absDir, mainPath)
	if err != nil {
		os.RemoveAll(absDir)
		return nil, err
	}

	// Update config with beans path and save again
	cfg.Beans = BeansConfig{
		Path: beansPath,
	}
	if err := cfg.SaveDocumentedConfig(configPath); err != nil {
		os.RemoveAll(absDir)
		return nil, err
	}

	// 6. Initialize tracking database
	dbPath := filepath.Join(configDir, TrackingDB)
	database, err := db.OpenPath(ctx, dbPath)
	if err != nil {
		os.RemoveAll(absDir)
		return nil, fmt.Errorf("failed to initialize tracking database: %w", err)
	}
	database.Close()

	return &Project{
		Root:   absDir,
		Config: cfg,
	}, nil
}

// BeansPathRepo is the path for beans in the repository (synced with git).
const BeansPathRepo = "main/.beans"

// BeansPathProject is the path for project-local beans (standalone, not synced).
const BeansPathProject = ".co/.beans"

// cloneRepo sets up the main/ directory based on the repo source.
// It only handles cloning/symlinking the repository.
// Returns the repo type ("local" or "github").
func cloneRepo(ctx context.Context, source, mainPath string) (repoType string, err error) {
	if isGitHubURL(source) {
		// Clone from GitHub
		if err := git.NewOperations().Clone(ctx, source, mainPath); err != nil {
			return "", err
		}
		return RepoTypeGitHub, nil
	}

	// Local path - create symlink
	absSource, err := filepath.Abs(source)
	if err != nil {
		return "", fmt.Errorf("failed to resolve source path: %w", err)
	}

	// Verify source exists and is a directory
	info, err := os.Stat(absSource)
	if err != nil {
		return "", fmt.Errorf("source path does not exist: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("source path is not a directory: %s", absSource)
	}

	// Create symlink
	if err := os.Symlink(absSource, mainPath); err != nil {
		return "", fmt.Errorf("failed to create symlink: %w", err)
	}
	return RepoTypeLocal, nil
}

// setupBeans initializes or connects to beans for the project.
// Returns the beans path (relative to project root).
func setupBeans(ctx context.Context, source, projectRoot, mainPath string) (beansPath string, err error) {
	// Check if repo already has beans
	repoBeansPath := filepath.Join(mainPath, ".beans")
	if _, err := os.Stat(repoBeansPath); err == nil {
		// Repo already has beans - use them
		fmt.Printf("Using existing beans in %s\n", repoBeansPath)
		beansPath = BeansPathRepo
	} else {
		// No beans in repo - create project-local beans
		projectBeansPath := filepath.Join(projectRoot, ConfigDir, ".beans")
		fmt.Printf("Initializing project-local beans in %s\n", projectBeansPath)
		beansPath = BeansPathProject

		// Derive prefix from repo name
		prefix := repoNameFromSource(source)

		// Initialize beans in project directory
		if err := beans.Init(ctx, projectBeansPath, prefix); err != nil {
			return "", fmt.Errorf("failed to initialize beans: %w", err)
		}
	}

	return beansPath, nil
}

// setupMise generates mise config and runs mise install.
// toolSelections controls which tools to include in the generated mise config.
func setupMise(projectRoot, mainPath string, toolSelections mise.ToolSelections) {
	// Generate mise config in project root with sarge's required tools
	if err := mise.GenerateConfigWithSelections(projectRoot, toolSelections); err != nil {
		fmt.Printf("Warning: failed to generate mise config: %v\n", err)
	} else {
		fmt.Printf("Mise: generated .mise.toml with co requirements\n")
	}

	// Initialize mise in project root (optional - warn on error)
	if err := mise.Initialize(projectRoot); err != nil {
		fmt.Printf("Warning: mise initialization failed in project root: %v\n", err)
	}

	// Also initialize mise in the main repo directory if it has mise config
	// This handles repos with their own .mise.toml or .tool-versions
	if mise.IsManaged(mainPath) {
		fmt.Printf("Mise: initializing repo tools in %s\n", mainPath)
		if err := mise.Initialize(mainPath); err != nil {
			fmt.Printf("Warning: mise initialization failed in repo: %v\n", err)
		}
	}
}

// isGitHubURL returns true if the source looks like a GitHub URL.
func isGitHubURL(source string) bool {
	return strings.HasPrefix(source, "https://github.com/") ||
		strings.HasPrefix(source, "git@github.com:") ||
		strings.HasPrefix(source, "http://github.com/")
}

// repoNameFromSource extracts the first letter of the repository name from a source URL or path.
// For GitHub URLs: https://github.com/org/services -> "s"
// For local paths: /path/to/myrepo -> "m"
func repoNameFromSource(source string) string {
	// Remove trailing slashes and .git suffix
	source = strings.TrimSuffix(source, "/")
	source = strings.TrimSuffix(source, ".git")

	var name string
	// For GitHub URLs, extract the repo name (last path component)
	if isGitHubURL(source) {
		parts := strings.Split(source, "/")
		if len(parts) > 0 {
			name = parts[len(parts)-1]
		}
	} else {
		// For local paths, use the directory name
		name = filepath.Base(source)
	}

	// Return just the first letter (lowercase)
	if len(name) > 0 {
		return strings.ToLower(string(name[0]))
	}
	return "b" // fallback prefix
}

// MainRepoPath returns the path to the main repository.
func (p *Project) MainRepoPath() string {
	return filepath.Join(p.Root, MainDir)
}

// BeansPath returns the path to the beans directory.
func (p *Project) BeansPath() string {
	return filepath.Join(p.Root, p.Config.Beans.Path)
}

// WorktreePath returns the path where a task's worktree should be created.
func (p *Project) WorktreePath(taskID string) string {
	return filepath.Join(p.Root, taskID)
}

// Close closes any open resources (database and beans client).
func (p *Project) Close() error {
	var errs []error
	if p.Beans != nil {
		if err := p.Beans.Close(); err != nil {
			errs = append(errs, fmt.Errorf("closing beans client: %w", err))
		}
	}
	if p.DB != nil {
		if err := p.DB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("closing database: %w", err))
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
