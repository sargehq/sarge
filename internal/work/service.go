package work

import (
	"path/filepath"

	"github.com/sargehq/sarge/internal/beads"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/git"
	"github.com/sargehq/sarge/internal/github"
	"github.com/sargehq/sarge/internal/names"
	"github.com/sargehq/sarge/internal/project"
	"github.com/sargehq/sarge/internal/task"
	"github.com/sargehq/sarge/internal/worktree"
)

// WorkService provides work operations with injectable dependencies.
// This enables both CLI and TUI to share the same tested core logic,
// and allows integration testing without external dependencies.
type WorkService struct {
	DB                  *db.DB
	Git                 git.Operations
	Worktree            worktree.Operations
	GitHubClient        github.ClientInterface
	BeadsReader         beads.Reader
	BeadsCLI            beads.CLI
	OrchestratorManager OrchestratorManager
	TaskPlanner         task.Planner
	NameGenerator       names.Generator
	Config              *project.Config
	ProjectRoot         string // Root directory of the project
	MainRepoPath        string // Path to the main repository
	BeadsDir            string // Path to beads directory
}

// NewWorkService creates a WorkService with production dependencies from a project.
func NewWorkService(proj *project.Project) *WorkService {
	// Compute beads directory from project config
	beadsDir := filepath.Join(proj.Root, proj.Config.Beads.Path)

	return &WorkService{
		DB:                  proj.DB,
		Git:                 git.NewOperations(),
		Worktree:            worktree.NewOperations(),
		GitHubClient:        github.NewClient(),
		BeadsReader:         proj.Beads,
		BeadsCLI:            beads.NewCLI(beadsDir),
		OrchestratorManager: NewOrchestratorManager(proj.DB),
		TaskPlanner:         nil, // Planner needs specific initialization, set separately if needed
		NameGenerator:       names.NewGenerator(),
		Config:              proj.Config,
		ProjectRoot:         proj.Root,
		MainRepoPath:        proj.MainRepoPath(),
		BeadsDir:            beadsDir,
	}
}

// WorkServiceDeps contains all dependencies for a WorkService.
// Used for testing to inject mocks for all dependencies.
type WorkServiceDeps struct {
	DB                  *db.DB
	Git                 git.Operations
	Worktree            worktree.Operations
	GitHubClient        github.ClientInterface
	BeadsReader         beads.Reader
	BeadsCLI            beads.CLI
	OrchestratorManager OrchestratorManager
	TaskPlanner         task.Planner
	NameGenerator       names.Generator
	Config              *project.Config
	ProjectRoot         string
	MainRepoPath        string
	BeadsDir            string
}

// NewWorkServiceWithDeps creates a WorkService with explicitly provided dependencies.
// This is the preferred constructor for testing.
func NewWorkServiceWithDeps(deps WorkServiceDeps) *WorkService {
	return &WorkService{
		DB:                  deps.DB,
		Git:                 deps.Git,
		Worktree:            deps.Worktree,
		GitHubClient:        deps.GitHubClient,
		BeadsReader:         deps.BeadsReader,
		BeadsCLI:            deps.BeadsCLI,
		OrchestratorManager: deps.OrchestratorManager,
		TaskPlanner:         deps.TaskPlanner,
		NameGenerator:       deps.NameGenerator,
		Config:              deps.Config,
		ProjectRoot:         deps.ProjectRoot,
		MainRepoPath:        deps.MainRepoPath,
		BeadsDir:            deps.BeadsDir,
	}
}

