package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sargehq/sarge/internal/beads"
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
func FormatTabName(prefix, workID, friendlyName string) string {
	baseName := fmt.Sprintf("%s-%s", prefix, workID)
	if friendlyName != "" {
		return fmt.Sprintf("%s (%s)", baseName, friendlyName)
	}
	return baseName
}

// Project represents an orchestrator project.
type Project struct {
	Root   string        // Project directory path
	Config *Config       // Parsed config.toml
	DB     *db.DB        // Tracking database (lazy loaded)
	Beads  *beads.Client // Beads database client (for issue tracking)
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

	// Open the beads client automatically
	// Use the configured beads path (relative to project root)
	beadsDBPath := filepath.Join(root, cfg.Beads.Path, "beads.db")
	beadsClient, err := beads.NewClient(ctx, beads.DefaultClientConfig(beadsDBPath))
	if err != nil {
		database.Close() // Clean up the already-opened DB
		return nil, fmt.Errorf("failed to open beads database: %w", err)
	}
	proj.Beads = beadsClient

	// Initialize logging to .co/debug.log
	if err := logging.Init(root); err != nil {
		// Log initialization failure is non-fatal, but log it if we can
		logging.Warn("failed to initialize logging", "error", err)
	}

	return proj, nil
}

// Create initializes a new project at the given directory.
// repoSource can be a local path (symlinked) or GitHub URL (cloned).
func Create(ctx context.Context, dir, repoSource string) (*Project, error) {
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
	setupMise(absDir, mainPath)

	// 4. Create config (before beads init, so config exists)
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
		// Beads path will be set after setupBeads
	}

	// Save config with comprehensive documentation
	configPath := filepath.Join(configDir, ConfigFile)
	if err := cfg.SaveDocumentedConfig(configPath); err != nil {
		os.RemoveAll(absDir)
		return nil, err
	}

	// 5. Initialize beads (after mise, so bd CLI is available)
	beadsPath, err := setupBeads(ctx, repoSource, absDir, mainPath)
	if err != nil {
		os.RemoveAll(absDir)
		return nil, err
	}

	// Update config with beads path and save again
	cfg.Beads = BeadsConfig{
		Path: beadsPath,
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

// BeadsPathRepo is the path for beads in the repository (synced with git).
const BeadsPathRepo = "main/.beads"

// BeadsPathProject is the path for project-local beads (standalone, not synced).
const BeadsPathProject = ".co/.beads"

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

// setupBeads initializes or connects to beads for the project.
// Returns the beads path (relative to project root).
func setupBeads(ctx context.Context, source, projectRoot, mainPath string) (beadsPath string, err error) {
	// Check if repo already has beads
	repoBeadsPath := filepath.Join(mainPath, ".beads")
	if _, err := os.Stat(repoBeadsPath); err == nil {
		// Repo already has beads - use them
		fmt.Printf("Using existing beads in %s\n", repoBeadsPath)
		beadsPath = BeadsPathRepo

		// Regenerate the database from existing JSONL files
		if err := beads.Reinit(ctx, mainPath); err != nil {
			return "", fmt.Errorf("failed to initialize beads: %w", err)
		}

		// Install hooks for repo-based beads
		if err := beads.InstallHooks(ctx, mainPath); err != nil {
			return "", fmt.Errorf("failed to install beads hooks: %w", err)
		}
	} else {
		// No beads in repo - create project-local beads
		projectBeadsPath := filepath.Join(projectRoot, ConfigDir, ".beads")
		fmt.Printf("Initializing project-local beads in %s\n", projectBeadsPath)
		beadsPath = BeadsPathProject

		// Derive prefix from repo name
		prefix := repoNameFromSource(source)

		// Initialize beads in project directory (skip hooks - not synced to git)
		if err := beads.Init(ctx, projectBeadsPath, prefix); err != nil {
			return "", fmt.Errorf("failed to initialize beads: %w", err)
		}
	}

	return beadsPath, nil
}

// setupMise generates mise config and runs mise install.
func setupMise(projectRoot, mainPath string) {
	// Generate mise config in project root with sarge's required tools
	if err := mise.GenerateConfig(projectRoot, ""); err != nil {
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

// BeadsPath returns the path to the beads directory.
func (p *Project) BeadsPath() string {
	return filepath.Join(p.Root, p.Config.Beads.Path)
}

// WorktreePath returns the path where a task's worktree should be created.
func (p *Project) WorktreePath(taskID string) string {
	return filepath.Join(p.Root, taskID)
}

// Close closes any open resources (database and beads client).
func (p *Project) Close() error {
	var errs []error
	if p.Beads != nil {
		if err := p.Beads.Close(); err != nil {
			errs = append(errs, fmt.Errorf("closing beads client: %w", err))
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
