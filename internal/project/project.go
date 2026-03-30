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
	"github.com/sargehq/sarge/internal/zellij"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/git"
	"github.com/sargehq/sarge/internal/logging"
	"github.com/sargehq/sarge/internal/mise"
)

const (
	// ConfigDir is the directory name for project configuration.
	ConfigDir = ".sarge"
	// LegacyConfigDir is the old directory name, still recognized for existing projects.
	LegacyConfigDir = ".co"
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

// sessionNameForProject returns the default zellij session name for a project.
// This always returns "sarge-<project>" regardless of whether we're inside a zellij session.
func sessionNameForProject(projectName string) string {
	return fmt.Sprintf("sarge-%s", projectName)
}

// ResolveSessionName returns the zellij session to use for tab management.
// If we're already inside a zellij session (detected via $ZELLIJ_SESSION_NAME),
// it reuses that session. Otherwise, it returns the default "sarge-<project>" name.
// This allows sarge to create tabs in the user's existing zellij session
// rather than forcing a separate session.
func ResolveSessionName(projectName string) string {
	if current := zellij.CurrentSessionName(); current != "" {
		return current
	}
	return sessionNameForProject(projectName)
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

// find walks up from startDir looking for a .sarge/ directory.
// If a legacy .co/ directory is found instead, it is renamed to .sarge/ before loading.
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

		// Migrate legacy .co/ to .sarge/
		legacyConfig := filepath.Join(dir, LegacyConfigDir, ConfigFile)
		if _, err := os.Stat(legacyConfig); err == nil {
			if err := migrateLegacyConfigDir(dir); err != nil {
				return nil, err
			}
			return load(ctx, dir)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, fmt.Errorf("no project found (no %s directory)", ConfigDir)
		}
		dir = parent
	}
}

// migrateLegacyConfigDir renames .co/ to .sarge/ and rewrites any .co/ paths
// in the config file to .sarge/.
func migrateLegacyConfigDir(projectRoot string) error {
	oldDir := filepath.Join(projectRoot, LegacyConfigDir)
	newDir := filepath.Join(projectRoot, ConfigDir)
	if err := os.Rename(oldDir, newDir); err != nil {
		return fmt.Errorf("failed to migrate %s to %s: %w", LegacyConfigDir, ConfigDir, err)
	}
	fmt.Printf("Migrated project config: %s/ -> %s/\n", LegacyConfigDir, ConfigDir)

	// Rewrite .co/ references in config.toml (e.g. beans path = ".co/.beans")
	configPath := filepath.Join(newDir, ConfigFile)
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil // directory renamed successfully, config read failure is non-fatal
	}
	updated := strings.ReplaceAll(string(data), "\""+LegacyConfigDir+"/", "\""+ConfigDir+"/")
	if updated != string(data) {
		if err := os.WriteFile(configPath, []byte(updated), 0o600); err != nil {
			fmt.Printf("Warning: could not update config paths: %v\n", err)
		}
	}
	return nil
}

// load loads a project from the given root directory.
// The config directory is always .sarge/ (legacy .co/ is migrated before load is called).
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
	beansPath := cfg.Beans.Path

	// If beans path is empty (e.g., legacy [beads] config), auto-configure beans
	// the same way a new project would — check for repo beans, otherwise init project-local.
	if beansPath == "" {
		mainPath := filepath.Join(root, MainDir)
		var err error
		beansPath, err = setupBeans(ctx, root, mainPath)
		if err != nil {
			database.Close()
			return nil, fmt.Errorf("failed to auto-configure beans: %w", err)
		}
		cfg.Beans.Path = beansPath
		// Persist the updated config so this migration only happens once
		if err := commentOutBeadsAndSaveBeans(configPath, beansPath); err != nil {
			fmt.Printf("Warning: could not update config file: %v\n", err)
		} else {
			fmt.Printf("Migrated config: commented out [beads], added [beans] path = %q\n", beansPath)
		}
	}

	beansDir := filepath.Join(root, beansPath)
	beansClient, err := beans.NewClient(ctx, beans.ClientConfig{BeansDir: beansDir})
	if err != nil {
		database.Close() // Clean up the already-opened DB
		return nil, fmt.Errorf("failed to open beans client at %s: %w", beansDir, err)
	}
	proj.Beans = beansClient

	// Initialize logging
	if err := logging.Init(root, ConfigDir); err != nil {
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

	// Check if project already exists (check both .sarge and legacy .co)
	for _, dir := range []string{ConfigDir, LegacyConfigDir} {
		if _, err := os.Stat(filepath.Join(absDir, dir)); err == nil {
			return nil, fmt.Errorf("project already exists at %s (found %s)", absDir, dir)
		}
	}
	configDir := filepath.Join(absDir, ConfigDir)

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
	beansPath, err := setupBeans(ctx, absDir, mainPath)
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
const BeansPathProject = ".sarge/.beans"

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
func setupBeans(ctx context.Context, projectRoot, mainPath string) (beansPath string, err error) {
	// Check if repo already has beans
	repoBeansPath := filepath.Join(mainPath, ".beans")
	if _, err := os.Stat(repoBeansPath); err == nil {
		// Repo already has beans - use them
		fmt.Printf("Using existing beans in %s\n", repoBeansPath)
		beansPath = BeansPathRepo
	} else {
		projectBeansPath := filepath.Join(projectRoot, ConfigDir, ".beans")
		beansPath = BeansPathProject

		// Skip init if beans already exist (e.g. migrated from .co/.beans/)
		if _, err := os.Stat(filepath.Join(projectBeansPath, ".beans.yml")); err == nil {
			fmt.Printf("Using existing project-local beans in %s\n", projectBeansPath)
		} else {
			fmt.Printf("Initializing project-local beans in %s\n", projectBeansPath)
			if err := beans.Init(ctx, projectBeansPath, projectRoot); err != nil {
				return "", fmt.Errorf("failed to initialize beans: %w", err)
			}
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
		fmt.Printf("Mise: generated .mise.toml with sarge requirements\n")
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


// commentOutBeadsAndSaveBeans reads the config file, comments out any [beads]
// section, and appends a [beans] section with the given path.
func commentOutBeadsAndSaveBeans(configPath, beansPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	var out []string
	inBeads := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "[beads]" {
			inBeads = true
			out = append(out, "# "+line+" # migrated to [beans]")
			continue
		}
		// Stop commenting when we hit the next section or end of file
		if inBeads && strings.HasPrefix(trimmed, "[") && trimmed != "[beads]" {
			inBeads = false
		}
		if inBeads {
			out = append(out, "# "+line)
		} else {
			out = append(out, line)
		}
	}

	// Append the new [beans] section
	out = append(out,
		"",
		"# =============================================================================",
		"# Beans Configuration",
		"# =============================================================================",
		"[beans]",
		fmt.Sprintf("path = %q", beansPath),
	)

	return os.WriteFile(configPath, []byte(strings.Join(out, "\n")), 0o600)
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
